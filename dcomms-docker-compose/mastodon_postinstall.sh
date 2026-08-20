#!/bin/bash

bash dcomms_compose.sh exec mastodon-sidekiq sh -c 'printf "%s\n" "$MASTODON_ADMIN_PASS" | npm run reset-password -- -u root'
