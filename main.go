package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/csv"
	"encoding/json"
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

	"github.com/moby/moby/client"
	"github.com/pkg/sftp"
	"go.podman.io/podman/v6/pkg/bindings"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	// "go.podman.io/podman/v6/pkg/bindings/images"
	// "go.podman.io/podman/v6/pkg/specgen"
	// "github.com/gorilla/websocket"
)

// NOTE(max): this is currently the main implementation file
//   everything here is considered important for implementation
//   everything else that I worked on as ideas that are not used right now moved to other go files around
//   some of these files are compiled into the app some don't

// some concepts:
//  - target (deployment target) is either VPS or cloud service
//  - service is either docker compose unit or docker swarm service

var (
	//go:embed debian_setup.sh
	installScriptContents []byte
	//go:embed debian_check.sh
	checkOsScriptContents []byte
)

type NetworkRequest struct {
	Name     string
	Internal bool
}

type SystemConfig struct {
	Peertube bool
	Services map[string]ServiceConfig
}

type ServiceConfigFile struct {
	Name string
	Path string
}

type ServiceConfig struct {
	Name        string
	Started     bool
	ComposePath string
	ConfigFiles []ServiceConfigFile
	ContainerID string
}

type ContainerBackendType int

const (
	ContainerBackendDocker = iota
	ContainerBackendPodman
)

// This is a deployment target info, either cloud or VPS, for now only VPS
type DeploymentTargetInfo struct {
	ID     int
	User   string
	Host   string
	Port   string
	Domain string
	// folder where we store the dcomms repo
	ProjectRoot      string
	ContainerBackend ContainerBackendType
	MastodonEnabled  bool
	MatrixEnabled    bool
	CenoEnabled      bool
	PeertubeEnabled  bool
	MauEnabled       bool

	SSHTunnelEstablished bool
}

type Application struct {
	conn DeploymentTargetInfo
	// int as key for persistent IDs
	targets   map[int]DeploymentTargetInfo
	ctx       context.Context
	systemCfg SystemConfig
}

