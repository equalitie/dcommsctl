#!/bin/sh

set -eu

umask 077

source ./helpers.sh

log "restoring PostgreSQL database '$PEERTUBE_DB_NAME' on $PEERTUBE_DB_HOST:$PEERTUBE_DB_PORT"
PGPASSWORD=$PEERTUBE_DB_PASSWORD pg_restore \
  --host="$PEERTUBE_DB_HOST" \
  --port="$PEERTUBE_DB_PORT" \
  --username="$PEERTUBE_DB_USER" \
  --clean \
  --if-exists \
  --no-owner \
  --no-privileges \
  --single-transaction \
  --dbname="$PEERTUBE_DB_NAME" \
  "$backup_dir/database.dump"

