#!/usr/bin/env bash
set -euo pipefail

report=$1
max_not_covered=${2:-}

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

if ((lived > 0)) || grep -Eq '"status":[[:space:]]*"LIVED"' "$report"; then
    echo "mutation: one or more mutants survived" >&2
    exit 10
fi

if grep -Eq '"status":[[:space:]]*"(RUNNABLE|TIMED OUT)"' "$report"; then
    echo "mutation: one or more mutants were not completed" >&2
    exit 12
fi

if ((killed == 0 && not_covered == 0)); then
    echo "mutation: report contains no mutants" >&2
    exit 10
fi

if ((killed > 0)) && ! grep -Eq '"test_efficacy":[[:space:]]*100([.]0+)?[[:space:]]*[,}]' "$report"; then
    echo "mutation: test efficacy is below 100 percent or missing" >&2
    exit 10
fi

if [ -n "$max_not_covered" ] && ((not_covered > max_not_covered)); then
    echo "mutation: uncovered mutants increased from $max_not_covered to $not_covered" >&2
    exit 11
fi
