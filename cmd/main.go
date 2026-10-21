package main

import (
	"context"
	"encoding/json"
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
	"strings"
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
		return fmt.Errorf("containers setup failed %s: \n%s", err, out)
	}

	res, err := app.InstancesList(ctx, api.InstancesListArgs{})
	if err != nil {
		return fmt.Errorf("failed to fetch installed instances %s", err)
	}

	caddyRunning := false
	caddyInstalled := false
	dawaRunning := false
	dawaInstalled := false
	metricsRunning := false
	metricsInstalled := false

	for _, i := range res.Instances {
		if i.App == "caddy" {
			caddyInstalled = true
			caddyRunning = i.Status == "running"
		}
		if i.App == "dawa" {
			dawaInstalled = true
			dawaRunning = i.Status == "running"
		}
		if i.App == "metrics" {
			metricsInstalled = true
			metricsRunning = i.Status == "running"
		}
	}

	if !caddyRunning {

		if !caddyInstalled {
			portHTTP, _ := json.Marshal(app.ProxyCfg.HTTPPort)
			portHTTPS, _ := json.Marshal(app.ProxyCfg.HTTPSPort)

			if _, err := app.InstanceInstall(ctx, api.InstanceInstallArgs{
				AppName:    "caddy",
				DomainName: app.SystemCfg.DomainName,
				Variables: map[string]json.RawMessage{
					"PORT_HTTP":  portHTTP,
					"PORT_HTTPS": portHTTPS,
				},
			}); err != nil {
				return err
			}
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
	}

	if app.SystemCfg.SkipDawa {
		log.LogInfo("Skipping Dawa initial install")
	} else {

		if !dawaRunning {
			if !dawaInstalled {
				dawaResult, err := app.InstanceInstall(ctx, api.InstanceInstallArgs{
					AppName:    "dawa",
					DomainName: "dawa." + app.SystemCfg.DomainName,
				})
				if err != nil {
					return err
				}
				if dawaResult.JobID != "" {
					if err := pollJobStatus(ctx, app, dawaResult.JobID); err != nil {
						return err
					}
				}

			}

			if _, err := app.InstanceExpose(ctx, api.InstanceExposeArgs{InstanceID: "dawa"}); err != nil {
				return err
			}

			startResult, err := app.InstanceStart(ctx, api.InstanceStartArgs{InstanceID: "dawa"})
			if err != nil {
				return err
			}
			if startResult.JobID != "" {
				if err := pollJobStatus(ctx, app, startResult.JobID); err != nil {
					return err
				}
			}
		}

		if !metricsRunning {
			if !metricsInstalled {
				metricsResult, err := app.InstanceInstall(ctx, api.InstanceInstallArgs{
					AppName: "metrics",
				})
				if err != nil {
					return err
				}

				if metricsResult.JobID != "" {
					if err := pollJobStatus(ctx, app, metricsResult.JobID); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// Containers reach dcommsctl at host.docker.internal, which is the default bridge gateway.
func dockerBridgeGateway(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "docker", "network", "inspect", "bridge",
		"--format", "{{range .IPAM.Config}}{{.Gateway}} {{end}}").Output()
	if err != nil {
		return "", fmt.Errorf("docker network inspect bridge: %w", err)
	}
	for _, field := range strings.Fields(string(out)) {
		if ip := net.ParseIP(field); ip != nil && ip.To4() != nil {
			return ip.String(), nil
		}
	}
	return "", fmt.Errorf("the docker bridge network has no IPv4 gateway")
}

// When bound to loopback, also listen on the docker bridge so Dawa can connect.
func listenForContainers(ctx context.Context, addr string) net.Listener {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return nil
	}

	gateway, err := dockerBridgeGateway(ctx)
	if err != nil {
		log.LogError("containers cannot reach dcommsctl", "err", err)
		return nil
	}

	bridgeAddr := net.JoinHostPort(gateway, port)
	listener, err := net.Listen("tcp4", bridgeAddr)
	if err != nil {
		log.LogError("containers cannot reach dcommsctl", "address", bridgeAddr, "err", err)
		return nil
	}

	log.LogInfo("listening for containers", "address", listener.Addr().String())
	return listener
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

	app := api.NewGlobalState(rootContext)
	app.SystemCfg.SkipDawa = *skipDawa
	app.SystemCfg.BaseDataPath, _ = filepath.Abs(*baseDataPath)
	app.SystemCfg.InstancesDir = filepath.Join(app.SystemCfg.BaseDataPath, "./.instances")
	app.SystemCfg.AppsDir = filepath.Join(app.SystemCfg.BaseDataPath, "./conf_tmpl")
	app.SystemCfg.BackupsDir = filepath.Join(app.SystemCfg.BaseDataPath, "./.backups")
	app.JobsManager.JobsDBpath = filepath.Join(app.SystemCfg.BaseDataPath, "./.jobs_db.json")
	app.JobsManager.JobsPath = filepath.Join(app.SystemCfg.BaseDataPath, "./.jobs")

	// A fresh install has none of the data folders yet.
	for _, dir := range []string{app.SystemCfg.InstancesDir, app.SystemCfg.BackupsDir, app.JobsManager.JobsPath} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			panic(err)
		}
	}

	webDomain := os.Getenv("DCOMMS_WEB_DOMAIN")

	if webDomain == "" {
		webDomain = "localhost"
	}
	app.SystemCfg.DomainName = webDomain

	addr := os.Getenv("WEBHOST_PORT")

	if addr == "" {
		addr = "0.0.0.0:6006"
	}

	proxyHTTPPort := os.Getenv("DCOMMS_PROXY_HTTP_PORT")
	proxyHTTPSPort := os.Getenv("DCOMMS_PROXY_HTTPS_PORT")

	environment := os.Getenv("DCOMMS_ENV")

	if environment != "production" {
		environment = "development"
		if proxyHTTPPort == "" {
			app.ProxyCfg.HTTPPort = 8000
		}
		if proxyHTTPSPort == "" {
			app.ProxyCfg.HTTPSPort = 3443
		}
	} else {

		if proxyHTTPPort == "" {
			app.ProxyCfg.HTTPPort = 80
		}

		if proxyHTTPSPort == "" {
			app.ProxyCfg.HTTPSPort = 443
		}
	}

	app.ProxyCfg.PlainHTTP = (app.SystemCfg.DomainName == "localhost")

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

	listeners := []net.Listener{listener}
	if l := listenForContainers(rootContext, addr); l != nil {
		listeners = append(listeners, l)
	}

	serverError := make(chan error, len(listeners))

	for _, l := range listeners {
		go func() {
			if err := server.Serve(l); !errors.Is(err, http.ErrServerClosed) {
				serverError <- err
			}
		}()
	}

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
