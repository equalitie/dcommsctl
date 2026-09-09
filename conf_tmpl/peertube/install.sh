#!/bin/sh

set -voeu pipefail

printf "[DCOMMS State] Generating Peertube config\n"

PEERTUBE_SECRET=$(openssl rand -hex 32)

printf "PEERTUBE_SECRET=$PEERTUBE_SECRET" >> env

printf "[DCOMMS State] Starting peertube app\n"

docker compose up -d

printf "[DCOMMS State] Performing peertube post-install\n"

docker compose exec -u peertube peertube-back sh -c 'printf "%s\n" "$PEERTUBE_ADMIN_PASS" | npm run reset-password -- -u root'

printf "[DCOMMS State] Configuring SSO\n"

docker compose exec -T -u peertube peertube-back \
  npm run plugin:install -- \
  --npm-name peertube-plugin-auth-openid-connect

docker compose restart peertube-back

curl -fsS -X PUT \
  -H "Authorization: Bearer $PEERTUBE_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  "$PEERTUBE_URL/api/v1/plugins/peertube-plugin-auth-openid-connect/settings" \
  --data '{
    "settings": {
      "client-id": "AUTH_CLIENT_ID",
      "client-secret": "AUTH_CLIENT_SECRET",
      "issuer": "https://id.example.com/realms/example"
    }
  }'

printf "[DCOMMS State] Peertube post-install done\n"

