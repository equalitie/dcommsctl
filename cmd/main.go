package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"eq/dcommsctl/api"
	"eq/dcommsctl/internal/jobs"
	"eq/dcommsctl/internal/log"
	"eq/dcommsctl/internal/rpc"
)

func pollJobStatus(ctx context.Context, app *api.GlobalState, jobID string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("cancelled")
		case <-ticker.C:
			jobStatusRes, err := app.JobStatus(ctx, api.JobStatusArgs{JobID: jobs.JobID(jobID)})
			if err != nil {
				return err
			}

			if jobStatusRes.State == jobs.JobDone {
				return nil
			}

			if jobStatusRes.State == jobs.JobFailed {
				return fmt.Errorf("job failed: %s", jobStatusRes.Error)
			}
		}
	}
}

func initApps(ctx context.Context, app *api.GlobalState) error {
	cmd := exec.CommandContext(ctx, "./scripts/docker_setup.sh")

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("containers setup failed %s: %s", err, out)
	}

	if _, err := app.InstanceInstall(ctx, api.InstanceInstallArgs{
		AppName:    "caddy",
		DomainName: app.SystemCfg.DomainName,
	}); err != nil {
		return err
	}

	caddyStartResult, err := app.InstanceStart(ctx, api.InstanceStartArgs{
		InstanceID: "caddy",
	})
	if err != nil {
		return err
	}

	if err := pollJobStatus(ctx, app, caddyStartResult.JobID); err != nil {
		return err
	}

	// FIXME: get the actual port
	log.LogInfo("Caddy should be running on port :8000 SSL on port :3443")

	if app.SystemCfg.SkipDawa {
		log.LogInfo("Skipping Dawa initial install")
	} else {
		dawaResult, err := app.InstanceInstall(ctx, api.InstanceInstallArgs{
			AppName:    "dawa",
			DomainName: "dawa." + app.SystemCfg.DomainName,
		})
		if err != nil {
			return err
		}
		if err := pollJobStatus(ctx, app, dawaResult.JobID); err != nil {
			return err
		}
	}

	return nil
}

// Installable applications, in catalog order. Also the allowlist for apps/get.
func main() {
	log.InitLogging()

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

	app := api.NewGlobalState(rootContext)
	app.SystemCfg.DomainName = webDomain
	app.SystemCfg.SkipDawa = *skipDawa
	app.SystemCfg.BaseDataPath, _ = filepath.Abs(*baseDataPath)
	app.SystemCfg.InstancesDir = filepath.Join(app.SystemCfg.BaseDataPath, "./.instances")
	app.SystemCfg.AppsDir = filepath.Join(app.SystemCfg.BaseDataPath, "./conf_tmpl")
	app.SystemCfg.BackupsDir = filepath.Join(app.SystemCfg.BaseDataPath, "./.backups")
	app.JobsManager.JobsDBpath = filepath.Join(app.SystemCfg.BaseDataPath, "./.jobs_db.json")
	app.JobsManager.JobsPath = filepath.Join(app.SystemCfg.BaseDataPath, "./.jobs")

	if err := initApps(rootContext, app); err != nil {
		panic(err)
	}

	if err := app.JobsManager.LoadJobs(); err != nil {
		panic(err)
	}

	jRPCServer := rpc.NewJRPCServer()
	jRPCServer.Logger = slog.Default()

	jRPCServer.Register("app/list", api.ApplicationsList)
	jRPCServer.Register("app/get", api.ApplicationGet)

	jRPCServer.Register("job/log", app.JobLog)
	jRPCServer.Register("job/status", app.JobStatus)
	jRPCServer.Register("job/list", app.JobsList)

	jRPCServer.Register("inst/list", app.InstancesList)
	jRPCServer.Register("inst/install", app.InstanceInstall)
	jRPCServer.Register("inst/uninstall", app.InstanceUninstall)
	jRPCServer.Register("inst/start", app.InstanceStart)
	jRPCServer.Register("inst/stop", app.InstanceStop)

	jRPCServer.Register("inst/expose", app.InstanceExpose)
	jRPCServer.Register("inst/hide", api.InstanceHide)

	jRPCServer.Register("inst/backup", app.InstanceBackup)
	jRPCServer.Register("inst/restore", app.InstanceRestore)
	jRPCServer.Register("inst/upgrade", app.InstanceUpgrade)

	jRPCServer.Register("backup/list", app.BackupsList)

	jRPCServer.Register("panic", app.Panic)

	jRPCServer.Reg.SaveSpecs("schema_jrpc.json")
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
		log.LogFatal("cannot listen", "address", addr, "err", err)
	}

	log.LogInfo("listening", "address", listener.Addr().String(), "ws_endpoint", listener.Addr().String()+"/ws")

	serverError := make(chan error, 1)

	go func() {
		if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
			serverError <- err
		}
	}()

	select {
	case err := <-serverError:
		log.LogError("HTTP server error", "err", err)
	case <-rootContext.Done():
		log.LogInfo("shutdown requested",
			"cause", context.Cause(rootContext), "script_grace_period", jobs.ScriptGracePeriod)
		stop()
	}

	log.LogInfo("shutting down the server")
	ctx, cancel := context.WithTimeout(rootContext, 15*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.LogError("server shutdown error", "err", err)
	}

	log.LogInfo("server shut down properly")
}
