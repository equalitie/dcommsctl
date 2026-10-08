package main

import (
	"context"
	"errors"
	"flag"
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
	"syscall"
	"text/template"
	"time"
)

const scriptGracePeriod = 20 * time.Second

type SystemConfig struct {
	DomainName   string
	SSODomain    string
	BaseDataPath string
	AppsDir      string
	InstancesDir string
	BackupsDir   string
	SkipDawa     bool
}

type InstanceGenericConfig struct {
	SSOIssuerURI string
	InstanceID   string
	ClientID     string
	ClientSecret string
	Domain       string
	OnionDomain  string
	Version      string
}

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

func genericInst(ctx context.Context, appName, instancesDir string, config InstanceGenericConfig) (string, error) {
	pathToApp := path.Join("conf_tmpl", appName)
	pathToInst, err := instanceCreate(instancesDir, appName)
	if err != nil {
		return "", err
	}

	instanceID := filepath.Base(pathToInst)

	config.InstanceID = instanceID

	if err := writeFileTemplate(
		path.Join(pathToApp, "docker-compose.tmpl.yml"),
		path.Join(pathToInst, "docker-compose.yml"),
		&config,
	); err != nil {
		return "", err
	}

	if err := writeFileTemplate(
		path.Join(pathToApp, "env.tmpl"),
		path.Join(pathToInst, ".env"),
		&config,
	); err != nil {
		return "", err
	}

	if err := writeFileTemplate(
		path.Join(pathToApp, "caddy-route.tmpl.json"),
		path.Join(pathToInst, "caddy-route.json"),
		&config,
	); err != nil {
		return "", err
	}

	if err := caddyExportConfig(ctx, instancesDir); err != nil {
		return "", err
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

func writeFileTemplate(tmplPath, targetPath string, data any) error {
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
	cmd := exec.CommandContext(ctx, "./scripts/docker_setup.sh")

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("containers setup failed %s: %s", err, out)
	}

	if _, err := app.rpcInstanceInstall(ctx, InstanceInstallArgs{
		AppName:    "caddy",
		DomainName: app.systemCfg.DomainName,
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
	logInfo("Caddy should be running on port :8000 SSL on port :3443")

	if app.systemCfg.SkipDawa {
		logInfo("Skipping Dawa initial install")
	} else {
		dawaResult, err := app.rpcInstanceInstall(ctx, InstanceInstallArgs{
			AppName:    "dawa",
			DomainName: "dawa." + app.systemCfg.DomainName,
		})
		if err != nil {
			return err
		}
		if err := app.pollJobStatus(ctx, dawaResult.JobID); err != nil {
			return err
		}
	}

	return nil
}

// Installable applications, in catalog order. Also the allowlist for apps/get.
var appIDs = []string{
	"synapse",
	"cryptpad",
	"peertube",
	"mastodon",
}

func main() {
	initLogging()

	rootContext, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	skipDawa := flag.Bool("skip-dawa", false, "Skip initial Dawa installation")
	baseDataPath := flag.String("path", ".", "Path to store all the data (instances, backups, job database)")
	flag.Parse()

	webDomain := os.Getenv("DCOMMS_WEB_DOMAIN")

	if webDomain == "" {
		webDomain = "localhost"
	}

	app := NewGlobalState(rootContext)
	app.systemCfg.DomainName = webDomain
	app.systemCfg.SkipDawa = *skipDawa
	app.systemCfg.BaseDataPath, _ = filepath.Abs(*baseDataPath)
	app.systemCfg.InstancesDir = filepath.Join(app.systemCfg.BaseDataPath, "./.instances")
	app.systemCfg.AppsDir = filepath.Join(app.systemCfg.BaseDataPath, "./conf_tmpl")
	app.systemCfg.BackupsDir = filepath.Join(app.systemCfg.BaseDataPath, "./.backups")
	app.jobsManager.jobsDBpath = filepath.Join(app.systemCfg.BaseDataPath, "./.jobs_db.json")
	app.jobsManager.jobsPath = filepath.Join(app.systemCfg.BaseDataPath, "./.jobs")

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

	jRPCServer.Register("backup/list", app.rpcBackupsList)

	jRPCServer.Register("panic", app.rpcPanic)
	// jRPCServer.Register("container/start", func() {})
	// jRPCServer.Register("container/stop", func() {})

	jRPCServer.reg.SaveSpecs("schema_jrpc.json")
	mux := http.NewServeMux()

	mux.Handle("/ws", jRPCServer.Handler())

	addr := os.Getenv("WEBHOST_PORT")

	if addr == "" {
		addr = "0.0.0.0:6006"
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
	listener, err := net.Listen("tcp4", addr)
	if err != nil {
		logFatal("cannot listen", "address", addr, "err", err)
	}

	logInfo("listening", "address", listener.Addr().String(), "ws_endpoint", listener.Addr().String()+"/ws")

	serverError := make(chan error, 1)

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
