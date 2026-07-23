#!/bin/bash

DCOMMS_GIT_REPO_PATH=./dcomms-docker-compose \
PODMAN_SOCKET=/run/user/$(id -u)/podman/podman.sock \
go run ./main.go
