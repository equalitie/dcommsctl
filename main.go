package main

import (
	"bytes"
	"context"
	_ "embed"
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
	"syscall"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"go.podman.io/podman/v6/pkg/bindings"
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

// This is a deployment target info, either cloud or VPS, for now only VPS
type DeploymentTargetInfo struct {
	User   string
	Host   string
	Port   string
	Domain string
	// folder where we store the dcomms repo
	ProjectRoot string
}

type Application struct {
	conn      DeploymentTargetInfo
	ctx       context.Context
	systemCfg SystemConfig
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

func copyFolder(client *sftp.Client, src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(src, path)
		if relPath == "." {
			return nil // Skip root
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
			return fmt.Errorf("create err: %s", err)
		}
		defer dstFile.Close()
		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}

func copyProject(app Application) error {
	fmt.Println("Starting copy")
	pathToDcomms := os.Getenv("DCOMMS_GIT_REPO_PATH")
	// cmd := exec.Command("scp", "-r", "-v", "-P", app.conn.Port, pathToDcomms + "/.", app.conn.User+"@"+app.conn.Host+":~/dcomms/setup")
	// output, err := cmd.CombinedOutput()

	// if err != nil {
	// 	log.Printf("Error running shell: %v\n%s\n", err, output)
	// 	return err
	// }

	socket := os.Getenv("SSH_AUTH_SOCK")

	conn, err := net.Dial("unix", socket)

	if err != nil {
		return fmt.Errorf("unable to connect to ssh-agent socket: %v", err)
	}

	agentClient := agent.NewClient(conn)

	config := &ssh.ClientConfig{
		User: app.conn.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", app.conn.Host+":"+app.conn.Port, config)

	if err != nil {
		return err
	}
	defer client.Close()
	sftpClient, err := sftp.NewClient(client)

	if err != nil {
		return fmt.Errorf("Failed to init sftp connection: %s", err)
	}

	if err := copyFolder(sftpClient, pathToDcomms, "/home/"+app.conn.User+"/dcomms/setup"); err != nil {
		return fmt.Errorf("Fail to copy files: %s", err)
	}

	fmt.Println("Ending copy")
	return nil
}

// FIXME: we need to get result out, either write to a buffer or something
func sshExecCommand(c DeploymentTargetInfo, cmd string) error {
	socket := os.Getenv("SSH_AUTH_SOCK")

	conn, err := net.Dial("unix", socket)

	if err != nil {
		return fmt.Errorf("unable to connect to ssh-agent socket: %v", err)
	}

	agentClient := agent.NewClient(conn)

	config := &ssh.ClientConfig{
		User: c.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", c.Host+":"+c.Port, config)

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

func remoteOsStatus(app Application) []byte {
	var buf bytes.Buffer

	commands := []string{
		"grep MemTotal /proc/meminfo |tr -s ' ' |cut -d ' ' -f2",
		"free -h",
		"df -h",
		"uptime",
	}

	for _, cmd := range commands {
		if err := sshExecCommand(app.conn, cmd); err != nil {
			return nil
		}
	}

	return buf.Bytes()
}

func remoteServiceDown(app Application, serviceID string) error {
	serviceCfg, found := app.systemCfg.Services[serviceID]

	if !found {
		return fmt.Errorf("Service is not registered %s", serviceID)
	}

	cmd := fmt.Sprintf("podman-compose -f %s down", path.Join("/home", app.conn.User, "dcomms", "setup", "conf", "compose", serviceCfg.ComposePath))

	if err := sshExecCommand(app.conn, cmd); err != nil {
		return err
	}

	return nil
}

func remoteServiceUp(app Application, serviceID string) error {
	serviceCfg, found := app.systemCfg.Services[serviceID]

	if !found {
		return fmt.Errorf("Service is not registered %s", serviceID)
	}

	cmd := fmt.Sprintf("podman-compose -d -f %s up", path.Join("/home", app.conn.User, "dcomms", "setup", "conf", "compose", serviceCfg.ComposePath))

	if err := sshExecCommand(app.conn, cmd); err != nil {
		return err
	}

	return nil
}

func getEnv(app Application, serviceID string) ([]byte, error) {
	serviceCfg, found := app.systemCfg.Services[serviceID]

	if !found {
		return nil, fmt.Errorf("Service is not registered %s", serviceID)
	}
	var stdoutBuf bytes.Buffer
	var out bytes.Buffer

	err := sshExecCommand(app.conn, fmt.Sprintf("podman-compose -f config %s --services", serviceCfg.ComposePath))

	if err != nil {
		return nil, err
	}

	services := bytes.Split(stdoutBuf.Bytes(), []byte("\n"))

	for _, serviceName := range services {

		name := string(serviceName)
		if name == "" {
			continue
		}
		// TODO: write this to `out`
		cmd := fmt.Sprintf("podman-compose -d -f %s exec %s", path.Join("/home", app.conn.User, "dcomms", "setup", "conf", "compose", serviceCfg.ComposePath), "env")

		if err := sshExecCommand(app.conn, cmd); err != nil {
			return nil, err
		}

		out.WriteString(string(serviceName) + "\n")

		out.WriteString("==========\n\n")
	}

	return out.Bytes(), nil
}

func httpGetInfo(app Application, w http.ResponseWriter, r *http.Request) {
	w.Write(remoteOsStatus(app))
}

func httpPostServiceStart(app Application, w http.ResponseWriter, r *http.Request) {
	serviceID := r.PathValue("id")
	remoteServiceUp(app, serviceID)
}

func httpPostServiceStop(app Application, w http.ResponseWriter, r *http.Request) {
	serviceID := r.PathValue("id")
	remoteServiceDown(app, serviceID)
}

func httpPostPanic(app Application, w http.ResponseWriter, r *http.Request) {
	serviceStopAll(app)
	// TODO: remove all volumes
}

func serviceStopAll(app Application) {
	for serviceID, _ := range app.systemCfg.Services {
		if err := remoteServiceDown(app, serviceID); err != nil {
			log.Printf("Failed to bring compose down: %s", err)
		}
	}
}

func httpPostMatrixInstall(app Application, w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Ok"))
}

func postTestInstallation(app Application, w http.ResponseWriter, r *http.Request) {

}

func postRunStartupScript(app Application, w http.ResponseWriter, r *http.Request) {

}

func postRunInstallScript(app Application, w http.ResponseWriter, r *http.Request) {
	// depTargetID := r.PathValue("target")
	// FIXME: properly process those
	// FIXME: we only have one connection here, do it properly with connection IDs and a list of connections
	app.conn = DeploymentTargetInfo{
		Host:   r.FormValue("host"),
		User:   r.FormValue("username"),
		Port:   r.FormValue("port"),
		Domain: r.FormValue("domainname"),
	}

	cmd := exec.Command("ssh", app.conn.User+"@"+app.conn.Host, "bash -s")
	stdin, err := cmd.StdinPipe()

	if err != nil {
		fmt.Println("Error accessing std in", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	go func() {
		defer stdin.Close()
		stdin.Write(installScriptContents)
	}()

	// FIXME: we need a unified output buffer we're sending back to the client
	output, err := cmd.CombinedOutput()
	fmt.Println("--- Running pre-req install script ---")
	fmt.Println(string(output))
	if err != nil {
		fmt.Println(app.conn.Host, app.conn.User, app.conn.Port)
		fmt.Println("Error running ssh to a VPS", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	fmt.Println("--- End of pre-req install script  ---")

	if err := copyProject(app); err != nil {
		fmt.Println("Failed to copy project files:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	pathToDComms := "/home/" + app.conn.User + "/dcomms/setup"
	pathToDCommsInstall := pathToDComms + "/install_podman.sh"

	// FIXME: temporary PATH setting
	err = sshExecCommand(app.conn, `cd `+pathToDComms+` && chmod +x `+pathToDCommsInstall+` && PATH=$PATH:~/.local/bin DWEB_DOMAIN=`+app.conn.Domain+` CHOICES="1" `+pathToDCommsInstall)
	if err != nil {
		fmt.Println("Failed to exec install script:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// w.Write(output)
}

func httpGetEnv(app Application, w http.ResponseWriter, r *http.Request) {
	serviceID := r.PathValue("id")

	env, err := getEnv(app, serviceID)

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
	rootContext := context.Background()
	serverError := make(chan error, 1)
	stop := make(chan os.Signal, 1)
	app := Application{}

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
	mux.HandleFunc("GET /info", func(w http.ResponseWriter, r *http.Request) {
		httpGetInfo(app, w, r)
	})
	mux.HandleFunc("POST /target/create", func(w http.ResponseWriter, r *http.Request) {
		// FIXME: call for target creation
	})
	mux.HandleFunc("GET /target/{target}/env/{id}", func(w http.ResponseWriter, r *http.Request) {
		httpGetEnv(app, w, r)
	})

	mux.HandleFunc("POST /target/{target}/service/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		httpPostServiceStop(app, w, r)
	})
	mux.HandleFunc("POST /target/{target}/service/{id}/start", func(w http.ResponseWriter, r *http.Request) {
		httpPostServiceStart(app, w, r)
	})
	mux.HandleFunc("POST /target/{target}/install", func(w http.ResponseWriter, r *http.Request) {
		postRunInstallScript(app, w, r)
	})
	mux.HandleFunc("POST /target/{target}/panic/", func(w http.ResponseWriter, r *http.Request) {
		httpPostPanic(app, w, r)
	})

	addr := os.Getenv("WEBHOST_PORT")

	if addr == "" {
		addr = ":8080"
	}

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
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

	// serviceStopAll(app)
	// log.Println("Compose services are shut down")

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %s", err)
	}

	log.Println("Server shut down properly")
}
