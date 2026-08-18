#!/bin/bash

# This is mostly copy-pasted peertube waiting for debug

set -oxeu pipefail

final_dir="$BACKUP_ROOT/mastodon/$BACKUP_ID"
work_dir="$final_dir.partial"
completed=0
# TODO: locks
lock_acquired=0

source ./helpers.sh

cleanup() {
  status=$?

  sh ./dcomms_compose.sh start mastodon-sidekiq 2>/dev/null

  if [ "$completed" -ne 1 ] && [ -n "${work_dir:-}" ] && [ -d "$work_dir" ]; then
    rm -rf -- "$work_dir"
  fi

  if [ "$lock_acquired" -eq 1 ]; then
    rm -f -- "$lock_dir/owner"
    rmdir "$lock_dir" 2>/dev/null || true
  fi

  exit "$status"
}

trap cleanup EXIT

# maybe we should check and prevent backup if the final dir exists
# but for now having $final_dir means backup was succesful and this one is not yet
rm -rf "$final_dir"

mkdir -p "$work_dir"

bash ./dcomms_compose.sh stop mastodon-sidekiq

# sleep 1

bash ./dcomms_compose.sh exec -T mastodon-db sh -c \
  'PGPASSWORD=$DB_PASSWORD pg_dumpall 2>/dev/null \
    -U $DB_USER \
    -l $DB_NAME' | \
  gzip > "$work_dir/pgdump.sql.gz"

bash ./dcomms_compose.sh exec -T mastodon-redis sh -c \
  'redis-cli SAVE' | \
  gzip > "$work_dir/redis.rdb.gz"

docker run --rm \
  --user "$(id -u):$(id -g)" \
  --volumes-from $(bash ./dcomms_compose.sh ps -aq mastodon-sidekiq) \
  -v "$work_dir:/backup" \
  alpine:latest \
  tar czf /backup/content.tar.gz /mastodon/public/system

chmod 600 "$work_dir"/*
mv -- "$work_dir" "$final_dir"

completed=1
