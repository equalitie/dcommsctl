#!/bin/sh

set -eu

umask 077

source ./helpers.sh

if ! which pg_dump tar sha256sum date cp redis-cli >/dev/null; then
        printf "${RED}## Some dependencies are not installed\n"
        printf "Please install: pg_dump tar sha256sum date cp\n"
fi

need_command() {
  which "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

require_readable_directory() {
  [ -d "$1" ] || die "directory does not exist: $1"
  [ -r "$1" ] || die "directory is not readable: $1"
}

BACKUP_ROOT=${BACKUP_ROOT:-/backups}
MEDIA_DIR=${MEDIA_DIR:-/source/system}
require_readable_directory "$MEDIA_DIR"

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

if ! mkdir "$lock_dir" 2>/dev/null; then
  die "another backup appears to be running; remove stale lock only after checking: $lock_dir"
fi
lock_acquired=1
printf 'pid=%s\nbackup_id=%s\n' "$$" "$BACKUP_ID" > "$lock_dir/owner"

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
