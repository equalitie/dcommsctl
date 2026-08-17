package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
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
	targets   map[int]DeploymentTargetInfo
	ctx       context.Context
	systemCfg SystemConfig
}

func NewGlobalState() *GlobalState {
	return &GlobalState{
		targets: make(map[int]DeploymentTargetInfo),
	}
}

func sftpCopy(client *sftp.Client, srcPath, dstPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("Failed to read: %s", err)
	}
	defer srcFile.Close()

	dstFile, err := client.Create(dstPath)
	if err != nil {
		return fmt.Errorf("Failed to create: %s", err)
	}
	defer dstFile.Close()

	if _, err := dstFile.ReadFrom(srcFile); err != nil {
		return fmt.Errorf("Failed to write: %s", err)
	}

	return nil
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

func copyFolder(client *sftp.Client, src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(src, path)
		if relPath == "." {
			return nil
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return client.MkdirAll(dstPath)
		}

		if err := client.MkdirAll(filepath.Dir(dstPath)); err != nil {
			return err
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		dstFile, err := client.Create(dstPath)
		if err != nil {
			return fmt.Errorf("Create err: %s", err)
		}
		defer dstFile.Close()
		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}

func sshCopyProject(target DeploymentTargetInfo) error {
	fmt.Println("Starting copy")
	pathToDcomms := os.Getenv("DCOMMS_GIT_REPO_PATH")

	socket := os.Getenv("SSH_AUTH_SOCK")

	conn, err := net.Dial("unix", socket)
	if err != nil {
		return fmt.Errorf("unable to connect to ssh-agent socket: %v", err)
	}
	defer conn.Close()

	agentClient := agent.NewClient(conn)

	config := &ssh.ClientConfig{
		User: target.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", target.Host+":"+target.Port, config)
	if err != nil {
		return err
	}
	defer client.Close()

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("Failed to init sftp connection: %s", err)
	}
	defer sftpClient.Close()

	if err := copyFolder(sftpClient, pathToDcomms, "/home/"+target.User+"/dcomms/dcomms-docker-compose"); err != nil {
		return fmt.Errorf("Fail to copy files: %s", err)
	}

	fmt.Println("Ending copy")
	return nil
}

func sshTunnel(ctx context.Context, t DeploymentTargetInfo, localAddr string) error {
	socket := os.Getenv("SSH_AUTH_SOCK")

	conn, err := net.Dial("unix", socket)
	if err != nil {
		return fmt.Errorf("Unable to connect to ssh-agent socket: %s", err)
	}
	defer conn.Close()

	agentClient := agent.NewClient(conn)

	config := &ssh.ClientConfig{
		User: t.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	remoteSSHClient, err := ssh.Dial("tcp", t.Host+":"+t.Port, config)
	if err != nil {
		return fmt.Errorf("Unable to dial ssh: %v", err)
	}
	// FIXME: when are we closing this?
	// defer remoteSSHClient.Close()
	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("Failed to listen on local address: %s", err)
	}

	go func() {
		idleTimer := time.NewTimer(10 * time.Second)
		idleTimer.Stop()

		resetIdleTimer := func() {
			if !idleTimer.Stop() {
				<-idleTimer.C
			}
			idleTimer.Reset(10 * time.Second)
		}

		go func() {
			<-idleTimer.C
			fmt.Println("SSH tunnel idle. Closing...")
			remoteSSHClient.Close()
			listener.Close()
		}()

		for {
			localConn, err := listener.Accept()
			if err != nil {
				log.Printf("Failed to accept local connection: %s", err)
				continue
			}

			remoteConn, err := remoteSSHClient.Dial("unix", "/var/run/docker.sock")
			if err != nil {
				log.Printf("Failed to dial remote Docker socket: %s", err)
				localConn.Close()
				continue
			}

			go func() {
				defer localConn.Close()
				defer remoteConn.Close()
				io.Copy(localConn, remoteConn)
				resetIdleTimer()
			}()

			go func() {
				defer localConn.Close()
				defer remoteConn.Close()
				io.Copy(remoteConn, localConn)
				resetIdleTimer()
			}()
		}
	}()

	if err := sshExecCommand(ctx, t, `docker ps -a --format "table {{.Names}}\t{{.Status}}"`); err != nil {
		return fmt.Errorf("Failed to get data on compose containers: %s", err)
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

// FIXME: we need to get result out, either write to a buffer or something
func sshExecCommand(ctx context.Context, t DeploymentTargetInfo, cmd string) error {
	socket := os.Getenv("SSH_AUTH_SOCK")

	conn, err := net.Dial("unix", socket)
	if err != nil {
		return fmt.Errorf("unable to connect to ssh-agent socket: %v", err)
	}
	defer conn.Close()

	agentClient := agent.NewClient(conn)

	config := &ssh.ClientConfig{
		User: t.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", t.Host+":"+t.Port, config)
	if err != nil {
		return fmt.Errorf("unable to dial ssh: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("unable to create ssh session: %s", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)

	fmt.Println("---    Running ssh command    ---")
	fmt.Println(string(out))
	fmt.Println("--- End of ssh command output ---")

	if err != nil {
		return err
	}

	return nil
}

func remoteOsStatus(target DeploymentTargetInfo) []byte {
	var buf bytes.Buffer

	commands := []string{
		"grep MemTotal /proc/meminfo |tr -s ' ' |cut -d ' ' -f2",
		"free -h",
		"df -h",
		"uptime",
	}

	for _, cmd := range commands {
		if err := sshExecCommand(context.TODO(), target, cmd); err != nil {
			return nil
		}
	}

	return buf.Bytes()
}

func getEnv(app *GlobalState, t DeploymentTargetInfo, appID string) ([]byte, error) {
	appCfg, found := app.systemCfg.Applications[appID]

	if !found {
		return nil, fmt.Errorf("application is not registered %s", appID)
	}
	var stdoutBuf bytes.Buffer
	var out bytes.Buffer

	cmd := fmt.Sprintf("podman-compose -f config %s --services", appCfg.ComposePath)

	if t.Remote {
		if err := sshExecCommand(app.ctx, t, cmd); err != nil {
			return nil, err
		}
	} else {
		if err := execCommand(app.ctx, t, cmd); err != nil {
			return nil, err
		}
	}

	services := bytes.SplitSeq(stdoutBuf.Bytes(), []byte("\n"))

	for serviceName := range services {
		name := string(serviceName)
		if name == "" {
			continue
		}

		cmd := fmt.Sprintf("podman-compose -d -f %s exec %s", path.Join("/home", t.User, "dcomms", "setup", "conf", "compose", appCfg.ComposePath), "env")

		if t.Remote {
			if err := sshExecCommand(app.ctx, t, cmd); err != nil {
				return nil, err
			}
		} else {
			if err := execCommand(app.ctx, t, cmd); err != nil {
				return nil, err
			}
		}

		out.WriteString(name)
		out.WriteString("\n==========\n\n")
	}

	return out.Bytes(), nil
}

func getTargetFromRequest(app *GlobalState, r *http.Request) (DeploymentTargetInfo, error) {
	target, err := strconv.Atoi(r.PathValue("target"))
	if err != nil {
		return DeploymentTargetInfo{}, fmt.Errorf("Failed to parse target id: %s", err)
	}

	t, found := app.targets[target]

	if !found {
		return DeploymentTargetInfo{}, fmt.Errorf("Target not found: %d", target)
	}

	return t, nil
}

func getApplicationFromRequest(app *GlobalState, r *http.Request) (ApplicationConfig, error) {
	applicationID := r.PathValue("application")

	if s, found := app.systemCfg.Applications[applicationID]; found {
		return s, nil
	} else {
		return ApplicationConfig{}, fmt.Errorf("application not found: %s", applicationID)
	}
}

func main() {
	serverError := make(chan error, 1)
	stop := make(chan os.Signal, 1)
	app := NewGlobalState()
	targetsLoad(app)

	tmpCtx := context.Background()
	rootContext := context.WithValue(tmpCtx, "app", app)

	app.ctx = rootContext
	app.systemCfg.DcommsConfFilesDir = "./dcomms-docker-compose"
	app.systemCfg.Applications = make(map[string]ApplicationConfig)

	// FIXME: don't like panic
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
	fileServer := http.FileServer(http.Dir("./static"))

	mux := http.NewServeMux()

	mux.Handle("GET /", fileServer)
	mux.HandleFunc("GET /target/{target}/info", httpGetInfo)
	mux.HandleFunc("POST /target/create", httpPostTargetCreate)
	mux.HandleFunc("GET /target/list", httpGetTargets)
	mux.HandleFunc("POST /target/{target}/update", httpPostTargetUpdate)
	mux.HandleFunc("GET /target/{target}/env/{id}", httpGetEnv)
	mux.HandleFunc("POST /target/{target}/install", httpPostRunInstallScript)
	mux.HandleFunc("POST /target/{target}/application/add/{application}", httpPostAddApplication)
	mux.HandleFunc("POST /target/{target}/backup/{application}", httpPostRunBackupScript)
	mux.HandleFunc("POST /target/{target}/restore/{application}", httpPostRunRestoreScript)
	mux.HandleFunc("POST /target/{target}/start", httpPostRunStartupScript)
	mux.HandleFunc("POST /target/{target}/stop", httpPostRunStopScript)
	mux.HandleFunc("POST /target/{target}/check", httpPostRunCheckScript)
	mux.HandleFunc("POST /target/{target}/panic/", httpPostPanic)

	addr := os.Getenv("WEBHOST_PORT")

	if addr == "" {
		addr = ":8080"
	}

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
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
