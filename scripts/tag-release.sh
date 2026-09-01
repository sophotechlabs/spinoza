#!/usr/bin/env bash
set -euo pipefail

subject=$(git log -1 --format=%s "$SHA")
if [[ ! "$subject" =~ ^chore\(main\):\ release\ ([0-9]+\.[0-9]+\.[0-9]+)(\ \(#[0-9]+\))?$ ]]; then
    echo "not a release commit"
    exit 0
fi

released=${BASH_REMATCH[1]}
manifest=$(jq -r '.["."]' .release-please-manifest.json)
if [ "$manifest" != "$released" ]; then
    echo "release commit names $released but the manifest names $manifest" >&2
    exit 1
fi

if ! git diff-tree --no-commit-id --name-only -r "$SHA" | grep -qx '.release-please-manifest.json'; then
    echo "release commit did not change .release-please-manifest.json" >&2
    exit 1
fi

tag="v$released"
existing=$(gh api "repos/$REPO/git/ref/tags/$tag" --jq .object.sha 2>/dev/null || true)
if [ -n "$existing" ]; then
    if [ "$existing" != "$SHA" ]; then
        echo "$tag points to $existing, not $SHA" >&2
        exit 1
    fi
    echo "$tag already points to this release commit"
    exit 0
fi

gh api "repos/$REPO/git/refs" -f "ref=refs/tags/$tag" -f "sha=$SHA"
