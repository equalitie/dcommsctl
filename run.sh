#!/bin/bash

: this should have a default value probably, specified in the code,
# commenting this out for now since podman implementation is unstable
# PODMAN_SOCKET=/run/user/$(id -u)/podman/podman.sock \
DCOMMS_GIT_REPO_PATH=./dcomms-docker-compose \
go run ./
