#!/bin/bash

set -oeu pipefail

backup_dir="$BACKUP_ROOT/peertube/$BACKUP_ID"
completed=0

cleanup() {
  status=$?

  docker compose start peertube-back 2>/dev/null

  exit "$status"
}

trap cleanup EXIT

docker compose stop peertube-back

docker compose exec -T peertube-postgres sh -c \
  'PGPASSWORD=$POSTGRES_PASSWORD dropdb \
    --force \
    -U "$POSTGRES_USER" \
    "$POSTGRES_DB"'

gunzip -c "$backup_dir/pgump.sql.gz" | \
  docker compose exec -T peertube-postgres sh -c \
    'PGPASSWORD=$POSTGRES_PASSWORD psql \
      -U "$POSTGRES_USER" \
      -d postgres' > /dev/null

docker compose exec -T peertube-redis \
  redis-cli FLUSHALL

docker run --rm \
  --volumes-from "$(docker compose ps -aq peertube-back)" \
  -v "$backup_dir:/backup:ro" \
  alpine:latest \
  sh -c 'rm -rf /data/* /data/.[!.]* /data/..?* &&
    tar xzf /backup/content.tar.gz -C /'

completed=1
