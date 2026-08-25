package main

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
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
	Applications       map[string]ApplicationConfig
}

type BackupRestoreStep struct {
	TmpContainerImage string
	ServiceName       string
	ScriptPath        string
}

type ApplicationConfig struct {
	Name         string
	Started      bool
	ComposePath  string
	BackupSteps  []BackupRestoreStep
	RestoreSteps []BackupRestoreStep
	// TODO: ideally this is the set of files but right know I don't know how to propely handle those
	ConfigFilePath string
	ContainerID    string
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

func targetsSave(app *GlobalState) error {
	// FIXME: since we're using JSON anyway, this should be saved to json
	f, err := os.Create("./targets.csv")
	if err != nil {
		return err
	}

	defer f.Close()
	w := csv.NewWriter(f)
	for id, t := range app.targets {

		backend := "docker"

		if t.ContainerBackend == ContainerBackendPodman {
			backend = "podman"
		}

		applications := []string{}

		if t.MatrixEnabled {
			applications = append(applications, "matrix")
		}
		if t.PeertubeEnabled {
			applications = append(applications, "peertube")
		}
		if t.MastodonEnabled {
			applications = append(applications, "mastodon")
		}

		if len(applications) == 0 {
			continue
		}

		if err := w.Write([]string{strconv.Itoa(id), backend, t.Host, t.Port, t.User, t.Domain, strings.Join(applications, "+")}); err != nil {
			fmt.Println(err)
		}
	}

	w.Flush()

	return nil
}

func targetsLoad(app *GlobalState) error {
	// FIXME: since we're using JSON anyway, this should be loaded from json
	f, err := os.Open("./targets.csv")
	if err != nil {
		return err
	}

	defer f.Close()
	r := csv.NewReader(f)

	recs, err := r.ReadAll()

	for _, rec := range recs {
		id, err := strconv.Atoi(rec[0])
		if err != nil {
			fmt.Println("Error while parsing error in targets.csv", err)
			continue
		}

		backend := rec[1]
		host := rec[2]
		port := rec[3]
		user := rec[4]
		domain := rec[5]
		applications := rec[6]

		appsSplit := strings.Split(applications, "+")

		if len(appsSplit) == 0 {
			fmt.Printf("No application specified for a target %d, ignoring\n", id)
			continue
		}

		containerBackend := ContainerBackendType(ContainerBackendDocker)

		if backend == "podman" {
			containerBackend = ContainerBackendPodman
		}

		t := DeploymentTargetInfo{
			Host:             host,
			Port:             port,
			User:             user,
			Domain:           domain,
			ContainerBackend: containerBackend,
			Remote:           false,
		}

		for _, srv := range appsSplit {
			switch srv {
			case "matrix":
				t.MatrixEnabled = true
			case "mastodon":
				t.MastodonEnabled = true
			case "peertube":
				t.PeertubeEnabled = true
			}
		}

		app.targets[id] = t
	}
	return nil
}

func execCommand(ctx context.Context, t DeploymentTargetInfo, cmd string) error {
	c := exec.Command(cmd)

	_, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error executing command: %s", err)
	}

	return nil
}

func appCreate(ctx context.Context, appID string) (string, error) {
	appFolder := "./.apps"

	pathToApp := path.Join(appFolder, appID)

	folderToCreate := path.Join(pathToApp, "jobs")

	if err := os.MkdirAll(folderToCreate, 0o700); err != nil {
		return "", fmt.Errorf("create app dir: %s", err)
	}

	return pathToApp, nil
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
	targetsLoad(app)

	tmpCtx := context.Background()
	rootContext := context.WithValue(tmpCtx, "app", app)

	// rootContext, stop = signal.NotifyContext(
	// 	rootContext,
	// 	os.Interrupt,
	// 	syscall.SIGTERM,
	// )
	// defer stop()
	app.ctx = rootContext
	app.systemCfg.DcommsConfFilesDir = "./dcomms-docker-compose"
	app.systemCfg.Applications = make(map[string]ApplicationConfig)

	jRPCServer := NewJRPCServer()
	jRPCServer.Logger = log.Default()

	jRPCServer.Register("install", app.rpcCreateApp)
	jRPCServer.Register("start", app.rpcStartApp)
	jRPCServer.Register("stop", app.rpcStartApp)
	jRPCServer.Register("backup", app.rpcStartApp)
	jRPCServer.Register("restore", app.rpcStartApp)

	// FIXME: these are probably not needed anymore
	peerTubeConf, err := initPeertubeConf(rootContext)
	if err != nil {
		panic(err)
	}
	mastodonConf, err := initMastodonConf(rootContext)
	if err != nil {
		panic(err)
	}
	app.systemCfg.Applications[peerTubeConf.Name] = peerTubeConf
	app.systemCfg.Applications[mastodonConf.Name] = mastodonConf

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

	targetsSave(app)

	log.Println("Server shut down properly")
}
