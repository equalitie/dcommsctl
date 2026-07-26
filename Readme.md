# What this is

This is definitely not production quality but mostly a result of explorative programming session.

# How to run

Requirements
 * Go 
 * A web browser

Run the backend

```
chmod +x ./run.sh
./run.sh
```

Open url `http://localhost:8080/` in any browser

# What is implemented

Right now it's an application that let's you connect via SSH to a deployment target of your choice (VPS-only for now). It assumes the deployment target to be a Debian machine. It install everything required to run dcomms project, copies all the compose files required and calls a modified install script.

You can connect to a VPS assuming you have a SSH-agent running and ssh key added. You specify host, username and port for SSH connection on the main page of Web UI at `http://localhost:8080/`

A lot of the things are hardcoded for now and only one deployment target is managed at the same time.

The dcomms-docker-compose project is included not as a git submodule because of changes to the scripts. There's a temporary copy of install.sh script from that repo called install_podman.sh which is tailored for podman use.

Due to a bug podman-compose is currently not found when running via ssh.

# API

The basic structure of entities looks like this:
    * Target (Deployment target) - VPS or Cloud service
     * Service - Docker/Podman compose service
      * Actual containers

So first of all we create a target, run install script there, then run the first run services

# TODO

* debug install_podman.sh (right now only CENO, parts of Peertube and Mastodon are succesfully installed, Matrix setup hangs when running podman-compose up) 
* implement choice of services you want to install
* make it possible to create new deployment targets and check for the status of running services
* actually handle running and stopping of podman services
* fetch status of all services
