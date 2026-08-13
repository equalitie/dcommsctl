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

require_boolean() {
  case "$2" in
    0|1) ;;
    *) die "$1 must be 0 or 1" ;;
  esac
}

require_non_empty() {
  variable_name=$1
  variable_value=$2
  [ -n "$variable_value" ] || die "required environment variable is missing or empty: $variable_name"
}

require_set() {
  variable_name=$1
  variable_marker=$2
  [ "$variable_marker" = x ] || die "required environment variable is not set: $variable_name"
}

require_port() {
  variable_name=$1
  variable_value=$2
  case "$variable_value" in
    *[!0-9]*) die "$variable_name must be an integer from 1 to 65535" ;;
  esac
  [ "$variable_value" -ge 1 ] && [ "$variable_value" -le 65535 ] || \
    die "$variable_name must be an integer from 1 to 65535"
}

# PostgreSQL settings are required unconditionally and are never inferred from
# PG*, POSTGRES_*, or Mastodon's DB_* aliases.
require_non_empty DB_HOST "${DB_HOST-}"
require_non_empty DB_PORT "${DB_PORT-}"
require_non_empty DB_NAME "${DB_NAME-}"
require_non_empty DB_USER "${DB_USER-}"
require_non_empty DB_PASSWORD "${DB_PASSWORD-}"
require_port DB_PORT "$DB_PORT"

BACKUP_ROOT=${BACKUP_ROOT:-/backups}
MEDIA_DIR=${MEDIA_DIR:-/source/system}
CONFIG_FILE=${CONFIG_FILE:-/source/.env.production}
REDIS_DATA_DIR=${REDIS_DATA_DIR:-/source/redis}
INCLUDE_MEDIA=${INCLUDE_MEDIA:-1}
INCLUDE_REDIS=${INCLUDE_REDIS:-1}
BACKUP_ID=${BACKUP_ID:-mastodon-$(date -u '+%Y%m%dT%H%M%SZ')}

require_boolean INCLUDE_MEDIA "$INCLUDE_MEDIA"
require_boolean INCLUDE_REDIS "$INCLUDE_REDIS"

case "$BACKUP_ID" in
  ''|.|..|latest|LATEST|.mastodon-backup.lock|*[!A-Za-z0-9_.-]*)
    die "BACKUP_ID is invalid or reserved; use letters, digits, dot, underscore, and dash"
    ;;
esac

need_command pg_dump
need_command tar
need_command sha256sum
need_command date
need_command cp

[ -f "$CONFIG_FILE" ] || die "configuration file does not exist: $CONFIG_FILE"
[ -r "$CONFIG_FILE" ] || die "configuration file is not readable: $CONFIG_FILE"

if [ "$INCLUDE_MEDIA" = 1 ]; then
  require_readable_directory "$MEDIA_DIR"
fi

if [ "$INCLUDE_REDIS" = 1 ]; then
  require_non_empty REDIS_HOST "${REDIS_HOST-}"
  require_non_empty REDIS_PORT "${REDIS_PORT-}"
  require_set REDIS_PASSWORD "${REDIS_PASSWORD+x}"
  require_port REDIS_PORT "$REDIS_PORT"
  need_command redis-cli
  require_readable_directory "$REDIS_DATA_DIR"
fi

mkdir -p "$BACKUP_ROOT"
[ -d "$BACKUP_ROOT" ] || die "backup root is not a directory: $BACKUP_ROOT"
[ -w "$BACKUP_ROOT" ] || die "backup root is not writable: $BACKUP_ROOT"

final_dir="$BACKUP_ROOT/$BACKUP_ID"
work_dir="$BACKUP_ROOT/.$BACKUP_ID.partial.$$"
lock_dir="$BACKUP_ROOT/.mastodon-backup.lock"
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

log "dumping PostgreSQL database '$DB_NAME' from $DB_HOST:$DB_PORT"
PGPASSWORD=$DB_PASSWORD pg_dump \
  --host="$DB_HOST" \
  --port="$DB_PORT" \
  --username="$DB_USER" \
  --dbname="$DB_NAME" \
  --format=custom \
  --no-owner \
  --no-privileges \
  --file="$work_dir/database.dump"

log "copying Mastodon application configuration"
cp -p "$CONFIG_FILE" "$work_dir/env.production"

if [ "$INCLUDE_MEDIA" = 1 ]; then
  log "archiving Mastodon local uploads from $MEDIA_DIR"
  tar \
    --create \
    --gzip \
    --numeric-owner \
    --file="$work_dir/media.tar.gz" \
    --directory="$MEDIA_DIR" \
    .
fi

if [ "$INCLUDE_REDIS" = 1 ]; then
  log "requesting a synchronous Redis snapshot from $REDIS_HOST:$REDIS_PORT"
  REDISCLI_AUTH=$REDIS_PASSWORD redis-cli \
    --host "$REDIS_HOST" \
    --port "$REDIS_PORT" \
    SAVE >/dev/null

  log "archiving Redis persisted data from $REDIS_DATA_DIR"
  tar \
    --create \
    --gzip \
    --numeric-owner \
    --file="$work_dir/redis.tar.gz" \
    --directory="$REDIS_DATA_DIR" \
    .
fi

created_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
{
  printf 'format_version=1\n'
  printf 'backup_id=%s\n' "$BACKUP_ID"
  printf 'created_at_utc=%s\n' "$created_at"
  printf 'database_name=%s\n' "$DB_NAME"
  printf 'database_format=postgresql-custom\n'
  printf 'config_file=env.production\n'
  printf 'media_included=%s\n' "$INCLUDE_MEDIA"
  printf 'redis_included=%s\n' "$INCLUDE_REDIS"
  printf 'mastodon_version=%s\n' "${MASTODON_VERSION:-unknown}"
} > "$work_dir/manifest.txt"

(
  cd "$work_dir"
  sha256sum database.dump env.production manifest.txt > SHA256SUMS
  if [ "$INCLUDE_MEDIA" = 1 ]; then
    sha256sum media.tar.gz >> SHA256SUMS
  fi
  if [ "$INCLUDE_REDIS" = 1 ]; then
    sha256sum redis.tar.gz >> SHA256SUMS
  fi
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
