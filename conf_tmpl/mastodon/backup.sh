#!/bin/bash

# This is mostly copy-pasted peertube waiting for debug

set -oxeu pipefail

umask 077

final_dir="$BACKUP_ROOT/mastodon/$BACKUP_ID"
work_dir="$final_dir.partial"
completed=0
service_stopped=0

cleanup() {
  status=$?

  if [ "$service_stopped" -ne 0 ]; then
      docker compose start mastodon-web mastodon-streaming 2>/dev/null
      docker compose start mastodon-sidekiq 2>/dev/null
  fi

  if [ "$completed" -ne 1 ] && [ -n "${work_dir:-}" ] && [ -d "$work_dir" ]; then
    rm -rf -- "$work_dir"
  fi

  exit "$status"
}

trap cleanup EXIT

# maybe we should check and prevent backup if the final dir exists
# but for now having $final_dir means backup was succesful and this one is not yet
rm -rf "$final_dir"

mkdir -p "$work_dir"

printf "[DCOMMS State] Stopping mastodon services\n"
docker compose stop --timeout 120 mastodon-web mastodon-streaming
docker compose stop --timeout 120 mastodon-sidekiq
service_stopped=1


printf "[DCOMMS State] Backing mastodon database\n"
docker compose exec -T mastodon-db sh -eu -c \
  'PGPASSWORD="$DB_PASSWORD" exec pg_dumpall \
    -w -U "$DB_USER" -l "$DB_NAME"' |
  gzip > "$work_dir/pgdump.sql.gz"

printf "[DCOMMS State] Backing redis\n"
docker compose exec -T mastodon-redis \
  redis-cli --rdb - |
  gzip > "$work_dir/redis.rdb.gz"

printf "[DCOMMS State] Backing content\n"
docker run --rm \
  --user "$(id -u):$(id -g)" \
  --volumes-from $(docker compose ps -aq mastodon-sidekiq) \
  -v "$work_dir:/backup" \
  alpine:latest \
  tar czf /backup/content.tar.gz /mastodon/public/system

chmod 600 "$work_dir"/*
mv -- "$work_dir" "$final_dir"

completed=1
printf "[DCOMMS State] Mastodon backup completed\n"

