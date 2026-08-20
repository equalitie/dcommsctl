#!/bin/bash

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

MAS_V='v4.5'

DCOMMS_DIR=$PWD

set -e

#Mastodon's config file requires a number of keys to be generated. We spin up a temporary container to do this.
#Volume must be removed before running
docker volume rm masto_data_tmp 2> /dev/null || true
printf "${YELLOW}## Generating mastodon config${NC}\n"
sudo cp -a $DCOMMS_DIR/conf/mastodon/example.env.production $DCOMMS_DIR/conf/mastodon/env.production
SECRET_KEY_BASE=$(docker run --rm \
    --mount type=volume,src=masto_data_tmp,dst=/opt/mastodon \
        -e RUBYOPT=-W0 tootsuite/mastodon:$MAS_V \
    bundle exec rails secret) >/dev/null

OTP_SECRET=$(docker run --rm \
    --mount type=volume,src=masto_data_tmp,dst=/opt/mastodon \
        -e RUBYOPT=-W0 tootsuite/mastodon:$MAS_V \
    bundle exec rails secret) >/dev/null

VAPID_KEYS=$(docker run --rm \
    --mount type=volume,src=masto_data_tmp,dst=/opt/mastodon \
        -e RUBYOPT=-W0 tootsuite/mastodon:$MAS_V \
    bundle exec rails mastodon:webpush:generate_vapid_key)>/dev/null
VAPID_FRIENDLY_KEYS=${VAPID_KEYS//$'\n'/\\$'\n'}

ACTIVE_RECORD_ENCRYPTION=$(docker run --rm \
    --mount type=volume,src=masto_data_tmp,dst=/opt/mastodon \
        -e RUBYOPT=-W0 tootsuite/mastodon:$MAS_V \
    bundle exec rake db:encryption:init | tail -3)>/dev/null
ACTIVE_RECORD_ENCRYPTION_FRIENDLY_KEYS=${ACTIVE_RECORD_ENCRYPTION//$'\n'/\\$'\n'}

#REDIS_PW=$(openssl rand -base64 12)

sed -i "s/REPLACEME/$DWEB_DOMAIN/" $DCOMMS_DIR/conf/mastodon/env.production
sed -i "s/SECRET_KEY_BASE=/&$SECRET_KEY_BASE/" $DCOMMS_DIR/conf/mastodon/env.production
sed -i "s/OTP_SECRET=/&$OTP_SECRET/" $DCOMMS_DIR/conf/mastodon/env.production
sed -i "s/VAPID_KEYS=/$VAPID_FRIENDLY_KEYS/" $DCOMMS_DIR/conf/mastodon/env.production
sed -i "s/ACTIVE_RECORD=/$ACTIVE_RECORD_ENCRYPTION_FRIENDLY_KEYS/" $DCOMMS_DIR/conf/mastodon/env.production
sed -i 's/\r$//g' $DCOMMS_DIR/conf/mastodon/env.production
sed -i "s/ALTERNATE_DOMAINS=social./&$DWEB_ONION/" $DCOMMS_DIR/conf/mastodon/env.production
sed -i "s/SMTP_SERVER=/&$DWEB_DOMAIN/" $DCOMMS_DIR/conf/mastodon/env.production
#sed -i "s/REDIS_PASSWORD=/&$REDIS_PW/" $DCOMMS_DIR/conf/mastodon/env.production

printf "${YELLOW}## Initializing mastodon database${NC}\n"

docker compose -f ./conf/compose/mastodon.docker-compose.yml run --entrypoint="bundle exec rails db:create" --rm mastodon-web
docker compose -f ./conf/compose/mastodon.docker-compose.yml run --entrypoint="bundle exec rake db:prepare" --rm mastodon-web
docker compose -f ./conf/compose/mastodon.docker-compose.yml run --entrypoint="bundle exec rake db:migrate" --rm mastodon-web

docker volume rm -f masto_data_tmp
docker compose -f ./conf/compose/mastodon.docker-compose.yml -d up

printf "${GREEN} Mastodon installed ${NC}\n"
