#!/bin/bash

set -oxeu pipefail

final_dir="$BACKUP_ROOT/peertube/$BACKUP_ID"
work_dir="$final_dir.partial"
completed=0
# TODO: locks
lock_acquired=0

source ./helpers.sh

cleanup() {
  status=$?

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

bash ./dcomms_compose.sh stop peertube-back

# sleep 1

bash ./dcomms_compose.sh exec -T peertube-postgres sh -c \
  'PG_PASSWORD=$POSTGRES_PASSWORD pg_dumpall 2>/dev/null \
    -U $POSTGRES_USER \
    -l $POSTGRES_DB' | \
  gzip > "$work_dir"/pgdump.sql.gz

# bash ./dcomms_compose.sh exec peertube-redis redis-cli BGSAVE 2>/dev/null | \
# gzip > "$work_dir/redis.gz"

# docker run --rm \
#   --volumes-from ${DATA_CONTAINER} \
#   -v "${BACKUP_ROOT}/peertube/volume" \
#   alpine:latest \
#   tar czf "/backup/" -C /data .

sh ./dcomms_compose.sh stop peertube-back

chmod 600 "$work_dir"/*
mv -- "$work_dir" "$final_dir"

completed=1
