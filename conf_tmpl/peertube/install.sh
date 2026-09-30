#!/bin/sh

set -voeu pipefail

set -a
source ./env
set +a

printf "[DCOMMS State] Generating Peertube config\n"

PEERTUBE_SECRET=$(openssl rand -hex 32)

printf "PEERTUBE_SECRET=$PEERTUBE_SECRET" >> env

printf "[DCOMMS State] Starting peertube app\n"

docker compose up -d

printf "[DCOMMS State] Performing peertube post-install\n"

# TODO: debug that, something broke

printf "[DCOMMS State] Configuring SSO\n"

docker compose exec -T -u peertube peertube-back \
  npm run plugin:install -- \
  --npm-name peertube-plugin-auth-openid-connect
docker compose restart peertube-back

client_json=$(curl -fsS \
  "$PEERTUBE_WEBSERVER_HOSTNAME/api/v1/oauth-clients/local")

client_id=$(jq -er '.client_id // empty' <<< "$client_json")
client_secret=$(jq -er '.client_secret // empty' <<< "$client_json")

# Pass the password through stdin.
token_json=$(
  printf '%s' "$PT_INITIAL_ROOT_PASSWORD" \
    curl -fsS "$base_url/api/v1/users/token" \
      --data-urlencode "client_id=$client_id" \
      --data-urlencode "client_secret=$client_secret" \
      --data-urlencode 'grant_type=password' \
      --data-urlencode 'response_type=code' \
      --data-urlencode 'username=root' \
      --data-urlencode 'password@-'
)

PEERTUBE_ADMIN_TOKEN=$(jq -er '.access_token // empty' <<< "$token_json")

curl -fsS -X PUT \
  -H "Authorization: Bearer $PEERTUBE_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  "$PEERTUBE_WEBSERVER_HOSTNAME/api/v1/plugins/peertube-plugin-auth-openid-connect/settings" \
  --data '{
    "settings": {
      "client-id": "$AUTH_CLIENT_ID",
      "client-secret": "$AUTH_CLIENT_SECRET",
      "issuer": "https://id.example.com/realms/example"
    }
}'

printf "[DCOMMS State] Peertube post-install done\n"

