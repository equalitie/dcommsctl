#!/bin/sh

set -eu

umask 077

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
    ''|/)
      die "refusing unsafe $label directory: $path"
      ;;
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

extract_archive() {
  archive=$1
  target=$2

    tar \
      --extract \
      --gzip \
      --numeric-owner \
      --preserve-permissions \
      --file="$archive" \
      --directory="$target"
}

BACKUP_ROOT=${BACKUP_ROOT:-/backups}
STORAGE_DIR=${STORAGE_DIR:-/restore/storage}
CONFIG_DIR=${CONFIG_DIR:-/restore/config}
backup_selector=${1:-latest}

if [ "$backup_selector" = latest ]; then
  [ -r "$BACKUP_ROOT/LATEST" ] || die "latest-backup pointer not found: $BACKUP_ROOT/LATEST"
  IFS= read -r backup_id < "$BACKUP_ROOT/LATEST"
else
  backup_id=$backup_selector
fi

case "$backup_id" in
  ''|.|..|latest|LATEST|.peertube-backup.lock|*[!A-Za-z0-9_.-]*)
    die "backup name is invalid or reserved"
    ;;
esac

backup_dir="$BACKUP_ROOT/$backup_id"
[ -d "$backup_dir" ] || die "backup does not exist: $backup_dir"

for required_file in database.dump storage.tar.gz config.tar.gz manifest.txt SHA256SUMS; do
  [ -f "$backup_dir/$required_file" ] || die "backup is missing $required_file"
  [ -r "$backup_dir/$required_file" ] || die "backup file is not readable: $required_file"
done

[ "$(manifest_value format_version)" = 1 ] || die "unsupported or missing backup format version"
[ "$(manifest_value backup_id)" = "$backup_id" ] || die "manifest backup ID does not match directory name"

log "verifying backup checksums"
verify_checksum database.dump
verify_checksum storage.tar.gz
verify_checksum config.tar.gz
verify_checksum manifest.txt

log "checking archive and database-dump structure"
tar --list --gzip --file="$backup_dir/storage.tar.gz" >/dev/null
tar --list --gzip --file="$backup_dir/config.tar.gz" >/dev/null
pg_restore --list "$backup_dir/database.dump" >/dev/null

if [ "${RESTORE_DRY_RUN:-0}" = 1 ]; then
  log "backup verification succeeded: $backup_dir"
  exit 0
fi

[ "${RESTORE_CONFIRM:-}" = YES_I_UNDERSTAND ] || die "set RESTORE_CONFIRM=YES_I_UNDERSTAND to perform the destructive restore"

storage_real=$(safe_target_directory storage "$STORAGE_DIR")
config_real=$(safe_target_directory config "$CONFIG_DIR")
backup_real=$(readlink -f "$backup_dir")

paths_overlap "$storage_real" "$config_real" && die "storage and config targets overlap"
paths_overlap "$storage_real" "$backup_real" && die "storage target overlaps the backup"
paths_overlap "$config_real" "$backup_real" && die "config target overlaps the backup"

if [ "${REQUIRE_MOUNTPOINTS:-1}" = 1 ]; then
  [ -r /proc/self/mountinfo ] || die "cannot verify restore mounts; set REQUIRE_MOUNTPOINTS=0 only if intentional"
  is_mountpoint "$storage_real" || die "storage target is not a distinct mount point: $storage_real"
  is_mountpoint "$config_real" || die "config target is not a distinct mount point: $config_real"
fi

log "replacing files in $storage_real"
rm -rf "$storage_real"/*
extract_archive "$backup_dir/storage.tar.gz" "$storage_real"

log "replacing configuration in $config_real"
rm -rf "$config_real"/*
extract_archive "$backup_dir/config.tar.gz" "$config_real"

log "restore completed from: $backup_dir"
