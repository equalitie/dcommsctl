#!/bin/bash

# This is mostly copy-pasted peertube waiting for debug

set -oxeu pipefail

final_dir="$BACKUP_ROOT/mastodon/$BACKUP_ID"
work_dir="$final_dir.partial"
completed=0
service_stopped=0

cleanup() {
  status=$?

  if [ "$service_stopped" -ne 0 ]
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

docker compose stop mastodon-sidekiq
service_stopped=1

# sleep 1

docker compose exec -T mastodon-db sh -c \
  'PGPASSWORD=$DB_PASSWORD pg_dumpall 2>/dev/null \
    -U $DB_USER \
    -l $DB_NAME' | \
  gzip > "$work_dir/pgdump.sql.gz"

docker compose exec -T mastodon-redis sh -c \
  'redis-cli SAVE' | \
  gzip > "$work_dir/redis.rdb.gz"

docker run --rm \
  --user "$(id -u):$(id -g)" \
  --volumes-from $(./dcomms_compose.sh ps -aq mastodon-sidekiq) \
  -v "$work_dir:/backup" \
  alpine:latest \
  tar czf /backup/content.tar.gz /mastodon/public/system

chmod 600 "$work_dir"/*
mv -- "$work_dir" "$final_dir"

completed=1
