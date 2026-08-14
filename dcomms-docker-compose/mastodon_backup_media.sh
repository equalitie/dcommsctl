#!/bin/sh

set -eu

umask 077

source ./helpers.sh

completed=0
work_dir="$BACKUP_ROOT/.$BACKUP_ID.partial.$$"

trap cleanup 0

log "archiving Mastodon local uploads from $MEDIA_DIR"
tar \
  --create \
  --gzip \
  --numeric-owner \
  --file="$work_dir/media.tar.gz" \
  --directory="$MEDIA_DIR" \
  .

log "copying Mastodon application configuration"
cp -p "$CONFIG_FILE" "$work_dir/env.production"

completed=1

