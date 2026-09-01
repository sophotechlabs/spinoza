#!/usr/bin/env bash
set -euo pipefail

root=$(mktemp -d)
trap 'rm -rf "$root"' EXIT

repo="$root/repo"
mkdir -p "$repo"
git -C "$repo" init -q
git -C "$repo" config user.name Test
git -C "$repo" config user.email test@example.com
printf '{".":"1.0.0"}\n' > "$repo/.release-please-manifest.json"
git -C "$repo" add .release-please-manifest.json
git -C "$repo" commit -qm 'chore: begin'

printf '{".":"1.0.1"}\n' > "$repo/.release-please-manifest.json"
git -C "$repo" add .release-please-manifest.json
git -C "$repo" commit -qm 'chore(main): release 1.0.1'
release=$(git -C "$repo" rev-parse HEAD)
git -C "$repo" commit --allow-empty -qm 'fix: later work'

script=$(cd "$(dirname "$0")/.." && pwd)/scripts/release-commit.sh
found=$(cd "$repo" && "$script")
test "$found" = "$release"

empty="$root/empty"
mkdir -p "$empty"
git -C "$empty" init -q
if (cd "$empty" && "$script" >/dev/null 2>&1); then
    exit 1
fi
