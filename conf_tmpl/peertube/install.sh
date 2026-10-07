#!/bin/sh

set -oeu pipefail

set -a
source ./.env || exit 1
set +a

cleanup() {
    docker compose down
}

trap cleanup EXIT ERR

printf "[DCOMMS State] Generating Peertube config\n"

PEERTUBE_SECRET=$(openssl rand -hex 32)

printf "PEERTUBE_SECRET=$PEERTUBE_SECRET" >> .env

printf "[DCOMMS State] Starting peertube app\n"

docker compose up --wait

printf "[DCOMMS State] Configuring SSO\n"

docker compose exec -T -u peertube peertube-back \
  npm run plugin:install -- \
  --npm-name peertube-plugin-auth-openid-connect
docker compose restart peertube-back

# pinging it after restart to make sure it's running
ready=false
for ((attempt = 0; attempt < 60; attempt++)); do
  if docker compose exec -T peertube-back \
    curl -fsS \
      -H "Host: ${PEERTUBE_WEBSERVER_HOSTNAME}:${PEERTUBE_WEBSERVER_PORT}" \
      'http://127.0.0.1:9000/api/v1/oauth-clients/local' \
      >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 2
done

if [[ "$ready" != true ]]; then
  echo 'PeerTube did not become ready within about two minutes.' >&2
  docker compose logs --tail=100 peertube-back
  exit 1
fi

client_json=$(docker compose exec -T peertube-back curl -fsS \
  -H "Host: $PEERTUBE_WEBSERVER_HOSTNAME:${PEERTUBE_WEBSERVER_PORT}" \
  "http://127.0.0.1:9000/api/v1/oauth-clients/local")

client_id=$(jq -er '.client_id // empty' <<< "$client_json")
client_secret=$(jq -er '.client_secret // empty' <<< "$client_json")

# fetching admin token
token_json=$(
  printf '%s' "$PT_INITIAL_ROOT_PASSWORD" |
  docker compose exec peertube-back curl -fsS \
    -H "Host: ${PEERTUBE_WEBSERVER_HOSTNAME}:${PEERTUBE_WEBSERVER_PORT}" \
    "http://127.0.0.1:9000/api/v1/users/token" \
    --data-urlencode "client_id=$client_id" \
    --data-urlencode "client_secret=$client_secret" \
    --data-urlencode 'grant_type=password' \
    --data-urlencode 'response_type=code' \
    --data-urlencode 'username=root' \
    --data-urlencode 'password@-'
)

PEERTUBE_ADMIN_TOKEN=$(jq -er '.access_token // empty' <<< "$token_json")

docker compose exec peertube-back curl -fsS -X PUT \
  -H "Host: ${PEERTUBE_WEBSERVER_HOSTNAME}:${PEERTUBE_WEBSERVER_PORT}" \
  -H "Authorization: Bearer $PEERTUBE_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  "http://127.0.0.1:9000/api/v1/plugins/peertube-plugin-auth-openid-connect/settings" \
  --data '{
    "settings": {
      "auth-display-name": "dComms Single Sign-On",
      "discover-url": "https://dawa.localhost:3443/.well-known/openid-configuration",
      "client-id": "$AUTH_CLIENT_ID",
      "client-secret": "$AUTH_CLIENT_SECRET",
      "scope": "openid profile"
    }
  }'

printf "[DCOMMS State] Peertube SSO configuration done\n"

