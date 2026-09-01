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
if [ "${GH_FAIL:-}" = true ]; then
    exit 1
fi
printf '%s\n' "${DRAFT:-false}"
EOF
chmod +x "$bin/gh"

export PATH="$bin:$PATH"
export GH_CALLS="$root/gh-calls"
script=$(cd "$(dirname "$0")/.." && pwd)/scripts/release-pending.sh

run() {
    : > "$GH_CALLS"
    (cd "$repo" && "$script")
}

export GITHUB_EVENT_NAME=workflow_dispatch
GITHUB_SHA=$(git -C "$repo" rev-parse HEAD)
export GITHUB_SHA
test "$(run)" = true
test ! -s "$GH_CALLS"

printf '{".":"1.0.1"}\n' > "$repo/.release-please-manifest.json"
git -C "$repo" add .release-please-manifest.json
git -C "$repo" commit -qm 'chore(main): release 1.0.1 (#1)'
export GITHUB_EVENT_NAME=push
GITHUB_SHA=$(git -C "$repo" rev-parse HEAD)
export GITHUB_SHA
test "$(run)" = true
test ! -s "$GH_CALLS"

git -C "$repo" commit --allow-empty -qm 'fix(server): repair release'
GITHUB_SHA=$(git -C "$repo" rev-parse HEAD)
export GITHUB_SHA
DRAFT=true run > "$root/pending"
test "$(cat "$root/pending")" = true
test "$(wc -l < "$GH_CALLS" | tr -d ' ')" = 1

DRAFT=false run > "$root/pending"
test "$(cat "$root/pending")" = false
test "$(wc -l < "$GH_CALLS" | tr -d ' ')" = 1

GH_FAIL=true run > "$root/pending"
test "$(cat "$root/pending")" = false
test "$(wc -l < "$GH_CALLS" | tr -d ' ')" = 1
