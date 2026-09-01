#!/usr/bin/env bash
set -euo pipefail

sha=$(git log -1 --format=%H -- .release-please-manifest.json)
if [[ ! "$sha" =~ ^[0-9a-f]{40}$ ]]; then
    echo "the release manifest has no commit" >&2
    exit 1
fi
printf '%s\n' "$sha"
