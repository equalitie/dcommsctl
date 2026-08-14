#!/bin/sh

set -eu

umask 077

source ./helpers.sh

completed=0
work_dir="$BACKUP_ROOT/.$BACKUP_ID.partial.$$"

trap cleanup 0

PGPASSWORD=$MASTODON_DB_PASSWORD pg_dump \
  --host="$MASTODON_DB_HOST" \
  --port="$MASTODON_DB_PORT" \
  --username="$MASTODON_DB_USER" \
  --dbname="$MASTODON_DB_NAME" \
  --format=custom \
  --no-owner \
  --no-privileges \
  --file="$work_dir/database.dump"

log "copying Mastodon application configuration"
cp -p "$CONFIG_FILE" "$work_dir/env.production"

completed=1

