#!/usr/bin/env bash
set -euo pipefail

mode=${1:?prepare or collect is required}
context=${2:?the test cluster context is required}
suite=${3:-cluster-mode-auth}
if [[ ! "$suite" =~ ^cluster-mode-[a-z]+$ ]]; then
    echo "invalid cluster-mode coverage suite: $suite" >&2
    exit 1
fi
kube=(kubectl --context "$context" --namespace spinoza)
stop_server() {
    local deployed
    deployed=$("${kube[@]}" get deployment spinoza --ignore-not-found -o name)
    if [ -n "$deployed" ]; then
        "${kube[@]}" scale deployment/spinoza --replicas=0
        "${kube[@]}" wait --for=delete pod -l app.kubernetes.io/instance=spinoza --timeout=120s
    fi
}
case "$mode" in
    prepare)
        stop_server
        "${kube[@]}" apply -f test/clustermode/counter-volume.yaml
        "${kube[@]}" wait --for=condition=Ready pod/coverage-collector --timeout=120s
        "${kube[@]}" exec coverage-collector -- find /coverage -type f -delete
        ;;
    collect)
        output=".tmp/system-coverage/$suite"
        mkdir -p "$output/counters"
        find "$output/counters" -type f -delete
        rm -f "$output/summary.json" "$output/coverage.out"
        "${kube[@]}" logs deployment/spinoza --all-containers --tail=-1 > "$output/server.log"
        stop_server
        "${kube[@]}" cp coverage-collector:/coverage/. "$output/counters"
        go tool covdata textfmt -i="$output/counters" -o="$output/coverage.out"
        node scripts/system-coverage.mjs "$output/coverage.out" "$suite" "$output/summary.json"
        ;;
    *)
        echo "unsupported cluster-mode coverage action: $mode" >&2
        exit 1
        ;;
esac
