#!/bin/bash

set -xoeu pipefail

backup_dir="$BACKUP_ROOT/mastodon/$BACKUP_ID"
completed=0
service_stopped=0

cleanup() {
  status=$?

  if [ "$service_stopped" -ne 0]
      docker compose start mastodon-sidekiq 2>/dev/null
  fi

  exit "$status"
}

trap cleanup EXIT

docker compose stop mastodon-sidekiq

docker compose exec -T mastodon-db sh -c \
  'PGPASSWORD=$DB_PASSWORD dropdb \
    --force \
    -U "$DB_USER" \
    "$DB_NAME"'

gunzip -c "$backup_dir/pgump.sql.gz" | \
  docker compose exec -T mastodon-db sh -c \
    'PGPASSWORD=$DB_PASSWORD psql \
      -U "$DB_USER" \
      -d postgres' > /dev/null

docker compose exec -T mastodon-redis \
  redis-cli FLUSHALL

docker run --rm \
  --volumes-from "$(docker compose ps -aq mastodon-sidekiq)" \
  -v "$backup_dir:/backup:ro" \
  alpine:latest \
  sh -c 'rm -rf /mastodon/system/public/* /mastodon/system/public/.[!.]* /mastodon/system/public/..?* &&
    tar xzf /backup/content.tar.gz -C /'

completed=1
