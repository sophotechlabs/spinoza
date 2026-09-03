#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
context="$work/input"
output="$work/output"
mkdir -p "$context" "$output"
cp "$root/.dockerignore" "$context/.dockerignore"
touch "$context/source.go"
mkdir -p "$context/internal/kubeconfig"
touch "$context/internal/kubeconfig/source.go"

credentials=(
    .env
    .env.local
    .npmrc
    .netrc
    .pypirc
    .aws/credentials
    .azure/accessTokens.json
    .config/gcloud/application_default_credentials.json
    .docker/config.json
    .kube/config
    kubeconfig
    kubeconfig.dev
    cluster.kubeconfig
    id_rsa
    id_dsa
    id_ecdsa
    id_ed25519
    client.pem
    client.key
    client.p12
    client.pfx
    client.jks
    frontend/.env
    frontend/.env.production
    frontend/.npmrc
)

for path in "${credentials[@]}"; do
    mkdir -p "$context/$(dirname "$path")"
    touch "$context/$path"
done

docker buildx build \
    --file "$root/test/docker-context.Dockerfile" \
    --output "type=local,dest=$output" \
    "$context" > /dev/null

if [ ! -f "$output/context/source.go" ]; then
    echo "Docker context excluded required source" >&2
    exit 1
fi

if [ ! -f "$output/context/internal/kubeconfig/source.go" ]; then
    echo "Docker context excluded the kubeconfig source package" >&2
    exit 1
fi

for path in "${credentials[@]}"; do
    if [ -e "$output/context/$path" ]; then
        echo "Docker context included credential path $path" >&2
        exit 1
    fi
done
