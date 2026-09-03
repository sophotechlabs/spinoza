#!/usr/bin/env bash
set -euo pipefail

report=$1
max_not_covered=$2

if ! grep -Eq '"test_efficacy":[[:space:]]*100([.]0+)?[[:space:]]*[,}]' "$report"; then
    echo "mutation: test efficacy is below 100 percent or missing" >&2
    exit 10
fi

not_covered=$(sed -nE 's/.*"mutants_not_covered":[[:space:]]*([0-9]+).*/\1/p' "$report")
if [[ ! "$not_covered" =~ ^[0-9]+$ ]]; then
    echo "mutation: uncovered mutant count is missing" >&2
    exit 11
fi

if ((not_covered > max_not_covered)); then
    echo "mutation: uncovered mutants increased from $max_not_covered to $not_covered" >&2
    exit 11
fi
