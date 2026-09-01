#!/usr/bin/env bash
set -euo pipefail

release_subject='^chore\(main\): release ([0-9]+\.[0-9]+\.[0-9]+)( \(#[0-9]+\))?$'
subject=$(git log -1 --format=%s "$SHA")
manifest=$(jq -r '.["."]' .release-please-manifest.json)
if [[ "$subject" =~ $release_subject ]]; then
    released=${BASH_REMATCH[1]}
    if [ "$manifest" != "$released" ]; then
        echo "release commit names $released but the manifest names $manifest" >&2
        exit 1
    fi
    if ! git diff-tree --no-commit-id --name-only -r "$SHA" | grep -qx '.release-please-manifest.json'; then
        echo "release commit did not change .release-please-manifest.json" >&2
        exit 1
    fi
fi

release_sha=
while IFS=$'\t' read -r candidate candidate_subject; do
    if [[ ! "$candidate_subject" =~ $release_subject ]]; then
        continue
    fi
    if [ "${BASH_REMATCH[1]}" != "$manifest" ]; then
        continue
    fi
    release_sha=$candidate
    break
done < <(git log --format=$'%H\t%s' -- .release-please-manifest.json)

if [ -z "$release_sha" ]; then
    echo "no release commit found for v$manifest"
    exit 0
fi

tag="v$manifest"
existing=$(gh api "repos/$REPO/git/ref/tags/$tag" --jq .object.sha 2>/dev/null || true)
if [ -n "$existing" ]; then
    if [ "$existing" != "$release_sha" ]; then
        echo "$tag points to $existing, not $release_sha" >&2
        exit 1
    fi
    echo "$tag already points to this release commit"
    exit 0
fi

gh api "repos/$REPO/git/refs" -f "ref=refs/tags/$tag" -f "sha=$release_sha"
