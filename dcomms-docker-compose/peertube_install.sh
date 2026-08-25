#!/bin/sh

APP_DIR=

log_path=./install.partial.log
final_log_path=./install.log

printf "[DCOMMS State] Generating Peertube config\n"

PEERTUBE_SECRET=$(openssl rand -hex 32)

cp $DCOMMS_DIR/conf/peertube/template-environment $APP_DIR/environment

sed -i "s/REPLACEME/$DWEB_DOMAIN/" $APP_DIR/environment
sed -i "s/PEERTUBE_SECRET=/PEERTUBE_SECRET=$PEERTUBE_SECRET/" $APP_DIR/environment

printf "[DCOMMS State] Peertube pre-install done\n"

printf "[DCOMMS State] Starting peertube app\n"

docker compose up -d

printf "[DCOMMS State] Performing peertube post-install\n"

docker compose exec -u peertube peertube-back sh -c 'printf "%s\n" "$PEERTUBE_ADMIN_PASS" | npm run reset-password -- -u root'

cp $log_path $final_log_path

printf "[DCOMMS State] Peertube post-install done\n"


