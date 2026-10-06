#!/bin/bash

pushd .instances/caddy
docker compose down --timeout 0 --volumes
popd

# Bash; replace these with your project directories.
for dir in .instances; do
  (
    cd "$dir" &&
    docker compose down --timeout 0 --volumes --remove-orphans
    rm -rf "$dir"
  ) &
done
wait
