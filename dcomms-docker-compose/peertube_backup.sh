#!/bin/sh

set -eu

umask 077

log() {
  printf '%s %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$*" >&2
}

die() {
  log "ERROR: $*"
  exit 1
}

need_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

require_readable_directory() {
  [ -d "$1" ] || die "directory does not exist: $1"
  [ -r "$1" ] || die "directory is not readable: $1"
}

require_non_empty() {
  variable_name=$1
  variable_value=$2
  [ -n "$variable_value" ] || die "required environment variable is missing or empty: $variable_name"
}

# PostgreSQL connection settings are intentionally explicit. Do this before
# creating directories, locks, or partial output so a configuration error has
# no filesystem side effects.
require_non_empty PEERTUBE_DB_HOST "${PEERTUBE_DB_HOST-}"
require_non_empty PEERTUBE_DB_PORT "${PEERTUBE_DB_PORT-}"
require_non_empty PEERTUBE_DB_NAME "${PEERTUBE_DB_NAME-}"
require_non_empty PEERTUBE_DB_USER "${PEERTUBE_DB_USER-}"
require_non_empty PEERTUBE_DB_PASSWORD "${PEERTUBE_DB_PASSWORD-}"

case "$PEERTUBE_DB_PORT" in
  *[!0-9]*) die "PEERTUBE_DB_PORT must be an integer from 1 to 65535" ;;
esac
[ "$PEERTUBE_DB_PORT" -ge 1 ] && [ "$PEERTUBE_DB_PORT" -le 65535 ] || \
  die "PEERTUBE_DB_PORT must be an integer from 1 to 65535"

BACKUP_ROOT=${BACKUP_ROOT:-/backups}
STORAGE_DIR=${STORAGE_DIR:-/source/storage}
CONFIG_DIR=${CONFIG_DIR:-/source/config}
BACKUP_ID=${BACKUP_ID:-peertube-$(date -u '+%Y%m%dT%H%M%SZ')}

case "$BACKUP_ID" in
  ''|.|..|latest|LATEST|.peertube-backup.lock|*[!A-Za-z0-9_.-]*)
    die "BACKUP_ID is invalid or reserved; use letters, digits, dot, underscore, and dash"
    ;;
esac

need_command pg_dump
need_command tar
need_command sha256sum
need_command date

require_readable_directory "$STORAGE_DIR"
require_readable_directory "$CONFIG_DIR"

mkdir -p "$BACKUP_ROOT"
[ -d "$BACKUP_ROOT" ] || die "backup root is not a directory: $BACKUP_ROOT"
[ -w "$BACKUP_ROOT" ] || die "backup root is not writable: $BACKUP_ROOT"

final_dir="$BACKUP_ROOT/$BACKUP_ID"
work_dir="$BACKUP_ROOT/.$BACKUP_ID.partial.$$"
lock_dir="$BACKUP_ROOT/.peertube-backup.lock"
lock_acquired=0
completed=0

[ ! -e "$final_dir" ] || die "backup already exists: $final_dir"
[ ! -e "$work_dir" ] || die "temporary backup path already exists: $work_dir"

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

trap cleanup 0
trap 'exit 130' INT
trap 'exit 143' TERM HUP

if ! mkdir "$lock_dir" 2>/dev/null; then
  die "another backup appears to be running; remove stale lock only after checking: $lock_dir"
fi
lock_acquired=1
printf 'pid=%s\nbackup_id=%s\n' "$$" "$BACKUP_ID" > "$lock_dir/owner"

mkdir "$work_dir"

log "dumping PostgreSQL database '$PEERTUBE_DB_NAME' from $PEERTUBE_DB_HOST:$PEERTUBE_DB_PORT"
PGPASSWORD=$PEERTUBE_DB_PASSWORD pg_dump \
  --host="$PEERTUBE_DB_HOST" \
  --port="$PEERTUBE_DB_PORT" \
  --username="$PEERTUBE_DB_USER" \
  --dbname="$PEERTUBE_DB_NAME" \
  --format=custom \
  --no-owner \
  --no-privileges \
  --file="$work_dir/database.dump"

log "archiving PeerTube storage from $STORAGE_DIR"
tar \
  --create \
  --gzip \
  --numeric-owner \
  --file="$work_dir/storage.tar.gz" \
  --directory="$STORAGE_DIR" \
  .

log "archiving PeerTube configuration from $CONFIG_DIR"
tar \
  --create \
  --gzip \
  --numeric-owner \
  --file="$work_dir/config.tar.gz" \
  --directory="$CONFIG_DIR" \
  .

created_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
{
  printf 'format_version=1\n'
  printf 'backup_id=%s\n' "$BACKUP_ID"
  printf 'created_at_utc=%s\n' "$created_at"
  printf 'database_name=%s\n' "$PEERTUBE_DB_NAME"
  printf 'database_format=postgresql-custom\n'
  printf 'storage_archive=storage.tar.gz\n'
  printf 'config_archive=config.tar.gz\n'
  printf 'peertube_version=%s\n' "${PEERTUBE_VERSION:-unknown}"
} > "$work_dir/manifest.txt"

(
  cd "$work_dir"
  sha256sum database.dump storage.tar.gz config.tar.gz manifest.txt > SHA256SUMS
)

chmod 600 "$work_dir"/*
mv "$work_dir" "$final_dir"
work_dir=

latest_tmp="$BACKUP_ROOT/.LATEST.$$"
printf '%s\n' "$BACKUP_ID" > "$latest_tmp"
chmod 600 "$latest_tmp"
mv "$latest_tmp" "$BACKUP_ROOT/LATEST"

completed=1
log "backup completed: $final_dir"
printf '%s\n' "$BACKUP_ID"
