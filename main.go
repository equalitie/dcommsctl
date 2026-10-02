package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	"text/template"
	"time"
)

const scriptGracePeriod = 20 * time.Second

type SystemConfig struct {
	SSODomain    string
	AppsDir      string
	InstancesDir string
	BackupsDir   string
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

func NewGlobalState(ctx context.Context) *GlobalState {
	return &GlobalState{
		jobsManager: NewJobManager(ctx),
	}
}

func instanceGenID() string {
	return instanceNames[rand.IntN(len(instanceNames))]
}

func instanceCreate(instancesDir string, instanceName string) (string, error) {
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

func portRandom() (port int) {
	for {
		port = rand.IntN(9999-1024) + 1024

		if portIsBindable(port) {
			break
		}
	}
	return port
}

func portIsBindable(port int) bool {
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

func executeTemplate(tmplPath, targetPath string, data any) error {
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return err
	}

	composeFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	if err := tmpl.Execute(composeFile, &data); err != nil {
		return err
	}

	return nil
}

func (app *GlobalState) pollJobStatus(ctx context.Context, jobID string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("cancelled")
		case <-ticker.C:
			jobStatusRes, err := app.rpcJobStatus(ctx, JobStatusArgs{JobID: JobID(jobID)})
			if err != nil {
				return err
			}

			if jobStatusRes.State == JobDone {
				return nil
			}

			if jobStatusRes.State == JobFailed {
				return fmt.Errorf("job failed: %s", jobStatusRes.Error)
			}
		}
	}
}

func (app *GlobalState) initApps(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "./docker_setup.sh")

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("containers setup failed %s: %s", err, out)
	}

	if _, err := app.rpcInstanceInstall(ctx, InstanceInstallArgs{
		AppName:    "caddy",
		DomainName: "localhost",
	}); err != nil {
		return err
	}

	caddyStartResult, err := app.rpcInstanceStart(ctx, InstanceStartArgs{
		InstanceID: "caddy",
	})
	if err != nil {
		return err
	}

	if err := app.pollJobStatus(ctx, caddyStartResult.JobID); err != nil {
		return err
	}

	// FIXME: get the actual port
	logInfo("Caddy should be running on port :8000")

	dawaResult, err := app.rpcInstanceInstall(ctx, InstanceInstallArgs{
		AppName:    "dawa",
		DomainName: "dawa.localhost",
	})
	if err != nil {
		return err
	}
	if err := app.pollJobStatus(ctx, dawaResult.JobID); err != nil {
		return err
	}

	return nil
}

func main() {
	initLogging()

	serverError := make(chan error, 1)

	rootContext, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	app := NewGlobalState(rootContext)

	app.jobsManager.jobsDBpath, _ = filepath.Abs("./.jobs_db.json")
	app.jobsManager.jobsPath, _ = filepath.Abs("./.jobs")

	app.systemCfg.InstancesDir, _ = filepath.Abs("./.instances")
	app.systemCfg.AppsDir, _ = filepath.Abs("./conf_tmpl")
	app.systemCfg.BackupsDir, _ = filepath.Abs("./.backups")

	// TODO: debug this
	if err := app.initApps(rootContext); err != nil {
		panic(err)
	}

	if err := app.jobsManager.loadJobs(); err != nil {
		panic(err)
	}

	jRPCServer := NewJRPCServer()
	jRPCServer.Logger = slog.Default()

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

	jRPCServer.Register("inst/expose", app.rpcInstanceExpose)
	jRPCServer.Register("inst/hide", rpcInstanceHide)

	jRPCServer.Register("inst/backup", app.rpcInstanceBackup)
	jRPCServer.Register("inst/restore", app.rpcInstanceRestore)
	jRPCServer.Register("inst/upgrade", app.rpcInstanceUpgrade)

	jRPCServer.Register("panic", app.rpcPanic)
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

	// Bind explicitly rather than letting ListenAndServe do it, so the address we
	// actually got (port 0 or a taken port would otherwise be guesswork) can be
	// logged before serving starts.
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logFatal("cannot listen", "address", addr, "err", err)
	}

	logInfo("listening", "address", listener.Addr().String(), "ws_endpoint", listener.Addr().String()+"/ws")

	go func() {
		if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
			serverError <- err
		}
	}()

	select {
	case err := <-serverError:
		logError("HTTP server error", "err", err)
	case <-rootContext.Done():
		logInfo("shutdown requested",
			"cause", context.Cause(rootContext), "script_grace_period", scriptGracePeriod)
		stop()
	}

	logInfo("shutting down the server")
	ctx, cancel := context.WithTimeout(rootContext, 15*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logError("server shutdown error", "err", err)
	}

	logInfo("server shut down properly")
}
