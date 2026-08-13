#!/bin/sh

BACKUP_ROOT=/backups

BACKUP_ID=${BACKUP_ID:-peertube-$(date -u '+%Y%m%dT%H%M%SZ')}

final_dir="$BACKUP_ROOT/$BACKUP_ID"
work_dir="$BACKUP_ROOT/.$BACKUP_ID.partial.$$"

STORAGE_DIR=/data
CONFIG_DIR=/config

mkdir -p "$work_dir"

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

