#!/usr/bin/env bash
set -euo pipefail

report=${1:?Trivy report path is required}
targets=(
    deploy/helm/spinoza/templates/deployment.yaml
    deploy/helm/spinoza/templates/rbac.yaml
    deploy/helm/spinoza/templates/secret.yaml
    deploy/helm/spinoza/templates/service.yaml
    deploy/helm/spinoza/templates/serviceaccount.yaml
)

for target in "${targets[@]}"; do
    if ! jq -e --arg target "$target" '
        (.Results // []) |
        any(.Target == $target and .Class == "config" and .Type == "helm")
    ' "$report" > /dev/null; then
        echo "Trivy did not scan expected Helm target $target" >&2
        exit 1
    fi
done

if jq -e '.. | strings | select(test("skipping chart"; "i"))' "$report" > /dev/null; then
    echo "Trivy reported that it skipped a Helm chart" >&2
    exit 1
fi
