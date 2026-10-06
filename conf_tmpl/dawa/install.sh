#!/bin/sh

set -voeu pipefail

printf "[DCOMMS State] Generating Dawa config\n"

SALT=$(openssl rand -hex 64)
SECRET_KEY_BASE=$(openssl rand -hex 64)
OPENID_PRIVATE_KEY=$(openssl ecparam -name secp521r1 -genkey -noout | grep -v -- '-----' | tr -d '\n')

printf "SALT=%s\n" "$SALT" >> env
printf "SECRET_KEY_BASE=%s\n" "$SECRET_KEY_BASE" >> env
printf "OPENID_PRIVATE_KEY=%s\n" "$OPENID_PRIVATE_KEY" >> env

printf "[DCOMMS State] Starting dawa app\n"

export DOCKER_GID=$(getent group docker | cut -d: -f3)

docker compose up --wait

printf "[DCOMMS State] Dawa install done\n"
