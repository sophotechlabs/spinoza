#!/usr/bin/env bash
set -euo pipefail

root=${1:-.}

minor_of() {
    printf '%s' "$1" | sed -E 's/^([0-9]+)\.([0-9]+)(\.[0-9]+)?$/\1.\2/'
}

pinned=$(sed -nE 's/^kubectl = "([0-9.]+)"$/\1/p' "$root/mise.toml")
shipped=$(sed -nE 's/^ARG KUBECTL_VERSION=([0-9.]+)$/\1/p' "$root/Dockerfile")
range=$(sed -nE 's/.*Kubernetes ([0-9]+\.[0-9]+) to ([0-9]+\.[0-9]+) by skew policy.*/\1 \2/p' "$root/README.md")

if [ -z "$pinned" ] || [ -z "$shipped" ] || [ -z "$range" ]; then
    echo "check-kubectl-skew: could not read the kubectl pins and the README support range (mise=$pinned image=$shipped range=$range)"
    exit 1
fi
if [ "$(minor_of "$pinned")" != "$(minor_of "$shipped")" ]; then
    echo "check-kubectl-skew: mise.toml pins kubectl $pinned but the image ships $shipped; keep them on one minor"
    exit 1
fi

kubectl_minor=${pinned#*.}
kubectl_minor=${kubectl_minor%%.*}
oldest=${range% *}
newest=${range#* }
oldest_minor=${oldest#*.}
newest_minor=${newest#*.}

if [ $((oldest_minor)) -lt $((kubectl_minor - 1)) ] || [ $((newest_minor)) -gt $((kubectl_minor + 1)) ]; then
    echo "check-kubectl-skew: the README promises Kubernetes $oldest to $newest, but kubectl $pinned only covers 1.$((kubectl_minor - 1)) to 1.$((kubectl_minor + 1)) under the skew policy"
    exit 1
fi

echo "check-kubectl-skew: kubectl $pinned covers the promised $oldest to $newest"
