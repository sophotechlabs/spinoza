#!/usr/bin/env bash
set -euo pipefail

reports=$1
max_default=$2
max_desktop=$3
common=0
default_root=
desktop_root=
found=0

while IFS= read -r -d '' report; do
    name=$(basename "$report")
    not_covered=$(sed -nE 's/.*"mutants_not_covered":[[:space:]]*([0-9]+).*/\1/p' "$report")
    if [[ ! "$not_covered" =~ ^[0-9]+$ ]]; then
        echo "mutation: uncovered mutant count is missing from $name" >&2
        exit 11
    fi
    found=$((found + 1))
    case "$name" in
        root-default-root.json)
            default_root=$not_covered
            ;;
        root-desktop-root.json)
            desktop_root=$not_covered
            ;;
        *)
            common=$((common + not_covered))
            ;;
    esac
done < <(find "$reports" -type f -name '*.json' -print0)

if [ "$found" -eq 0 ]; then
    echo "mutation: no package reports found" >&2
    exit 11
fi
if [[ ! "$default_root" =~ ^[0-9]+$ ]]; then
    echo "mutation: default root report is missing" >&2
    exit 11
fi
if [[ ! "$desktop_root" =~ ^[0-9]+$ ]]; then
    echo "mutation: desktop root report is missing" >&2
    exit 11
fi

default_total=$((common + default_root))
desktop_total=$((common + desktop_root))
printf 'mutation: default uncovered %d/%d, desktop uncovered %d/%d\n' \
    "$default_total" "$max_default" "$desktop_total" "$max_desktop"
if [ "$default_total" -gt "$max_default" ]; then
    exit 11
fi
if [ "$desktop_total" -gt "$max_desktop" ]; then
    exit 12
fi
