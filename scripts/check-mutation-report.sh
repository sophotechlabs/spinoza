#!/usr/bin/env bash
set -euo pipefail

report=$1

if ! grep -Eq '"test_efficacy":[[:space:]]*100([.]0+)?[[:space:]]*[,}]' "$report"; then
    echo "mutation: test efficacy is below 100 percent or missing" >&2
    exit 10
fi

if ! grep -Eq '"mutations_coverage":[[:space:]]*100([.]0+)?[[:space:]]*[,}]' "$report"; then
    echo "mutation: mutator coverage is below 100 percent or missing" >&2
    exit 11
fi
