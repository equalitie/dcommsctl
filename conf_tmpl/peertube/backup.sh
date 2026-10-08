#!/bin/bash

set -euo pipefail
umask 077

final_dir="$BACKUP_ROOT/$BACKUP_ID"
work_dir="$final_dir.partial"
work_created=0
completed=0
restart_needed=0

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  set +e

  if (( restart_needed )); then
    if ! docker compose start peertube-back; then
      printf 'ERROR: failed to restart peertube-back.\n' >&2
      if (( status == 0 )); then status=1; fi
    fi
  fi

  if (( work_created && ! completed )); then
    if ! rm -rf -- "$work_dir"; then
      printf 'ERROR: failed to remove %s\n' "$work_dir" >&2
      if (( status == 0 )); then status=1; fi
    fi
  fi

  exit "$status"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir -p -- "$BACKUP_ROOT"

if [[ -e "$final_dir" || -L "$final_dir" ]]; then
  printf 'Refusing to overwrite existing backup: %s\n' "$final_dir" >&2
  exit 1
fi

container_id="$(docker compose ps -aq peertube-back)"
if [[ -z "$container_id" || "$container_id" == *$'\n'* ]]; then
  printf 'Expected exactly one peertube-back container.\n' >&2
  exit 1
fi
was_running="$(docker inspect --format '{{.State.Running}}' "$container_id")"

# An existing staging directory is left untouched; use a new BACKUP_ID.
mkdir -- "$work_dir"
work_created=1

if [[ "$was_running" == true ]]; then
  # Set before stopping so cleanup also handles an interrupted stop.
  restart_needed=1
  docker compose stop peertube-back
fi

# Keep the cluster-wide SQL dump, including roles, from the original script.
# pg_dump uses a consistent snapshot; waiting for all clients is unnecessary.
docker compose exec -T peertube-postgres sh -eu -c '
  export PGPASSWORD="${POSTGRES_PASSWORD:-}"
  exec pg_dumpall -U "$POSTGRES_USER" -l "$POSTGRES_DB"
' | gzip > "$work_dir/pgdump.sql.gz"

# Optional authentication: set REDISCLI_AUTH in the script environment.
# Otherwise redis-cli inherits any authentication configured in the container.
redis_env=()
if [[ ${REDISCLI_AUTH+x} ]]; then
  redis_env=(-e "REDISCLI_AUTH=$REDISCLI_AUTH")
fi

# --rdb - writes the actual binary snapshot to stdout.
docker compose exec -T "${redis_env[@]}" peertube-redis \
  redis-cli --rdb - | gzip > "$work_dir/redis.rdb.gz"

# The host shell writes the archive, avoiding bind-mount UID mapping issues.
docker run --rm \
  --volumes-from "$container_id:ro" \
  alpine:latest \
  tar -czf - -C / data \
  > "$work_dir/content.tar.gz"

chmod 600 "$work_dir"/*

# GNU mv: do not nest into or overwrite a destination created in the meantime.
mv -Tn -- "$work_dir" "$final_dir"
if [[ -e "$work_dir" ]]; then
  printf 'Refusing to overwrite existing backup: %s\n' "$final_dir" >&2
  exit 1
fi
completed=1

printf 'Backup complete: %s\n' "$final_dir"
