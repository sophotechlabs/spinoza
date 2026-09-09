#!/usr/bin/env bash
set -euo pipefail

dir=$1
out=$2
expected_jobs=${3:-}
selected_groups=${4:-}
total_groups=${5:-}

if [ ! -d "$dir" ]; then
    echo "e2e-cover: $dir does not exist; the suite did not run" >&2
    exit 1
fi

unclean=$(find "$dir" -name '*.unclean' | sort)
if [ -n "$unclean" ]; then
    echo "e2e-cover: instances were killed before they could write their counters:" >&2
    while IFS= read -r marker; do
        printf '  %s: %s\n' "$marker" "$(cat "$marker")" >&2
    done <<< "$unclean"
    exit 1
fi

inputs=()
while IFS= read -r meta; do
    inputs+=("$(dirname "$meta")")
done < <(find "$dir" -name 'covmeta.*' | sort)
if [ "${#inputs[@]}" -eq 0 ]; then
    echo "e2e-cover: no counter files under $dir; was the binary built with -cover?" >&2
    exit 1
fi

joined=$(IFS=,; printf '%s' "${inputs[*]}")
go tool covdata textfmt -i="$joined" -o "$out"
report=$(go tool covdata percent -i="$joined")
total=$(awk 'NR > 1 { all += $2; if ($3 > 0) { hit += $2 } } END { if (all == 0) { print "0.0%" } else { printf "%.1f%%", 100 * hit / all } }' "$out")

partial=""
lines=()
if [ -n "$expected_jobs" ]; then
    contributed=$(find "$dir" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')
    lines+=("$contributed of $expected_jobs browser jobs contributed coverage.")
    if [ "$contributed" -lt "$expected_jobs" ]; then
        partial="partial"
    fi
fi
if [ -n "$selected_groups" ] && [ -n "$total_groups" ]; then
    lines+=("$selected_groups of $total_groups groups were selected for this change.")
    if [ "$selected_groups" -lt "$total_groups" ]; then
        partial="partial"
    fi
fi
lines+=("${#inputs[@]} instrumented instances were merged.")

heading="E2E Go coverage: $total"
if [ -n "$partial" ]; then
    heading="E2E Go coverage (partial): $total"
fi
printf '%s\n' "$heading"
printf '%s\n' "${lines[@]}"
printf '%s\n' "$report"

fence='```'
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    {
        printf '## %s\n\n' "$heading"
        printf '%s\n' "${lines[@]}"
        printf '\n%s\n%s\n%s\n' "$fence" "$report" "$fence"
    } >> "$GITHUB_STEP_SUMMARY"
fi
