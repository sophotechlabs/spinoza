#!/usr/bin/env bash
set -euo pipefail

report=$1

integer_field() {
    local field=$1
    local value
    value=$(sed -nE "s/.*\"$field\":[[:space:]]*([0-9]+).*/\\1/p" "$report")
    if [[ ! "$value" =~ ^[0-9]+$ ]]; then
        echo "mutation: $field is missing" >&2
        exit 11
    fi
    printf '%s\n' "$value"
}

killed=$(integer_field mutants_killed)
lived=$(integer_field mutants_lived)
not_covered=$(integer_field mutants_not_covered)

if grep -Eq '"status":[[:space:]]*"(RUNNABLE|TIMED OUT)"' "$report"; then
    echo "mutation: one or more mutants were not completed" >&2
    exit 12
fi

if ((killed == 0 && lived == 0 && not_covered == 0)); then
    echo "mutation: report contains no mutants" >&2
    exit 10
fi

printf 'mutation: %s killed %d, survived %d, uncovered %d\n' \
    "$(basename "$report")" "$killed" "$lived" "$not_covered"
