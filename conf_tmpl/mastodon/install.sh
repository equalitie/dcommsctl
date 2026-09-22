#!/bin/bash

set -e

#Mastodon's config file requires a number of keys to be generated. We spin up a temporary container to do this.
#Volume must be removed before running
docker volume rm masto_data_tmp 2> /dev/null || true

printf "[DCOMMS State] Generating mastodon config\n"

SECRET_KEY_BASE=$(docker run --rm \
    --mount type=volume,src=masto_data_tmp,dst=/opt/mastodon \
        -e RUBYOPT=-W0 tootsuite/mastodon:$VERSION \
    bundle exec rails secret) >/dev/null

OTP_SECRET=$(docker run --rm \
    --mount type=volume,src=masto_data_tmp,dst=/opt/mastodon \
        -e RUBYOPT=-W0 tootsuite/mastodon:$VERSION \
    bundle exec rails secret) >/dev/null

VAPID_KEYS=$(docker run --rm \
    --mount type=volume,src=masto_data_tmp,dst=/opt/mastodon \
        -e RUBYOPT=-W0 tootsuite/mastodon:$VERSION \
    bundle exec rails mastodon:webpush:generate_vapid_key)>/dev/null
VAPID_FRIENDLY_KEYS=${VAPID_KEYS//$'\n'/\\$'\n'}

ACTIVE_RECORD_ENCRYPTION=$(docker run --rm \
    --mount type=volume,src=masto_data_tmp,dst=/opt/mastodon \
        -e RUBYOPT=-W0 tootsuite/mastodon:$VERSION \
    bundle exec rake db:encryption:init | tail -3)>/dev/null
ACTIVE_RECORD_ENCRYPTION_FRIENDLY_KEYS=${ACTIVE_RECORD_ENCRYPTION//$'\n'/\\$'\n'}

#REDIS_PW=$(openssl rand -base64 12)

sed -i "s/SECRET_KEY_BASE=/&$SECRET_KEY_BASE/" ./env
sed -i "s/OTP_SECRET=/&$OTP_SECRET/" ./env
sed -i "s/VAPID_KEYS=/$VAPID_FRIENDLY_KEYS/" ./env
sed -i "s/ACTIVE_RECORD=/$ACTIVE_RECORD_ENCRYPTION_FRIENDLY_KEYS/" ./env
sed -i 's/\r$//g' ./env
#sed -i "s/REDIS_PASSWORD=/&$REDIS_PW/" ./env

printf "[DCOMMS State] Initializing mastodon database\n"

docker compose run --entrypoint="bundle exec rails db:create" --rm mastodon-web
docker compose run --entrypoint="bundle exec rake db:prepare" --rm mastodon-web
docker compose run --entrypoint="bundle exec rake db:migrate" --rm mastodon-web

docker volume rm -f masto_data_tmp

printf "[DCOMMS State] Mastodon installed\n"
