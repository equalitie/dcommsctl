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
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"
)

// some concepts:
//  - target (deployment target) is either VPS or cloud service
//  - application is either docker compose project or docker swarm service

type NetworkRequest struct {
	Name     string
	Internal bool
}

type SystemConfig struct {
	DcommsConfFilesDir string
	ScriptsDir         string
	InstancesDir       string
	BackupsDir         string
}

type BackupRestoreStep struct {
	TmpContainerImage string
	ServiceName       string
	ScriptPath        string
}

type ContainerBackendType int

const (
	ContainerBackendDocker = iota
	ContainerBackendPodman
)

// This is a deployment target info, either cloud or VPS, for now only VPS
type DeploymentTargetInfo struct {
	ID                   int
	Remote               bool
	SSHTunnelEstablished bool

	User   string
	Host   string
	Port   string
	Domain string
	// folder where we store the dcomms repo
	ProjectRoot      string
	ContainerBackend ContainerBackendType
	// do not want to deal with map
	MatrixEnabled   bool
	MauEnabled      bool
	PeertubeEnabled bool
	MastodonEnabled bool
	CenoEnabled     bool
}

type GlobalState struct {
	conn DeploymentTargetInfo
	// int as key for persistent IDs
	targets     map[int]DeploymentTargetInfo
	ctx         context.Context
	systemCfg   SystemConfig
	jobsManager JobManager
}

func NewGlobalState() *GlobalState {
	return &GlobalState{
		targets: make(map[int]DeploymentTargetInfo),
	}
}

func execCommand(ctx context.Context, t DeploymentTargetInfo, cmd string) error {
	c := exec.Command(cmd)

	_, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error executing command: %s", err)
	}

	return nil
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
	stop := make(chan os.Signal, 1)
	app := NewGlobalState()

	tmpCtx := context.Background()
	rootContext := context.WithValue(tmpCtx, "app", app)

	app.ctx = rootContext
	app.systemCfg.DcommsConfFilesDir, _ = filepath.Abs("./dcomms-docker-compose")
	app.systemCfg.InstancesDir, _ = filepath.Abs("./.instances")
	app.systemCfg.ScriptsDir, _ = filepath.Abs("./scripts")
	app.systemCfg.BackupsDir, _ = filepath.Abs("./.backups")

	jRPCServer := NewJRPCServer()
	jRPCServer.Logger = log.Default()

	jRPCServer.Register("inst/install", app.rpcInstanceInstall)
	jRPCServer.Register("inst/uninstall", app.rpcInstanceInstall)
	jRPCServer.Register("inst/start", app.rpcInstanceStart)
	jRPCServer.Register("inst/stop", app.rpcInstanceStop)
	jRPCServer.Register("inst/backup", app.rpcInstanceBackup)
	jRPCServer.Register("inst/restore", app.rpcInstanceRestore)

	signal.Notify(stop, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	mux := http.NewServeMux()

	mux.Handle("/ws", jRPCServer.Handler())

	addr := os.Getenv("WEBHOST_PORT")

	if addr == "" {
		addr = ":8080"
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
	case sig := <-stop:
		log.Printf("Got shutdown signal: %s", sig)
	}

	log.Println("Shutting down the server...")
	ctx, cancel := context.WithTimeout(rootContext, 15*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %s", err)
	}

	log.Println("Server shut down properly")
}
