#!/bin/sh

set -voeu pipefail

log_path=./install.partial.log
final_log_path=./install.log

cleanup() {
  status=$?

  rm "$LOCK_FILE"

  exit "$status"
}

trap cleanup EXIT

printf "[DCOMMS State] Generating Peertube config\n"

PEERTUBE_SECRET=$(openssl rand -hex 32)

printf "PEERTUBE_SECRET=$PEERTUBE_SECRET" >> env

printf "[DCOMMS State] Starting peertube app\n"

docker compose up -d

printf "[DCOMMS State] Performing peertube post-install\n"

docker compose exec -u peertube peertube-back sh -c 'printf "%s\n" "$PEERTUBE_ADMIN_PASS" | npm run reset-password -- -u root'

cp $log_path $final_log_path

printf "[DCOMMS State] Peertube post-install done\n"

