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

# Validate PostgreSQL before inspecting or mutating restore targets. No
# alternative environment-variable names or libpq defaults are accepted.
require_non_empty DB_HOST "${DB_HOST-}"
require_non_empty DB_PORT "${DB_PORT-}"
require_non_empty DB_NAME "${DB_NAME-}"
require_non_empty DB_USER "${DB_USER-}"
require_non_empty DB_PASSWORD "${DB_PASSWORD-}"
require_port DB_PORT "$DB_PORT"

manifest_value() {
  awk -F= -v wanted="$1" '$1 == wanted { sub(/^[^=]*=/, ""); print; exit }' "$backup_dir/manifest.txt"
}

verify_checksum() {
  file_name=$1
  expected=$(awk -v wanted="$file_name" '$2 == wanted { print $1; exit }' "$backup_dir/SHA256SUMS")
  [ -n "$expected" ] || die "no checksum found for $file_name"
  actual=$(sha256sum "$backup_dir/$file_name" | awk '{ print $1 }')
  [ "$actual" = "$expected" ] || die "checksum mismatch: $file_name"
}

safe_target_directory() {
  label=$1
  path=$2

  [ -d "$path" ] || die "$label directory does not exist: $path"
  [ -w "$path" ] || die "$label directory is not writable: $path"

  resolved=$(readlink -f "$path")
  case "$resolved" in
    ''|/) die "refusing unsafe $label directory: $path" ;;
  esac

  printf '%s\n' "$resolved"
}

safe_target_file() {
  label=$1
  path=$2

  [ -f "$path" ] || die "$label file does not exist: $path"
  [ -w "$path" ] || die "$label file is not writable: $path"

  resolved=$(readlink -f "$path")
  case "$resolved" in
    ''|/) die "refusing unsafe $label file: $path" ;;
  esac

  printf '%s\n' "$resolved"
}

paths_overlap() {
  first=$1
  second=$2

  [ "$first" = "$second" ] && return 0
  case "$first/" in "$second/"*) return 0 ;; esac
  case "$second/" in "$first/"*) return 0 ;; esac
  return 1
}

is_mountpoint() {
  awk -v wanted="$1" '$5 == wanted { found = 1 } END { exit(found ? 0 : 1) }' /proc/self/mountinfo
}

clear_directory() {
  target=$1
  find "$target" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
}

extract_archive() {
  archive=$1
  target=$2

  if [ "${RESTORE_PRESERVE_OWNERSHIP:-1}" = 1 ]; then
    tar \
      --extract \
      --gzip \
      --numeric-owner \
      --preserve-permissions \
      --file="$archive" \
      --directory="$target"
  else
    tar \
      --extract \
      --gzip \
      --no-same-owner \
      --preserve-permissions \
      --file="$archive" \
      --directory="$target"
  fi
}

BACKUP_ROOT=${BACKUP_ROOT:-/backups}
MEDIA_DIR=${MEDIA_DIR:-/restore/system}
CONFIG_FILE=${CONFIG_FILE:-/restore/.env.production}
REDIS_DATA_DIR=${REDIS_DATA_DIR:-/restore/redis}
RESTORE_MEDIA=${RESTORE_MEDIA:-1}
RESTORE_CONFIG=${RESTORE_CONFIG:-1}
RESTORE_REDIS=${RESTORE_REDIS:-1}
REQUIRE_MOUNTPOINTS=${REQUIRE_MOUNTPOINTS:-1}
backup_selector=${1:-latest}

require_boolean RESTORE_MEDIA "$RESTORE_MEDIA"
require_boolean RESTORE_CONFIG "$RESTORE_CONFIG"
require_boolean RESTORE_REDIS "$RESTORE_REDIS"
require_boolean REQUIRE_MOUNTPOINTS "$REQUIRE_MOUNTPOINTS"

if [ "$backup_selector" = latest ]; then
  [ -r "$BACKUP_ROOT/LATEST" ] || die "latest-backup pointer not found: $BACKUP_ROOT/LATEST"
  IFS= read -r backup_id < "$BACKUP_ROOT/LATEST"
else
  backup_id=$backup_selector
fi

case "$backup_id" in
  ''|.|..|latest|LATEST|.mastodon-backup.lock|*[!A-Za-z0-9_.-]*)
    die "backup name is invalid or reserved"
    ;;
esac

backup_dir="$BACKUP_ROOT/$backup_id"
[ -d "$backup_dir" ] || die "backup does not exist: $backup_dir"

need_command awk
need_command sha256sum
need_command tar
need_command pg_restore
need_command date
need_command readlink
need_command find
need_command tr
need_command cat

for required_file in database.dump env.production manifest.txt SHA256SUMS; do
  [ -f "$backup_dir/$required_file" ] || die "backup is missing $required_file"
  [ -r "$backup_dir/$required_file" ] || die "backup file is not readable: $required_file"
done

[ "$(manifest_value format_version)" = 1 ] || die "unsupported or missing backup format version"
[ "$(manifest_value backup_id)" = "$backup_id" ] || die "manifest backup ID does not match directory name"

media_included=$(manifest_value media_included)
redis_included=$(manifest_value redis_included)
require_boolean media_included "$media_included"
require_boolean redis_included "$redis_included"

if [ "$media_included" = 1 ]; then
  [ -f "$backup_dir/media.tar.gz" ] || die "backup is missing media.tar.gz"
  [ -r "$backup_dir/media.tar.gz" ] || die "backup file is not readable: media.tar.gz"
fi

if [ "$redis_included" = 1 ]; then
  [ -f "$backup_dir/redis.tar.gz" ] || die "backup is missing redis.tar.gz"
  [ -r "$backup_dir/redis.tar.gz" ] || die "backup file is not readable: redis.tar.gz"
