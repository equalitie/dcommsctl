package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"
)

const scriptGracePeriod = 20 * time.Second

type NetworkRequest struct {
	Name     string
	Internal bool
}

type SystemConfig struct {
	SSODomain          string
	DcommsConfFilesDir string
	AppsDir            string
	InstancesDir       string
	BackupsDir         string
}

type BackupRestoreStep struct {
	TmpContainerImage string
	ServiceName       string
	ScriptPath        string
}

type InstanceGenericConfig struct {
	SSOIssuerURI string
	ClientID     string
	ClientSecret string
	Domain       string
	OnionDomain  string
	Version      string
}

type ContainerBackendType int

const (
	ContainerBackendDocker = iota
	ContainerBackendPodman
)

type GlobalState struct {
	ctx         context.Context
	systemCfg   SystemConfig
	jobsManager *JobManager
}

func NewGlobalState() *GlobalState {
	return &GlobalState{
		jobsManager: NewJobManager(),
	}
}

func instanceGenID() string {
	return instanceNames[rand.IntN(len(instanceNames))]
}

func instanceCreate(ctx context.Context, instancesDir string, instanceName string) (string, error) {
	instanceID := instanceName + "-" + instanceGenID()

	pathToInst := path.Join(instancesDir, instanceID)
	for {
		if _, err := os.Stat(pathToInst); err != nil {
			if os.IsNotExist(err) {
				break
			}
		}

		instanceID = instanceName + "-" + instanceGenID()
		pathToInst = path.Join(instancesDir, instanceID)
	}

	folderToCreate := path.Join(pathToInst, "jobs")

	if err := os.MkdirAll(folderToCreate, 0o700); err != nil {
		return "", fmt.Errorf("create instance dir: %s", err)
	}

	return pathToInst, nil
}

func isPortBindable(port int) bool {
	addr := fmt.Sprintf(":%d", port)

	c, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	c.Close()

	return true
}

func crashLogInit() {
	name := fmt.Sprintf(
		"crash-%s-pid-%d.log",
		time.Now().UTC().Format("20060102T150405Z"),
		os.Getpid(),
	)

	f, err := os.OpenFile(
		filepath.Join("./", name),
		os.O_CREATE|os.O_WRONLY|os.O_EXCL,
		0o600,
	)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		panic(err)
	}
}

func main() {
	serverError := make(chan error, 1)
	app := NewGlobalState()

	rootContext, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	app.jobsManager.jobsDBpath, _ = filepath.Abs("./.jobs_db.json")

	if err := app.jobsManager.loadJobs(); err != nil {
		panic(err)
	}

	app.systemCfg.DcommsConfFilesDir, _ = filepath.Abs("./dcomms-docker-compose")
	app.systemCfg.InstancesDir, _ = filepath.Abs("./.instances")
	app.systemCfg.AppsDir, _ = filepath.Abs("./conf_tmpl")
	app.systemCfg.BackupsDir, _ = filepath.Abs("./.backups")

	jRPCServer := NewJRPCServer()
	jRPCServer.Logger = log.Default()

	jRPCServer.Register("app/list", rpcApplicationsList)
	jRPCServer.Register("app/get", rpcApplicationGet)

	jRPCServer.Register("job/log", app.rpcJobLog)
	jRPCServer.Register("job/status", app.rpcJobStatus)
	jRPCServer.Register("job/list", app.rpcJobsList)

	jRPCServer.Register("inst/list", app.rpcInstancesList)
	jRPCServer.Register("inst/install", app.rpcInstanceInstall)
	jRPCServer.Register("inst/uninstall", app.rpcInstanceUninstall)
	jRPCServer.Register("inst/start", app.rpcInstanceStart)
	jRPCServer.Register("inst/stop", app.rpcInstanceStop)

	jRPCServer.Register("inst/expose", rpcInstanceExpose)
	jRPCServer.Register("inst/hide", rpcInstanceHide)

	jRPCServer.Register("inst/backup", app.rpcInstanceBackup)
	jRPCServer.Register("inst/restore", app.rpcInstanceRestore)
	jRPCServer.Register("inst/upgrade", app.rpcInstanceUpgrade)
	// jRPCServer.Register("container/start", func() {})
	// jRPCServer.Register("container/stop", func() {})

	jRPCServer.reg.SaveSpecs("schema_jrpc.json")
	mux := http.NewServeMux()

	mux.Handle("/ws", jRPCServer.Handler())

	addr := os.Getenv("WEBHOST_PORT")

	if addr == "" {
		addr = ":6006"
	}

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  5 * time.Second,
		BaseContext:  func(net.Listener) context.Context { return rootContext },
	}
	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverError <- err
		}
	}()

	select {
	case err := <-serverError:
		log.Printf("HTTP server error: %s", err)
	case <-rootContext.Done():
		log.Printf("shutdown requested (%v); wayting for %s grace period for scripts",
			context.Cause(rootContext), scriptGracePeriod)
		stop()
	}

	log.Println("Shutting down the server...")
	ctx, cancel := context.WithTimeout(rootContext, 15*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %s", err)
	}

	log.Println("Server shut down properly")
}
