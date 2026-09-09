#!/usr/bin/env bash
set -euo pipefail

reports=$1
rows=40

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
survivors=$work/survivors
uncovered=$work/uncovered
: >"$survivors"
: >"$uncovered"

killed=0
lived=0
common=0
default_root=
desktop_root=
found=0

while IFS= read -r -d '' report; do
    name=$(basename "$report" .json)
    if ! counts=$(jq -r '[(.mutants_killed // "?"), (.mutants_lived // "?"), (.mutants_not_covered // "?")] | @tsv' "$report" 2>/dev/null); then
        echo "mutation: $name is not a readable report" >&2
        exit 11
    fi
    IFS=$'\t' read -r report_killed report_lived report_not_covered <<<"$counts"
    for count in "$report_killed" "$report_lived" "$report_not_covered"; do
        if [[ ! "$count" =~ ^[0-9]+$ ]]; then
            echo "mutation: $name is missing its mutant counts" >&2
            exit 11
        fi
    done
    found=$((found + 1))
    killed=$((killed + report_killed))
    lived=$((lived + report_lived))
    case "$name" in
        root-default-root)
            default_root=$report_not_covered
            ;;
        root-desktop-root)
            desktop_root=$report_not_covered
            ;;
        *)
            common=$((common + report_not_covered))
            ;;
    esac
    jq -r --arg report "$name" '
        (.files // [])[]
        | .file_name as $file
        | (.mutations // [])[]
        | select(.status == "LIVED")
        | [$report, $file, (.line | tostring), .type]
        | @tsv' "$report" >>"$survivors"
    jq -r --arg report "$name" '
        (.files // [])[]
        | {file: .file_name, count: ([(.mutations // [])[] | select(.status == "NOT COVERED")] | length)}
        | select(.count > 0)
        | [(.count | tostring), $report, .file]
        | @tsv' "$report" >>"$uncovered"
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
printf 'mutation: %d killed, %d survived; uncovered %d default, %d desktop\n' \
    "$killed" "$lived" "$default_total" "$desktop_total"

while IFS=$'\t' read -r report file line mutator; do
    printf '::warning title=Mutant survived::%s changed %s:%s and no test noticed\n' \
        "$mutator" "$file" "$line"
    printf 'mutation: %s survived %s in %s:%s\n' "$report" "$mutator" "$file" "$line" >&2
done <"$survivors"

{
    printf '## Mutation testing\n\n'
    printf '%d mutants killed, %d survived.\n\n' "$killed" "$lived"
    printf 'Mutants no test reaches: %d in the default build, %d in the desktop build.\n\n' \
        "$default_total" "$desktop_total"
    if [ -s "$survivors" ]; then
        printf '### Survived\n\n'
        printf 'A test ran this line, the change altered behaviour, and nothing failed.\n\n'
        printf '| package | file | line | mutator |\n| --- | --- | --- | --- |\n'
        while IFS=$'\t' read -r report file line mutator; do
            printf '| %s | %s | %s | %s |\n' "$report" "$file" "$line" "$mutator"
        done <"$survivors"
        printf '\n'
    fi
    if [ -s "$uncovered" ]; then
        printf '### Not reached\n\n'
        printf 'No test executes these lines, so the mutants there were never run.\n'
        printf 'Go does not instrument const and var declarations or the case expressions of a\n'
        printf 'conditionless switch, so mutants on those lines can never be reached at all.\n\n'
        printf '| mutants | package | file |\n| --- | --- | --- |\n'
        sort -t "$(printf '\t')" -k1,1nr -k2,2 -k3,3 "$uncovered" | head -n "$rows" |
            while IFS=$'\t' read -r count report file; do
                printf '| %s | %s | %s |\n' "$count" "$report" "$file"
            done
        listed=$(wc -l <"$uncovered")
        if [ "$listed" -gt "$rows" ]; then
            printf '\n%d more files are not listed.\n' "$((listed - rows))"
        fi
    fi
} >>"${GITHUB_STEP_SUMMARY:-/dev/stdout}"
