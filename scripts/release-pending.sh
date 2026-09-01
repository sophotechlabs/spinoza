#!/usr/bin/env bash
set -euo pipefail

if [ "$GITHUB_EVENT_NAME" != push ]; then
    echo true
    exit 0
fi

if git diff-tree --no-commit-id --name-only -r "$GITHUB_SHA" | grep -qx '.release-please-manifest.json'; then
    echo true
    exit 0
fi

version=$(jq -r '.["."]' .release-please-manifest.json)
draft=$(gh release view "v$version" --json isDraft --jq .isDraft 2>/dev/null || true)
if [ "$draft" = true ]; then
    echo true
    exit 0
fi

echo false