func NewApplication() Application {
	return Application{
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

func targetsSave(app Application) error {
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

		services := []string{}
		if t.MatrixEnabled {
			services = append(services, "matrix")
		}
		if t.CenoEnabled {
			services = append(services, "ceno")
		}
		if t.MauEnabled {
			services = append(services, "mau")
		}
		if t.MastodonEnabled {
			services = append(services, "mastodon")
		}
		if t.PeertubeEnabled {
			services = append(services, "peertube")
		}

		if len(services) == 0 {
			continue
		}

		if err := w.Write([]string{strconv.Itoa(id), backend, t.Host, t.Port, t.User, t.Domain, strings.Join(services, "+")}); err != nil {
			fmt.Println(err)
		}
	}

	w.Flush()

	return nil
}

func targetsLoad(app Application) error {
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
		services := rec[6]

		srvsSplit := strings.Split(services, "+")

		if len(srvsSplit) == 0 {
			fmt.Printf("No services specified for a target %d, ignoring\n", id)
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
		}

		for _, srv := range srvsSplit {
			if srv == "matrix" {
				t.MatrixEnabled = true
			}
			if srv == "ceno" {
				t.CenoEnabled = true
			}
			if srv == "mau" {
				t.MauEnabled = true
			}
			if srv == "mastodon" {
				t.MastodonEnabled = true
			}
			if srv == "peertube" {
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

	if err := copyFolder(sftpClient, pathToDcomms, "/home/"+target.User+"/dcomms/setup"); err != nil {
		return fmt.Errorf("Fail to copy files: %s", err)
	}

	fmt.Println("Ending copy")
	return nil
}

func sshDockerSend(t DeploymentTargetInfo, localAddr string) error {
	if !t.SSHTunnelEstablished {
		if err := sshTunnel(t, localAddr); err != nil {
			return err
		}
	}

	containerClient, err := client.New(
		client.FromEnv,
		client.WithHost("tcp://"+localAddr),
	)

	if err != nil {
		return fmt.Errorf("Failed to create docker client: %s", err)
	}
	defer containerClient.Close()

	ctx := context.TODO()

	pingResult, err := containerClient.Ping(ctx, client.PingOptions{})

	if err != nil {
		return fmt.Errorf("Failed to ping docker client: %s", err)
	}

	fmt.Println("Docker API version", pingResult.APIVersion)

	result, err := containerClient.ContainerList(context.Background(), client.ContainerListOptions{
		All: true,
	})
	if err != nil {
		return fmt.Errorf("Failed to list all docker containers: %s", err)
	}

	fmt.Printf("%s  %-22s  %s\n", "ID", "STATUS", "IMAGE")
	for _, ctr := range result.Items {
		// FIXME: just testing execution of commands, no idea for now how to call it directly on container
		if strings.HasPrefix(ctr.Image, "matrixdotorg/synapse:") {
			createResp, err := containerClient.ExecCreate(ctx, ctr.ID, client.ExecCreateOptions{
				Cmd: []string{
					"sh", "-c",
					// FIXME: gen password and feed it from stdin
					"register_new_matrix_user -u dcomms-admin -p test --admin -c /data/homeserver.yaml",
				},
				AttachStdout: true,
				AttachStderr: true,
			})

			if err != nil {
				fmt.Println("Failed to create exec command on container:", err)
				continue
			}

			attachResp, err := containerClient.ExecAttach(ctx, createResp.ID, client.ExecAttachOptions{})
			if err != nil {
				log.Fatalf("Failed to attach to exec: %v", err)
			}
			defer attachResp.Close()
			var stdoutBuf bytes.Buffer
			_, err = io.Copy(&stdoutBuf, attachResp.Reader)
			if err != nil {
				log.Fatalf("Failed to read output: %v", err)
			}

			fmt.Println("--- Running command on container ---")
			fmt.Println(stdoutBuf.String())
			fmt.Println("---     End of command output    ---")

		}
		fmt.Printf("%s  %-22s  %s\n", ctr.ID, ctr.Status, ctr.Image)
	}
	return nil
}

func sshTunnel(t DeploymentTargetInfo, localAddr string) error {
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

	return nil
}

// FIXME: we need to get result out, either write to a buffer or something
func sshExecCommand(c DeploymentTargetInfo, cmd string) error {
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
		if err := sshExecCommand(target, cmd); err != nil {
			return nil
		}
	}

	return buf.Bytes()
}

func remoteServiceDown(app Application, target DeploymentTargetInfo, serviceID string) error {
	serviceCfg, found := app.systemCfg.Services[serviceID]

	if !found {
		return fmt.Errorf("Service is not registered %s", serviceID)
	}

	cmd := fmt.Sprintf("podman-compose -f %s down", path.Join("/home", target.User, "dcomms", "setup", "conf", "compose", serviceCfg.ComposePath))

	if err := sshExecCommand(target, cmd); err != nil {
		return err
	}

	return nil
}

func remoteServiceUp(app Application, target DeploymentTargetInfo, serviceID string) error {
	serviceCfg, found := app.systemCfg.Services[serviceID]

	if !found {
		return fmt.Errorf("Service is not registered %s", serviceID)
	}

	cmd := fmt.Sprintf("podman-compose -d -f %s up", path.Join("/home", target.User, "dcomms", "setup", "conf", "compose", serviceCfg.ComposePath))

	if err := sshExecCommand(target, cmd); err != nil {
		return err
	}

	return nil
}

func getEnv(app Application, target DeploymentTargetInfo, serviceID string) ([]byte, error) {
	serviceCfg, found := app.systemCfg.Services[serviceID]

	if !found {
		return nil, fmt.Errorf("Service is not registered %s", serviceID)
	}
	var stdoutBuf bytes.Buffer
	var out bytes.Buffer

	err := sshExecCommand(target, fmt.Sprintf("podman-compose -f config %s --services", serviceCfg.ComposePath))

	if err != nil {
		return nil, err
	}

	services := bytes.SplitSeq(stdoutBuf.Bytes(), []byte("\n"))

	for serviceName := range services {
		name := string(serviceName)
		if name == "" {
			continue
		}
		// TODO: write this to `out`
		cmd := fmt.Sprintf("podman-compose -d -f %s exec %s", path.Join("/home", target.User, "dcomms", "setup", "conf", "compose", serviceCfg.ComposePath), "env")

		if err := sshExecCommand(target, cmd); err != nil {
			return nil, err
		}

		out.WriteString(name)
		out.WriteString("\n==========\n\n")
	}

	return out.Bytes(), nil
}

func getTargetFromRequest(app Application, r *http.Request) (DeploymentTargetInfo, error) {
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

func httpGetInfo(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(Application)
	t, err := getTargetFromRequest(app, r)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.Write(remoteOsStatus(t))
}

func httpPostServiceStart(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(Application)
	serviceID := r.PathValue("id")
	t, err := getTargetFromRequest(app, r)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	remoteServiceUp(app, t, serviceID)
}

func httpPostServiceStop(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(Application)
	serviceID := r.PathValue("id")
	t, err := getTargetFromRequest(app, r)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	remoteServiceDown(app, t, serviceID)
}

func httpPostPanic(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(Application)
	_, err := getTargetFromRequest(app, r)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	// TODO: remove all volumes
}

func postTestInstallation(app Application, w http.ResponseWriter, r *http.Request) {
	// TODO: implement a check script to see if we're ready to install Dcomms on deployment target
}

func httpGetTargets(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(Application)
	out, err := json.Marshal(app.targets)
	if err != nil {
		fmt.Println("Failed to marshal targets into json:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(out)
}

func httpPostTargetCreate(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(Application)
	maxId := 0

	// for now we just pick the biggest ID
	for id := range app.targets {
		if id > maxId {
			maxId = id
		}
	}

	backend := r.FormValue("backend")
	containerBackend := ContainerBackendType(ContainerBackendDocker)

	if backend == "podman" {
		containerBackend = ContainerBackendPodman
	}

	// FIXME: properly process form values
	t := DeploymentTargetInfo{
		ID:               maxId + 1,
		Host:             r.FormValue("host"),
		User:             r.FormValue("username"),
		Port:             r.FormValue("port"),
		Domain:           r.FormValue("domainname"),
		MatrixEnabled:    r.FormValue("matrix_on") == "on",
		MauEnabled:       r.FormValue("mau_on") == "on",
		CenoEnabled:      r.FormValue("ceno_on") == "on",
		PeertubeEnabled:  r.FormValue("peertube_on") == "on",
		MastodonEnabled:  r.FormValue("mastodon_on") == "on",
		ContainerBackend: containerBackend,
	}

	if t.MauEnabled && !t.MatrixEnabled {
		t.MauEnabled = false
	}

	app.targets[maxId+1] = t

	targetsSave(app)

	fmt.Println("Target created. ID:", maxId+1)

	out, err := json.Marshal(t)

	if err != nil {
		fmt.Println("Failed to marshal target into json:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(out)
}

func httpPostTargetUpdate(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(Application)
	t, err := getTargetFromRequest(app, r)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	// FIXME: properly process form values
	app.targets[t.ID] = DeploymentTargetInfo{
		Host:            r.FormValue("host"),
		User:            r.FormValue("username"),
		Port:            r.FormValue("port"),
		Domain:          r.FormValue("domainname"),
		MatrixEnabled:   r.FormValue("matrix_on") == "on",
		MauEnabled:      r.FormValue("mau_on") == "on",
		CenoEnabled:     r.FormValue("ceno_on") == "on",
		PeertubeEnabled: r.FormValue("peertube_on") == "on",
		MastodonEnabled: r.FormValue("mastodon_on") == "on",
	}

	targetsSave(app)

	fmt.Println("Target updated. ID:", t.ID)
}

func httpPostRunStartupScript(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(Application)
	t, err := getTargetFromRequest(app, r)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	pathToDComms := "/home/" + t.User + "/dcomms/setup"
	pathToDCommsRun := pathToDComms + "/run.sh"
	pathToDCommsStop := pathToDComms + "/stop.sh"

	// FIXME: we probably should not run this, but we do for now
	_ = sshExecCommand(t, `cd `+pathToDComms+` && PATH=$PATH:~/.local/bin `+pathToDCommsStop)

	err = sshExecCommand(t, `cd `+pathToDComms+` && PATH=$PATH:~/.local/bin `+pathToDCommsRun)
	if err != nil {
		fmt.Println("Failed to exec run.sh script:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func httpPostRunCheckScript(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(Application)
	t, err := getTargetFromRequest(app, r)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	// TODO: pick a new port for different target
	if err := sshDockerSend(t, "localhost:3000"); err != nil {
		t.SSHTunnelEstablished = false
		fmt.Println("Failed to tunnel to docker", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Tunnel failed: %s", err)
		return
	}

	t.SSHTunnelEstablished = true

	app.targets[t.ID] = t
}

func httpPostRunStopScript(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(Application)
	t, err := getTargetFromRequest(app, r)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	pathToDComms := "/home/" + t.User + "/dcomms/setup"
	pathToDCommsStop := pathToDComms + "/stop.sh"

	err = sshExecCommand(t, `cd `+pathToDComms+` && PATH=$PATH:~/.local/bin `+pathToDCommsStop)
	if err != nil {
		fmt.Println("Failed to exec stop.sh script:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func httpPostRunInstallScript(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(Application)
	t, err := getTargetFromRequest(app, r)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	choices := []string{}

	if t.MatrixEnabled {
		choices = append(choices, "1")
	}
	if t.CenoEnabled {
		choices = append(choices, "2")
	}
	if t.MauEnabled {
		choices = append(choices, "3")
	}
	if t.MastodonEnabled {
		choices = append(choices, "4")
	}
	if t.PeertubeEnabled {
		choices = append(choices, "5")
	}

	if len(choices) == 0 {
		fmt.Println("Install ran without any services selected")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("No services selected"))
		return
	}

	cmd := exec.Command("ssh", "-o", "ControlMaster=no", "-o", "ControlPath=none",  t.User+"@"+t.Host, "bash -c", string(installScriptContents))
	// FIXME: we need a unified output buffer we're sending back to the client
	// TODO: we need to also stream this, not wait for the whole output and probably leave some markers in output to check for progress
	output, err := cmd.CombinedOutput()
	fmt.Println("--- Running pre-req install script ---")

	fmt.Println(string(output))
	if err != nil {
		fmt.Println(t.Host, t.User, t.Port)
		fmt.Println("Error running ssh to a VPS", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	fmt.Println("--- End of pre-req install script  ---")

	if err := sshCopyProject(t); err != nil {
		fmt.Println("Failed to copy project files:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	pathToDComms := "/home/" + t.User + "/dcomms/setup"
	pathToDCommsInstall := pathToDComms + "/install.sh"

	if t.ContainerBackend == ContainerBackendPodman {
		pathToDCommsInstall = pathToDComms + "/install_podman.sh"
	}

	// FIXME: temporary PATH setting
	err = sshExecCommand(t, fmt.Sprintf(`cd %s && chmod +x %s && PATH=$PATH:~/.local/bin DWEB_DOMAIN=%s CHOICES="%s" %s`, pathToDComms, pathToDCommsInstall, t.Domain, strings.Join(choices, " "), pathToDCommsInstall))
	if err != nil {
		fmt.Println("Failed to exec install script:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func httpGetEnv(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(Application)
	serviceID := r.PathValue("id")

	t, err := getTargetFromRequest(app, r)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	env, err := getEnv(app, t, serviceID)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Get env variables problem", err)
		return
	}

	w.Write(env)
}

func podmanConnect(rootContext context.Context) context.Context {
	podmanSocket := os.Getenv("PODMAN_SOCKET")

	if podmanSocket == "" {
		log.Fatalln("You need to set PODMAN_SOCKET to a path to socket file for running podman. See run.sh for example")
	}

	ctx, err := bindings.NewConnection(rootContext, "unix://"+podmanSocket)
	if err != nil {
		log.Fatalln(err)
		return nil
	}

	return ctx
}

func main() {
	serverError := make(chan error, 1)
	stop := make(chan os.Signal, 1)
	app := NewApplication()
	targetsLoad(app)

	tmpCtx := context.Background()
	rootContext := context.WithValue(tmpCtx, "app", app)

	app.ctx = rootContext

	app.systemCfg.Services = make(map[string]ServiceConfig)

	app.systemCfg.Services["peertube"] = ServiceConfig{
		Name:    "Peertube",
		Started: false,
		ConfigFiles: []ServiceConfigFile{
			{
				Name: "Environment vars",
				Path: "./dcomms-docker-compose/conf/peertube/environment",
			},
		},
		ComposePath: "peertube.docker-compose.yml",
	}

	app.systemCfg.Services["mastodon"] = ServiceConfig{
		Name:    "Mastodon",
		Started: false,
		ConfigFiles: []ServiceConfigFile{
			{
				Name: "Environment vars",
				Path: "./dcomms-docker-compose/conf/mastodon/env.production",
			},
		},
		ComposePath: "mastodon.docker-compose.yml",
	}

	app.systemCfg.Services["synapse"] = ServiceConfig{
		Name:    "Matrix Synapse",
		Started: false,
		ConfigFiles: []ServiceConfigFile{
			{
				Name: "Environment vars",
				Path: "./dcomms-docker-compose/conf/synapse/livekit.yaml",
			},
		},
		ComposePath: "synapse.docker-compose.yml",
	}

	signal.Notify(stop, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	fileServer := http.FileServer(http.Dir("./static"))

	mux := http.NewServeMux()

	mux.Handle("GET /", fileServer)
	mux.HandleFunc("GET /target/{target}/info", httpGetInfo)
	mux.HandleFunc("POST /target/create", httpPostTargetCreate)
	mux.HandleFunc("GET /target/list", httpGetTargets)
	mux.HandleFunc("POST /target/{target}/update", httpPostTargetUpdate)
	mux.HandleFunc("GET /target/{target}/env/{id}", httpGetEnv)
	mux.HandleFunc("POST /target/{target}/service/{id}/stop", httpPostServiceStop)
	mux.HandleFunc("POST /target/{target}/service/{id}/start", httpPostServiceStart)
	mux.HandleFunc("POST /target/{target}/install", httpPostRunInstallScript)
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
