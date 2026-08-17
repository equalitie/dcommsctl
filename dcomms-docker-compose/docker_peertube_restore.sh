#!/bin/bash

completed=0

source ./helpers.sh

set -xeu

mkdir -p $work_dir

bash ./dcomms_compose.sh stop peertube-back

# sleep 1

gunzip < $BACKUP_DIR/pgdump.sql.gz | \
    bash ./dcomms_compose.sh exec peertube-postgres pgsql

sh ./dcomms_compose.sh stop peertube-back

completed=1
