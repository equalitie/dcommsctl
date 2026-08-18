#!/bin/bash

bash dcomms_compose.sh exec -u peertube peertube-back sh -c 'printf "%s\n" "$PEERTUBE_ADMIN_PASS" | npm run reset-password -- -u root'
