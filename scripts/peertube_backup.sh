#!/bin/bash

set -oeu pipefail

# FIXME: pass instance name
final_dir="$BACKUP_ROOT/peertube/$BACKUP_ID"
work_dir="$final_dir.partial"
completed=0

cleanup() {
  status=$?

  # NOTE: can I just run this and not store if containers was stopped
  docker compose start peertube-back 2>/dev/null

  if [ "$completed" -ne 1 ] && [ -n "${work_dir:-}" ] && [ -d "$work_dir" ]; then
    rm -rf -- "$work_dir"
  fi

  exit "$status"
}

trap cleanup EXIT

# maybe we should check and prevent backup if the final dir exists
# but for now having $final_dir means backup was succesful and this one is not yet
rm -rf "$final_dir"

mkdir -p "$work_dir"

docker compose stop peertube-back

docker compose exec -T peertube-postgres sh -c \
  'PGPASSWORD=$POSTGRES_PASSWORD psql \
    -U "$POSTGRES_USER" \
    -d "$POSTGRES_DB" \
    -v ON_ERROR_STOP=1' \
  > /dev/null <<'SQL'
DO $$
BEGIN
  WHILE EXISTS (
    SELECT 1
    FROM pg_stat_activity
    WHERE datname = current_database()
      AND pid <> pg_backend_pid()
      AND backend_type = 'client backend'
  ) LOOP
    PERFORM pg_sleep(1);
  END LOOP;
END
$$;
SQL

docker compose exec -T peertube-postgres sh -c \
  'PGPASSWORD=$POSTGRES_PASSWORD pg_dumpall 2>/dev/null \
    -U $POSTGRES_USER \
    -l $POSTGRES_DB' | \
  gzip > "$work_dir/pgump.sql.gz"

# FIXME: figure out auth, works for default setup with an empty password
docker compose exec -T peertube-redis sh -c \
  'redis-cli SAVE' | \
  gzip > "$work_dir/redis.rdb.gz"

docker run --rm \
  --user "$(id -u):$(id -g)" \
  --volumes-from $(./dcomms_compose.sh ps -aq peertube-back) \
  -v "$work_dir:/backup" \
  alpine:latest \
  tar czf /backup/content.tar.gz /data

chmod 600 "$work_dir"/*
mv -- "$work_dir" "$final_dir"

completed=1