fi

log "verifying backup checksums"
verify_checksum database.dump
verify_checksum env.production
verify_checksum manifest.txt

if [ "$media_included" = 1 ]; then
  verify_checksum media.tar.gz
fi

if [ "$redis_included" = 1 ]; then
  verify_checksum redis.tar.gz
fi

log "checking archive and database-dump structure"
pg_restore --list "$backup_dir/database.dump" >/dev/null

if [ "$media_included" = 1 ]; then
  tar --list --gzip --file="$backup_dir/media.tar.gz" >/dev/null
fi

if [ "$redis_included" = 1 ]; then
  tar --list --gzip --file="$backup_dir/redis.tar.gz" >/dev/null
fi

if [ "${RESTORE_DRY_RUN:-0}" = 1 ]; then
  log "backup verification succeeded: $backup_dir"
  exit 0
fi

[ "${RESTORE_CONFIRM:-}" = YES_I_UNDERSTAND ] || die "set RESTORE_CONFIRM=YES_I_UNDERSTAND to perform the destructive restore"

backup_real=$(readlink -f "$backup_dir")
media_real=
config_real=
redis_real=

if [ "$RESTORE_MEDIA" = 1 ] && [ "$media_included" = 1 ]; then
  media_real=$(safe_target_directory media "$MEDIA_DIR")
  paths_overlap "$media_real" "$backup_real" && die "media target overlaps the backup"
fi

if [ "$RESTORE_CONFIG" = 1 ]; then
  config_real=$(safe_target_file configuration "$CONFIG_FILE")
  paths_overlap "$config_real" "$backup_real" && die "configuration target overlaps the backup"
fi

if [ "$RESTORE_REDIS" = 1 ] && [ "$redis_included" = 1 ]; then
  redis_real=$(safe_target_directory Redis "$REDIS_DATA_DIR")
  paths_overlap "$redis_real" "$backup_real" && die "Redis target overlaps the backup"
fi

if [ -n "$media_real" ] && [ -n "$config_real" ]; then
  paths_overlap "$media_real" "$config_real" && die "media and configuration targets overlap"
fi
if [ -n "$media_real" ] && [ -n "$redis_real" ]; then
  paths_overlap "$media_real" "$redis_real" && die "media and Redis targets overlap"
fi
if [ -n "$config_real" ] && [ -n "$redis_real" ]; then
  paths_overlap "$config_real" "$redis_real" && die "configuration and Redis targets overlap"
fi

if [ "$REQUIRE_MOUNTPOINTS" = 1 ]; then
  [ -r /proc/self/mountinfo ] || die "cannot verify restore mounts; set REQUIRE_MOUNTPOINTS=0 only if intentional"
  [ -z "$media_real" ] || is_mountpoint "$media_real" || die "media target is not a distinct mount point: $media_real"
  [ -z "$config_real" ] || is_mountpoint "$config_real" || die "configuration target is not a distinct mount point: $config_real"
  [ -z "$redis_real" ] || is_mountpoint "$redis_real" || die "Redis target is not a distinct mount point: $redis_real"
fi

need_command psql

if [ "${ALLOW_ACTIVE_DB_CONNECTIONS:-0}" != 1 ]; then
  active_connections=$(PGPASSWORD=$DB_PASSWORD psql \
    --host="$DB_HOST" \
    --port="$DB_PORT" \
    --username="$DB_USER" \
    --dbname="$DB_NAME" \
    --no-psqlrc \
    --tuples-only \
    --no-align \
    --command="SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND backend_type = 'client backend' AND pid <> pg_backend_pid()" | tr -d '[:space:]')
  case "$active_connections" in
    ''|*[!0-9]*) die "could not determine active database connection count" ;;
  esac
  [ "$active_connections" -eq 0 ] || die "database has $active_connections other active connection(s); stop Mastodon or set ALLOW_ACTIVE_DB_CONNECTIONS=1"
fi

if [ -n "$redis_real" ] && [ "${ALLOW_RUNNING_REDIS:-0}" != 1 ]; then
  need_command redis-cli
  need_command grep
  require_non_empty REDIS_HOST "${REDIS_HOST-}"
  require_non_empty REDIS_PORT "${REDIS_PORT-}"
  require_set REDIS_PASSWORD "${REDIS_PASSWORD+x}"
  require_port REDIS_PORT "$REDIS_PORT"
  if REDISCLI_AUTH=$REDIS_PASSWORD redis-cli --host "$REDIS_HOST" --port "$REDIS_PORT" PING 2>/dev/null | grep -q PONG; then
    die "Redis is still running; stop it before restoring or set ALLOW_RUNNING_REDIS=1"
  fi
fi

if [ -n "$media_real" ]; then
  log "replacing local uploads in $media_real"
  clear_directory "$media_real"
  extract_archive "$backup_dir/media.tar.gz" "$media_real"
fi

if [ -n "$config_real" ]; then
  log "replacing Mastodon application configuration"
  cat "$backup_dir/env.production" > "$config_real"
fi

if [ -n "$redis_real" ]; then
  log "replacing Redis persisted data in $redis_real"
  clear_directory "$redis_real"
  extract_archive "$backup_dir/redis.tar.gz" "$redis_real"
fi

log "restoring PostgreSQL database '$DB_NAME' on $DB_HOST:$DB_PORT"
PGPASSWORD=$DB_PASSWORD pg_restore \
  --host="$DB_HOST" \
  --port="$DB_PORT" \
  --username="$DB_USER" \
  --clean \
  --if-exists \
  --exit-on-error \
  --no-owner \
  --no-privileges \
  --single-transaction \
  --schema=public \
  --dbname="$DB_NAME" \
  "$backup_dir/database.dump"

log "restore completed from: $backup_dir"
