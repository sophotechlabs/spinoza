#!/usr/bin/env sh
# Proves `just editorconfig` finds the checker under either name it ships as:
# mise installs it as `ec`, the Forgejo toolbox renames it to `editorconfig-checker`.
set -eu

root=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

fake() {
    mkdir -p "$work/bin"
    rm -f "$work/bin/ec" "$work/bin/editorconfig-checker"
    printf '#!/bin/sh\necho ran:%s\n' "$1" > "$work/bin/$1"
    chmod +x "$work/bin/$1"
}

just_bin=$(command -v just)

run() {
    PATH="$work/bin:/usr/bin:/bin" "$just_bin" \
        --justfile "$root/justfile" --working-directory "$root" editorconfig
}

fake ec
got=$(run)
[ "$got" = "ran:ec" ] || { echo "under mise: got '$got', want ran:ec" >&2; exit 1; }

fake editorconfig-checker
got=$(run)
[ "$got" = "ran:editorconfig-checker" ] || {
    echo "under the toolbox: got '$got', want ran:editorconfig-checker" >&2
    exit 1
}

rm -f "$work/bin/ec" "$work/bin/editorconfig-checker"
if run > /dev/null 2>&1; then
    echo "with neither name present it reported success" >&2
    exit 1
fi

echo "editorconfig-name: resolves ec, resolves editorconfig-checker, fails loudly with neither"
