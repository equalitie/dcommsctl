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

# API

The basic structure of entities looks like this:
    * Target (Deployment target) - VPS or Cloud service
     * Service - Docker/Podman compose service
      * Actual containers

So first of all we create a target, run install script there, then run the services

This is a rough draft, to check the actual api look at the `main.go` where mux handlers are registered

```
POST /target/create - create target with POST form data
POST /target/{target_id}/update - update target with POST form data
POST /target/{target_id}/install - install pre-requisites
POST /target/{target_id}/startup - run services
```

Right now there is no properly working web UI so it's enough to just use curl:

```
curl -d 'host=127.127.127.127&username=dcomms&port=22&domainname=domainname.org&backend=docker' localhost:8080/target/create
```

Service choice is also hard-coded in main.go where the `install.sh` is called via CHOICES environment variable. Details are in the install.sh script.

# TODO

* debug install_podman.sh (right now only CENO, parts of Peertube and Mastodon are succesfully installed, Matrix setup hangs when running podman-compose up) 
* implement choice of services you want to install
* make it possible to check for the status of running services
* actually handle running and stopping of podman services
* fetch status of all services
