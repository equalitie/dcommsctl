# What this is

This is definitely not production quality but mostly a result of explorative programming session.

# How to run

Requirements
 * Go 
 * A web browser or curl

Run the backend

```
chmod +x ./run.sh
./run.sh
```

Open url `http://localhost:8080/` in any browser

# What is implemented

Right now it's an application that let's you connect via SSH to a deployment target of your choice (VPS-only for now). It assumes the deployment target to be a Debian machine. It install everything required to run dcomms project, copies all the compose files required and calls a modified install script.

You can connect to a VPS assuming you have a SSH-agent running and ssh key added. You specify host, username and port for SSH connection on the main page of Web UI at `http://localhost:8080/`

A lot of the things are hardcoded for now. Podman implementation is bugged.

The dcomms-docker-compose project is included not as a git submodule because of changes to the scripts. There's a temporary copy of install.sh script from that repo called install_podman.sh which is tailored for podman use.

Targets are stored in `targets.csv` (soon to be JSON) file. If you need to remove all targets, just delete the file.

# API

The basic structure of entities looks like this:
    * Target (Deployment target) - VPS or Cloud service
     * Service - Docker/Podman compose service
      * Actual containers

So first of all we create a target, run install script there, then run the services

This is a rough draft, to check the actual api look at the `main.go` where mux handlers are registered

```
POST /target/create - create target with POST form data, returns target data in JSON format
GET /target/list - get all targets in JSON format
POST /target/{target_id}/update - update target with POST form data
POST /target/{target_id}/install - install pre-requisites
POST /target/{target_id}/start - run services
POST /target/{target_id}/stop - stop services
POST /target/{target_id}/check - right now pings remote Docker socket, gets API version and all running containers
```

Right now there is no properly working web UI so it's enough to just use curl:

```
curl -d 'host=127.127.127.127&username=dcomms&port=22&domainname=domainname.org&backend=docker&matrix_on=on' localhost:8080/target/create
```

# TODO

* make it possible to run commands on containers
    * make SSH tunnel to call docker api on socket through SSH connection
    * parse compose files to get service names and containers they run

* debug install_podman.sh (right now only CENO, parts of Peertube and Mastodon are succesfully installed, Matrix setup hangs when running podman-compose up) 
