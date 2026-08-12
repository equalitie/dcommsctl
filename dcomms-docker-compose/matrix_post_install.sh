#!/bin/sh
register_new_matrix_user -u dcomms-admin --admin -c /data/homeserver.yaml
curl -X POST --header "Authorization: Bearer syt_abc123" http://localhost:8008/_synapse/admin/v1/registration_tokens/new -d '{}'
