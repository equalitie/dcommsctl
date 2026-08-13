
REDIS_DATA_DIR=${REDIS_DATA_DIR:-/source/redis}
require_readable_directory "$REDIS_DATA_DIR"

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
