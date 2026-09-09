#!/usr/bin/env bash
set -euo pipefail

verdict=$1
branch=ci/nightly
status=.tmp/nightly/status.json

notify=false
if [ -f "$status" ]; then
    notify=$(jq -r '.notify' "$status")
fi

for label in nightly:green nightly:regression; do
    gh label create "$label" --color ededed --description "Nightly CI verdict" --force >/dev/null
done

git config user.name 'github-actions[bot]'
git config user.email '41898282+github-actions[bot]@users.noreply.github.com'
git config credential.helper "$PWD/scripts/nightly-credential-helper.sh"
git checkout -B "$branch"
git add docs/ci/nightly
if git diff --cached --quiet; then
    echo "nightly: the report is unchanged"
    exit 0
fi
git commit --message "chore(ci): nightly report for $(date -u +%F)"
git push --force origin "HEAD:refs/heads/$branch"

number=$(gh pr list --head "$branch" --state open --json number --jq '.[0].number // ""')
if [ -z "$number" ]; then
    gh pr create \
        --base main \
        --head "$branch" \
        --title "Nightly CI report" \
        --body-file docs/ci/nightly/report.md \
        --label "$verdict"
    echo "nightly: opened the pull request"
    exit 0
fi

other=nightly:green
if [ "$verdict" = nightly:green ]; then
    other=nightly:regression
fi
gh pr edit "$number" --body-file docs/ci/nightly/report.md --add-label "$verdict" --remove-label "$other"
if [ "$notify" = true ]; then
    gh pr comment "$number" --body-file docs/ci/nightly/report.md
    echo "nightly: commented on $number, the failing set changed"
fi
echo "nightly: updated pull request $number"
