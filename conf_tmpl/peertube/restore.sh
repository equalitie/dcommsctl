#!/bin/bash

set -euo pipefail
umask 077

printf "Restore started"

backup_dir="$BACKUP_PATH"
completed=0
restore_started=0
restart_needed=0
temp_dir=""

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  set +e

  if (( completed && restart_needed )); then
    if ! docker compose start peertube-back; then
      printf 'ERROR: failed to restart peertube-back.\n' >&2
      if (( status == 0 )); then status=1; fi
    fi
  elif (( restore_started )); then
    printf 'Restore failed; peertube-back will not be restarted automatically.\n' >&2
  fi

  if [[ -n "$temp_dir" ]]; then
    if ! rm -rf -- "$temp_dir"; then
      printf 'ERROR: failed to remove temporary files.\n' >&2
      if (( status == 0 )); then status=1; fi
    fi
  fi
  exit "$status"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Match the backup script's lock when restoring from its final directory.
exec 9> "$(dirname -- "$backup_dir")/.backup.lock"
if ! flock -n 9; then
  printf 'Another backup or restore is running.\n' >&2
  exit 1
fi

for filename in pgdump.sql.gz redis.rdb.gz content.tar.gz; do
  [[ -r "$backup_dir/$filename" ]] || {
    printf 'Missing or unreadable backup: %s\n' "$backup_dir/$filename" >&2
    exit 1
  }
  gzip -t -- "$backup_dir/$filename"
done
tar -tzf "$backup_dir/content.tar.gz" > /dev/null

container_id="$(docker compose ps -aq peertube-back)"
redis_id="$(docker compose ps -aq peertube-redis)"
for container in "$container_id" "$redis_id"; do
  if [[ -z "$container" || "$container" == *$'\n'* ]]; then
    printf 'Expected exactly one container for each PeerTube service.\n' >&2
    exit 1
  fi
done
was_running="$(docker inspect --format '{{.State.Running}}' "$container_id")"
if [[ "$was_running" == true ]]; then restart_needed=1; fi

redis_env=()
if [[ ${REDISCLI_AUTH+x} ]]; then
  redis_env=(-e "REDISCLI_AUTH=$REDISCLI_AUTH")
fi

redis_config() {
  local reply
  reply="$(docker compose exec -T "${redis_env[@]}" peertube-redis \
    redis-cli --raw CONFIG GET "$1")"
  if [[ "$reply" != "$1"$'\n'* ]]; then
    printf 'Cannot read Redis configuration %s: %s\n' "$1" "$reply" >&2
    return 1
  fi
  printf '%s' "${reply#*$'\n'}"
}

redis_dir="$(redis_config dir)"
redis_filename="$(redis_config dbfilename)"
if [[ "$(redis_config appendonly)" != no ]]; then
  printf 'This RDB restore requires Redis appendonly=no. No data has been changed.\n' >&2
  exit 1
fi
if [[ "$redis_dir" != /* || -z "$redis_filename" ||
      "$redis_filename" == */* || "$redis_filename" == . || "$redis_filename" == .. ]]; then
  printf 'Unexpected Redis snapshot path.\n' >&2
  exit 1
fi

# The configured Redis directory must be on a persistent container mount.
mounts="$(docker inspect --format '{{range .Mounts}}{{println .Destination}}{{end}}' "$redis_id")"
redis_mounted=0
while IFS= read -r mount_path; do
  if [[ -n "$mount_path" &&
        ( "$redis_dir" == "$mount_path" || "$redis_dir" == "$mount_path/"* ) ]]; then
    redis_mounted=1
  fi
done <<< "$mounts"
if (( ! redis_mounted )); then
  printf 'Redis snapshot directory is not on a persistent mount: %s\n' "$redis_dir" >&2
  exit 1
fi

# Cache this image before stopping services so extraction needs no download.
docker run --rm alpine:latest true

temp_dir="$(mktemp -d)"
gzip -dc -- "$backup_dir/redis.rdb.gz" > "$temp_dir/redis.rdb"
if [[ "$(head -c 5 "$temp_dir/redis.rdb")" != REDIS ]]; then
  printf 'redis.rdb.gz does not contain a Redis RDB snapshot.\n' >&2
  exit 1
fi

# pg_dumpall also creates roles. Skip only CREATE ROLE statements whose exact
# quoted names already exist; keep ALTER ROLE and all database restoration SQL.
docker compose exec -T peertube-postgres sh -eu -c '
  export PGPASSWORD="${POSTGRES_PASSWORD:-}"
  exec psql -X -At -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres \
    -c "SELECT format('\''CREATE ROLE %I;'\'', rolname) FROM pg_roles"
' > "$temp_dir/existing-roles.sql"

restore_started=1
docker compose stop peertube-back

docker compose exec -T peertube-postgres sh -eu -c '
  export PGPASSWORD="${POSTGRES_PASSWORD:-}"
  exec dropdb --force --if-exists -U "$POSTGRES_USER" "$POSTGRES_DB"
'

gzip -dc -- "$backup_dir/pgdump.sql.gz" | \
  awk 'FILENAME != "-" { existing[$0] = 1; next }
       /^\\connect / { roles_done = 1 }
       /^CREATE ROLE / && !roles_done && ($0 in existing) { next }
       { print }' "$temp_dir/existing-roles.sql" - | \
  docker compose exec -T peertube-postgres sh -eu -c '
    export PGPASSWORD="${POSTGRES_PASSWORD:-}"
    exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres
  ' > /dev/null

# Extract from stdin; the host backup directory is never mounted in Docker.
docker run --rm -i \
  --volumes-from "$container_id:rw" \
  alpine:latest sh -eu -c '
    test -d /data
    rm -rf /data/* /data/.[!.]* /data/..?*
    tar -xzpf - -C /
  ' < "$backup_dir/content.tar.gz"

# Stop Redis first: its shutdown may save the current, unwanted dataset.
docker compose stop peertube-redis
docker run --rm -i \
  --volumes-from "$redis_id:rw" \
  alpine:latest sh -eu -c '
    directory=$1
    target="$directory/$2"
    test -d "$directory"
    if [ -f "$target" ]; then
      owner=$(stat -c "%u:%g" "$target")
    else
      owner=$(stat -c "%u:%g" "$directory")
    fi
    temporary=$(mktemp "$directory/.restore-rdb.XXXXXX")
    trap '\''rm -f -- "$temporary"'\'' EXIT
    cat > "$temporary"
    chown "$owner" "$temporary"
    chmod 600 "$temporary"
    mv -f -- "$temporary" "$target"
  ' sh "$redis_dir" "$redis_filename" < "$temp_dir/redis.rdb"

docker compose start peertube-redis

# Wait for Redis to finish loading before starting PeerTube.
redis_ready=0
for (( attempt=0; attempt<60; attempt++ )); do
  if reply="$(docker compose exec -T "${redis_env[@]}" peertube-redis \
      redis-cli --raw PING 2>/dev/null)" && [[ "$reply" == PONG ]]; then
    redis_ready=1
    break
  fi
  sleep 1
done
if (( ! redis_ready )); then
  printf 'Redis did not become ready; inspect its container logs.\n' >&2
  exit 1
fi

completed=1

printf "Restore done"
