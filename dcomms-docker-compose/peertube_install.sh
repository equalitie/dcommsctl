#!/bin/bash

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

printf "${YELLOW}## Generating Peertube config${NC}\n"

PEERTUBE_SECRET=$(openssl rand -hex 32)

cp $DCOMMS_DIR/conf/peertube/template-environment $DCOMMS_DIR/conf/peertube/environment
sed -i "s/REPLACEME/$DWEB_DOMAIN/" $DCOMMS_DIR/conf/peertube/environment
sed -i "s/PEERTUBE_SECRET=/PEERTUBE_SECRET=$PEERTUBE_SECRET/" $DCOMMS_DIR/conf/peertube/environment

docker compose -f ./conf/compose/peertube.docker-compose.yml -d up

printf "${GREEN} Peertube installed ${NC}\n"
