#!/bin/bash
DCOMMS_DATA="${DCOMMS_DATA:-$PWD}"

root="$DCOMMS_DATA/.instances"

set -uo pipefail

for tool in docker jq; do
    command -v "$tool" >/dev/null || {
        printf 'Missing required command: %s\n' "$tool" >&2
        exit 1
    }
done
docker compose version >/dev/null || exit 1
docker info >/dev/null || exit 1

shopt -s nullglob dotglob
failed=0
found=0

for dir in "$root"/*/; do
    [[ -f "$dir/docker-compose.yml" ]] || continue
    found=$((found + 1))

    # A subshell keeps each project's working directory and failures isolated.
    (
        set -euo pipefail
        cd -- "$dir"
        compose=(docker compose -f ./docker-compose.yml)
        config=$("${compose[@]}" config --format json)
        project=$(jq -er '.name | select(type == "string" and length > 0)' <<< "$config")
        [[ "$project" =~ ^[a-z0-9][a-z0-9_-]*$ ]] || {
            printf 'Invalid project name: %s\n' "$project" >&2
            exit 1
        }

        printf '\nCleaning project %s in %s\n' "$project" "$PWD"
        "${compose[@]}" -p "$project" down --volumes --remove-orphans

        # down only knows the current YAML. Labels also find leftovers from
        # previous versions of this same project, even after resource renames.
        cleanup_failed=0
        for kind in volume network; do
            resources=$(docker "$kind" ls \
                --filter "label=com.docker.compose.project=$project" \
                --format '{{.Name}}')

            while IFS= read -r resource; do
                [[ -n "$resource" ]] || continue

                # Protect current external declarations, even if a resource
                # still carries an old label belonging to this project.
                if jq -e --arg section "${kind}s" --arg name "$resource" '
                    (.[$section] // {}) | to_entries |
                    any(.[]; .value.external == true and
                        ((.value.name // .key) == $name))
                ' <<< "$config" >/dev/null; then
                    printf 'Keeping external %s: %s\n' "$kind" "$resource"
                    continue
                fi

                # No force: Docker refuses to remove resources still in use.
                printf 'Removing leftover %s: %s\n' "$kind" "$resource"
                if ! docker "$kind" rm "$resource"; then
                    printf 'Could not remove %s %s\n' "$kind" "$resource" >&2
                    cleanup_failed=1
                fi
            done <<< "$resources"
        done
        exit "$cleanup_failed"
    )
    result=$?
    if (( result != 0 )); then
        printf 'Cleanup incomplete for %s; continuing.\n' "$dir" >&2
        failed=1
    fi
done

rm -rf "$root"

if (( found == 0 )); then
    printf 'No immediate subfolders containing docker-compose.yml in %s\n' "$root"
fi
exit "$failed"
