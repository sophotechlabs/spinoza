#!/usr/bin/env bash
set -euo pipefail

root=$(mktemp -d)
trap 'rm -rf "$root"' EXIT

repo="$root/repo"
bin="$root/bin"
mkdir -p "$repo" "$bin"
git -C "$repo" init -q
git -C "$repo" config user.name Test
git -C "$repo" config user.email test@example.com
printf '{".":"1.0.0"}\n' > "$repo/.release-please-manifest.json"
git -C "$repo" add .release-please-manifest.json
git -C "$repo" commit -qm 'chore: begin'

cat > "$bin/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$GH_CALLS"
if [[ "$*" == *'/git/ref/tags/'* ]]; then
    if [ -n "${EXISTING_SHA:-}" ]; then
        printf '%s\n' "$EXISTING_SHA"
        exit 0
    fi
    exit 1
fi
EOF
chmod +x "$bin/gh"

cat > "$bin/jq" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
sed -n 's/.*"\."[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$3"
EOF
chmod +x "$bin/jq"

export PATH="$bin:$PATH"
export REPO=example/spinoza
export GH_CALLS="$root/gh-calls"
script=$(cd "$(dirname "$0")/.." && pwd)/scripts/tag-release.sh

commit() {
    local version=$1
    local subject=$2
    printf '{".":"%s"}\n' "$version" > "$repo/.release-please-manifest.json"
    git -C "$repo" add .release-please-manifest.json
    git -C "$repo" commit --allow-empty -qm "$subject"
    SHA=$(git -C "$repo" rev-parse HEAD)
    export SHA
}

run() {
    : > "$GH_CALLS"
    (cd "$repo" && "$script")
}

commit 1.0.1 'fix(server): ordinary manifest edit'
run
test ! -s "$GH_CALLS"

commit 1.0.1 'chore(main): release 1.0.1'
if run; then
    echo 'accepted a release commit that did not change the manifest' >&2
    exit 1
fi
test ! -s "$GH_CALLS"

commit 1.0.2 'chore(main): release 1.0.3'
if run; then
    echo 'accepted a release whose subject and manifest disagree' >&2
    exit 1
fi
test ! -s "$GH_CALLS"

commit 1.0.3 'chore(main): release 1.0.3'
EXISTING_SHA="$SHA" run
test "$(wc -l < "$GH_CALLS" | tr -d ' ')" = 1

EXISTING_SHA=deadbeef
export EXISTING_SHA
if run; then
    echo 'accepted a tag that points to another commit' >&2
    exit 1
fi
test "$(wc -l < "$GH_CALLS" | tr -d ' ')" = 1
unset EXISTING_SHA

run
test "$(wc -l < "$GH_CALLS" | tr -d ' ')" = 2
grep -q 'ref=refs/tags/v1.0.3' "$GH_CALLS"
grep -q "sha=$SHA" "$GH_CALLS"
