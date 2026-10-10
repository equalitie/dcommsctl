#!/bin/bash

INSTANCE_ID="$(basename "$PWD")"

printf "[ERSATZ] Writing the config file\n"

cat > ersatz.yml <<EOF
message: ersatz instance ${INSTANCE_ID}
bg_color: "#0b1020"
EOF

printf "[ERSATZ] Starting containers\n"

docker compose up --wait

printf "[ERSATZ] Install complete\n"
