#!/bin/bash

docker network inspect dawa >/dev/null 2>&1 || docker network create dawa
docker network inspect metrics >/dev/null 2>&1 || docker network create metrics
