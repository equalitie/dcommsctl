#!/usr/bin/env bash
set -euo pipefail

# Run from the Compose project directory.
# Assumes a valid pg_dumpall/RDB/media backup, one container per service,
# a dedicated PostgreSQL instance with superuser DB_USER, and an official
# Redis image using /data/dump.rdb with appendonly=no.
backup_dir=$(cd -- "$BACKUP_ROOT/mastodon/$BACKUP_ID" && pwd -P)
app_services=(mastodon-web mastodon-streaming mastodon-sidekiq)
restart_services=()

# Remember which services to restart; no container-count validation.
for service in "${app_services[@]}"; do
  state=$(docker compose ps --all --format '{{.State}}' "$service")
  if [[ "$state" == running ]]; then
    restart_services+=("$service")
  fi
done

sidekiq_id=$(docker compose ps --all --quiet mastodon-sidekiq)
redis_id=$(docker compose ps --all --quiet mastodon-redis)
redis_image=$(docker inspect --format '{{.Image}}' "$redis_id")

printf "[DCOMMS State] Stopping mastodon services\n"

docker compose stop --timeout 120 mastodon-web mastodon-streaming
docker compose stop --timeout 120 mastodon-sidekiq
docker compose stop --timeout 120 mastodon-redis

printf "[DCOMMS State] Restoring mastodon database\n"
# pg_dumpall creates roles that may already exist. Ignore only that error;
# preserve subsequent ALTER ROLE statements and fail on other SQL errors.
# Stop rewriting at the first database section to preserve COPY payloads.
docker compose exec -T mastodon-db sh -eu -c '
  export PGPASSWORD="$DB_PASSWORD"
  exec dropdb -w --if-exists --force -U "$DB_USER" "$DB_NAME"
'
gzip -dc -- "$backup_dir/pgdump.sql.gz" | awk '
  /^-- PostgreSQL database dump$/ { databases = 1 }
  !databases && /^CREATE ROLE / {
    delimiter = "$mastodon_restore_role$"
    while (index($0, delimiter))
      delimiter = "$mastodon_restore_role_" ++suffix "$"
    print "DO " delimiter " BEGIN " $0 \
      " EXCEPTION WHEN duplicate_object THEN NULL; END; " delimiter ";"
    next
  }
  { print }
' | docker compose exec -T mastodon-db sh -eu -c '
  export PGPASSWORD="$DB_PASSWORD"
  exec psql -X -w -v ON_ERROR_STOP=1 -U "$DB_USER" -d postgres
'

printf "[DCOMMS State] Restoring media database\n"

docker run --rm --network none --user 0 \
  --volumes-from "$sidekiq_id" \
  --mount "type=bind,src=$backup_dir,dst=/backup,readonly" \
  --entrypoint sh "$redis_image" -eu -c '
    cd /mastodon/public/system
    rm -rf -- ./* ./.[!.]* ./..?*
    tar xzf /backup/content.tar.gz --strip-components=3 -C /mastodon/public/system
  '

printf "[DCOMMS State] Restoring redis\n"
# Replace the RDB while Redis is stopped.
docker run --rm --network none --user 0 \
  --volumes-from "$redis_id" \
  --mount "type=bind,src=$backup_dir,dst=/backup,readonly" \
  --entrypoint sh "$redis_image" -eu -c '
    umask 077
    gzip -dc /backup/redis.rdb.gz > /data/dump.rdb.restore
    chown "$(id -u redis):$(id -g redis)" /data/dump.rdb.restore
    chmod 600 /data/dump.rdb.restore
    mv -f /data/dump.rdb.restore /data/dump.rdb
  '

# Wait for Redis to load before restarting the application.
# With set -e, an earlier restore failure never reaches these start commands.
docker compose start mastodon-redis
redis_reply=''
for ((attempt = 0; attempt < 60; attempt++)); do
  if redis_reply=$(docker compose exec -T mastodon-redis redis-cli --raw PING 2>/dev/null) &&
     [[ "$redis_reply" == PONG ]]; then
    break
  fi
  sleep 1
done
[[ "$redis_reply" == PONG ]] || {
  printf '%s\n' 'ERROR: Redis is not ready; Mastodon remains stopped' >&2
  exit 1
}

if ((${#restart_services[@]})); then
  docker compose start "${restart_services[@]}"
fi
printf "[DCOMMS State] Mastodon restore completed\n"
