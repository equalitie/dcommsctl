#!/bin/sh

set -eu

umask 077

source ./helpers.sh

completed=0
work_dir="$BACKUP_ROOT/.$BACKUP_ID.partial.$$"

trap 0 cleanup

REDIS_DATA_DIR=/data

log "requesting a Redis snapshot from $REDIS_HOST:$REDIS_PORT"
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

completed=1
