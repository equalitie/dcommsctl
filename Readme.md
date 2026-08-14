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

Right now it's an application that you run on a host machine. It has some support for SSH connection to a VPS and running it there but not fully tested.

It assumes the deployment target to be a Debian machine. It install everything required to run dcomms project, copies all the compose files required and calls a modified install script.

A lot of the things are hardcoded for now. Podman implementation is bugged.

The dcomms-docker-compose project is included not as a git submodule because of changes to the scripts. There's a temporary copy of install.sh script from that repo called install_podman.sh which is tailored for podman use.

Targets are stored in `targets.csv` (soon to be JSON) file. If you need to remove all targets, just delete the file.

# API

The basic structure of entities looks like this:
    * Target (Deployment target) - VPS or Cloud service
     * Service - Docker/Podman compose service
      * Actual containers

So first of all we create a target, run install script there, then run the applications

This is a rough draft, to check the actual api look at the `main.go` where mux handlers are registered

```
POST /target/create - create target with POST form data, returns target data in JSON format
GET /target/list - get all targets in JSON format
POST /target/{target_id}/update - update target with POST form data
POST /target/{target_id}/install - install pre-requisites
POST /target/{target_id}/start - run all applications
POST /target/{target_id}/stop - stop all applications
POST /target/{target_id}/check - right now pings remote Docker socket, gets API version and all running containers
POST /target/{target_id}/backup/{application_name} - runs a backup for application
POST /target/{target_id}/restore/{application_name} - runs a restore for application
POST /target/{target_id}/application/add/{application_name} - adds a new application to the current stack
```

There is scratch throw-away Web UI at `http://localhost:8080/` but it might miss some features, so it's enough to just use curl:

```
curl -d 'host=127.127.127.127&username=dcomms&port=22&domainname=domainname.org&backend=docker&matrix_on=on' localhost:8080/target/create
```

# Toools to add

* CreepPad
