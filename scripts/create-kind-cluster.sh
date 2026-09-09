#!/usr/bin/env bash
set -euo pipefail

name=$1
shift
retry_delay_seconds=5
if [ -n "${KIND_CREATE_RETRY_DELAY_SECONDS:-}" ]; then
    retry_delay_seconds=$KIND_CREATE_RETRY_DELAY_SECONDS
fi

for attempt in 1 2 3; do
    if kind create cluster --name "$name" "$@"; then
        exit 0
    fi
    kind delete cluster --name "$name"
    if [ "$attempt" -lt 3 ]; then
        sleep "$((attempt * retry_delay_seconds))"
    fi
done
exit 1
