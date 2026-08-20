package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"

	dockerClient "github.com/moby/moby/client"
)

func httpGetInfo(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(*GlobalState)
	t, err := getTargetFromRequest(app, r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.Write(remoteOsStatus(t))
}

func httpPostPanic(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(*GlobalState)
	_, err := getTargetFromRequest(app, r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	// TODO: remove all volumes
}

func httPostTestInstallation(app GlobalState, w http.ResponseWriter, r *http.Request) {
	// TODO: implement a check script to see if we're ready to install Dcomms on deployment target
}

func httpGetTargets(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(*GlobalState)
	out, err := json.Marshal(app.targets)
	if err != nil {
		fmt.Println("Failed to marshal targets into json:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(out)
}

func httpPostTargetCreate(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(*GlobalState)
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
		Remote:           false,
		Host:             r.FormValue("host"),
		User:             r.FormValue("username"),
		Port:             r.FormValue("port"),
		Domain:           r.FormValue("domainname"),
		ContainerBackend: containerBackend,
		MatrixEnabled:    r.FormValue("matrix_on") == "on",
		CenoEnabled:      r.FormValue("ceno_on") == "on",
		PeertubeEnabled:  r.FormValue("peertube_on") == "on",
		MastodonEnabled:  r.FormValue("mastodon_on") == "on",
		MauEnabled:       r.FormValue("mau_on") == "on",
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
	app := r.Context().Value("app").(*GlobalState)
	t, err := getTargetFromRequest(app, r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	// FIXME: properly process form values
	t.Remote = false
	t.Host = r.FormValue("host")
	t.User = r.FormValue("username")
	t.Port = r.FormValue("port")
	t.Domain = r.FormValue("domainname")
	t.MatrixEnabled = r.FormValue("matrix_on") == "on"
	t.CenoEnabled = r.FormValue("ceno_on") == "on"
	t.PeertubeEnabled = r.FormValue("peertube_on") == "on"
	t.MastodonEnabled = r.FormValue("mastodon_on") == "on"
	t.MauEnabled = r.FormValue("mau_on") == "on"

	app.targets[t.ID] = t

	targetsSave(app)

	fmt.Println("Target updated. ID:", t.ID)

	out, err := json.Marshal(t)
	if err != nil {
		fmt.Println("Failed to marshal target into json:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(out)
}

func httpPostRunCheckScript(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(*GlobalState)
	t, err := getTargetFromRequest(app, r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	ctx := r.Context()

	// TODO: pick a new port for different target
	cli, err := sshDockerSend(ctx, t, "localhost:3000")
	if err != nil {
		t.SSHTunnelEstablished = false
		fmt.Println("Failed to tunnel to docker", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Tunnel failed: %s", err)
		return
	}

	t.SSHTunnelEstablished = true

	app.targets[t.ID] = t

	if err := dockerInspectContainers(ctx, cli); err != nil {
		fmt.Println("Failed to inspect docker containers", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Inspecting containers on target failed: %s", err)
		return
	}
}

func httpPostRunStartupScript(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app := ctx.Value("app").(*GlobalState)
	t, err := getTargetFromRequest(app, r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	pathToDComms := "/home/" + t.User + "/dcommsctl/dcomms-docker-compose"
	pathToDCommsRun := pathToDComms + "/run.sh"

	if t.Remote {
		err = sshExecCommand(ctx, t, `cd `+pathToDComms+` && PATH=$PATH:~/.local/bin `+pathToDCommsRun)
		if err != nil {
			fmt.Println("Failed to exec run.sh script:", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		cmd := exec.CommandContext(ctx, "./dcomms_compose.sh", "up -d")
		cmd.Dir = pathToDComms

		scriptOut, err := cmd.CombinedOutput()
		fmt.Println(string(scriptOut))
		if err != nil {
			fmt.Println("Failed to exec run.sh script:", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		}
	}
}

func httpPostRunStopScript(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app := ctx.Value("app").(*GlobalState)
	t, err := getTargetFromRequest(app, r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	pathToDComms := "/home/" + t.User + "/dcommsctl/dcomms-docker-compose"
	pathToDCommsStop := pathToDComms + "/stop.sh"

	if t.Remote {
		err = sshExecCommand(ctx, t, `cd `+pathToDComms+` && PATH=$PATH:~/.local/bin `+pathToDCommsStop)
		if err != nil {
			fmt.Println("Failed to exec run.sh script:", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		cmd := exec.CommandContext(ctx, "./dcomms_compose.sh", "down")
		cmd.Dir = pathToDComms

		scriptOut, err := cmd.CombinedOutput()
		fmt.Println(string(scriptOut))
		if err != nil {
			fmt.Println("Failed to exec run.sh script:", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		}
	}
}

func httpPostRunBackupScript(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app := ctx.Value("app").(*GlobalState)
	_, err := getTargetFromRequest(app, r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "can't get target %s", err)
		return
	}

	// FIXME: actually check if it's active
	application, err := getApplicationFromRequest(app, r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "can't get application %s", err)
		return
	}

	switch application.Name {

	case "peertube":
		{
			if err := peertubeDockerBackup(ctx, app); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "peertube backup: %s", err)
				return
			}
		}
	case "mastodon":
		{
			if err := mastodonDockerBackup(ctx, app); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "peertube backup: %s", err)
				return
			}
		}
	}
	fmt.Fprint(w, "Done")
}

func httpPostRunRestoreScript(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app := ctx.Value("app").(*GlobalState)
	_, err := getTargetFromRequest(app, r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	application, err := getApplicationFromRequest(app, r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	appName := r.PathValue("application")

	if appName == "peertube" {
		// FIXME: get a backupID
		if err := peertubeDockerRestore(ctx, app, "tmp1"); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "peertube restore: %s", err)
			return
		}
	} else {
		containerClient, err := dockerClient.New(
			dockerClient.FromEnv,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer containerClient.Close()

		for _, script := range application.RestoreSteps {
			if err := dockerSpinAndRun(
				ctx,
				app,
				containerClient,
				"",
				script.TmpContainerImage,
				[]string{script.ScriptPath},
				script.ScriptPath,
			); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
	}
}

func httpPostAddApplication(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(*GlobalState)
	t, err := getTargetFromRequest(app, r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	applicationID := r.PathValue("application")
	application, found := app.systemCfg.Applications[applicationID]

	if !found {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Target not found")
		return
	}

	if application.Name == "mastodon" {
		t.MastodonEnabled = true
	}
	if application.Name == "matrix" {
		t.MatrixEnabled = true
	}

	if application.Name == "peertube" {
		t.PeertubeEnabled = true
	}

	if err := scriptInstall(r.Context(), app, t); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	app.targets[t.ID] = t
}

func httpPostRunInstallScript(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(*GlobalState)
	t, err := getTargetFromRequest(app, r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	if err := scriptInstall(r.Context(), app, t); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
}

func httpGetEnv(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(*GlobalState)
	applicationID := r.PathValue("id")

	t, err := getTargetFromRequest(app, r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	env, err := getEnv(app, t, applicationID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Get env variables problem", err)
		return
	}

	w.Write(env)
}
