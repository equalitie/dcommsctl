#!/bin/sh
set -eu

umask 077

source ./helpers.sh

if ! which pg_dump date >/dev/null; then
        printf "${RED}## Some dependencies are not installed\n"
        printf "Maybe this script is running on the wrong container. This is a PostgreSQL backup script\n"
fi

require_readable_directory() {
  [ -d "$1" ] || die "directory does not exist: $1"
  [ -r "$1" ] || die "directory is not readable: $1"
}

BACKUP_ROOT=/backups

final_dir="$BACKUP_ROOT/$BACKUP_ID"
work_dir="$BACKUP_ROOT/.$BACKUP_ID.partial.$$"

mkdir -p "$work_dir"

log "dumping PostgreSQL database '$DB_NAME' from $DB_HOST:$DB_PORT"
PGPASSWORD=$DB_PASSWORD pg_dump \
  --host="$DB_HOST" \
  --port="$DB_PORT" \
  --username="$DB_USER" \
  --dbname="$DB_NAME" \
  --format=custom \
  --no-owner \
  --no-privileges \
  --file="$work_dir/peertube_database.dump"

