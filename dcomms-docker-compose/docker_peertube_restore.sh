#!/bin/bash

set -oeu pipefail

backup_dir="$BACKUP_ROOT/peertube/$BACKUP_ID"
completed=0

source ./helpers.sh

cleanup() {
  status=$?

  sh ./dcomms_compose.sh start peertube-back 2>/dev/null

  exit "$status"
}

trap cleanup EXIT

./dcomms_compose.sh stop peertube-back

./dcomms_compose.sh exec -T peertube-postgres sh -c \
  'PGPASSWORD=$POSTGRES_PASSWORD dropdb \
    --force \
    -U "$POSTGRES_USER" \
    "$POSTGRES_DB"'

gunzip -c "$backup_dir/pgump.sql.gz" | \
  ./dcomms_compose.sh exec -T peertube-postgres sh -c \
    'PGPASSWORD=$POSTGRES_PASSWORD psql \
      -U "$POSTGRES_USER" \
      -d postgres' > /dev/null

./dcomms_compose.sh exec -T peertube-redis \
  redis-cli FLUSHALL

docker run --rm \
  --volumes-from "$(./dcomms_compose.sh ps -aq peertube-back)" \
  -v "$backup_dir:/backup:ro" \
  alpine:latest \
  sh -c 'rm -rf /data/* /data/.[!.]* /data/..?* &&
    tar xzf /backup/content.tar.gz -C /'

completed=1
