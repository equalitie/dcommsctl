#!/bin/bash

final_dir=${BACKUP_ROOT}/peertube/${BACKUP_ID}
work_dir="$final_dir.partial"
completed=0
# TODO: locks
lock_acquired=0

source ./helpers.sh

set -xeu

mkdir -p $work_dir

trap cleanup EXIT

bash ./dcomms_compose.sh stop peertube-back

# sleep 1

bash ./dcomms_compose.sh exec peertube-postgres sh -c \
  'PG_PASSWORD=$POSTGRES_PASSWORD pg_dumpall 2>/dev/null -Fc \
    -U $POSTGRES_USER \
    -l $POSTGRES_DB \
    -Fc' | \
  gzip > "$work_dir/pgdump.sql.gz"

# bash ./dcomms_compose.sh exec peertube-redis redis-cli BGSAVE 2>/dev/null | \
# gzip > "$work_dir/redis.gz"

# docker run --rm \
#   --volumes-from ${DATA_CONTAINER} \
#   -v "${BACKUP_ROOT}/peertube/volume" \
#   alpine:latest \
#   tar czf "/backup/" -C /data .

sh ./dcomms_compose.sh stop peertube-back

chmod 600 "$work_dir"/*
mv "$work_dir" "$final_dir"

completed=1
