#!/usr/bin/env bash
set -euo pipefail

window_hours=20
if [ -n "${NIGHTLY_WINDOW_HOURS:-}" ]; then
    window_hours=$NIGHTLY_WINDOW_HOURS
fi

event=${GITHUB_EVENT_NAME:-}
if [ "$event" = workflow_dispatch ] || [ "$event" = repository_dispatch ]; then
    echo "nightly: $event asked for a run" >&2
    echo true
    exit 0
fi

lookback=30
runs=$(gh run list --workflow nightly.yaml --status success --limit "$lookback" --json databaseId,updatedAt --jq '.[] | "\(.databaseId) \(.updatedAt)"')
last=""
while read -r id updated; do
    if [ -z "$id" ]; then
        continue
    fi
    ran=$(gh run view "$id" --json jobs --jq '[.jobs[] | select(.name != "guard" and .conclusion == "success")] | length')
    if [ "$ran" -gt 0 ]; then
        last=$updated
        break
    fi
done <<< "$runs"
if [ -z "$last" ]; then
    echo "nightly: none of the last ${lookback} successful runs got past the guard" >&2
    echo true
    exit 0
fi

if ! finished=$(date -u -d "$last" +%s 2>/dev/null); then
    finished=$(date -u -j -f '%Y-%m-%dT%H:%M:%SZ' "$last" +%s)
fi
now=$(date -u +%s)
age_hours=$(((now - finished) / 3600))
if [ "$age_hours" -ge "$window_hours" ]; then
    echo "nightly: the last run finished ${age_hours}h ago, past the ${window_hours}h window" >&2
    echo true
    exit 0
fi

echo "nightly: the last run finished ${age_hours}h ago, inside the ${window_hours}h window" >&2
echo false
