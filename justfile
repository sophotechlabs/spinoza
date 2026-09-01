export PATH := if os() == 'windows' { env_var('PATH') } else { env_var('HOME') + '/go/bin:' + env_var('PATH') }

exe := if os() == 'windows' { '.exe' } else { '' }
go_pkgs := './internal/... ./cmd/... .'
addr := env_var_or_default('SPINOZA_ADDR', '127.0.0.1:34115')
test_cluster := env_var_or_default('SPINOZA_KIND_CLUSTER', 'spinoza')
test_context := 'kind-' + test_cluster
kind_dir := 'test/integration'
kind_merged := '.tmp/kind'
registry_host := 'localhost:5001'
registry_endpoint := 'http://kind-registry:5000'
ldflags := '-s -w'
version_pkg := 'github.com/sophotechlabs/spinoza/internal/version.value'

default:
    @just --list

[private]
app-version:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -n "${SPINOZA_VERSION:-}" ]; then
        echo "${SPINOZA_VERSION}"
        exit 0
    fi
    if [ -n "${GITHUB_REF_NAME:-}" ]; then
        echo "${GITHUB_REF_NAME}"
        exit 0
    fi
    described=$(git describe --tags --always --dirty 2>/dev/null || true)
    if [ -n "$described" ]; then
        echo "$described"
        exit 0
    fi
    echo dev

deps:
    cd frontend && npm ci

tidy:
    go mod tidy

build:
    #!/usr/bin/env bash
    set -euo pipefail
    export SPINOZA_VERSION="$(just app-version)"
    cd frontend && npm run build && cd ..
    go build -trimpath -ldflags "{{ ldflags }} -X {{ version_pkg }}=$SPINOZA_VERSION" -o spinoza{{ exe }} .

stub-assets:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -f web/dist/index.html ]; then
        exit 0
    fi
    mkdir -p web/dist
    echo "stub-assets: writing a placeholder index.html so the go-only recipes can compile"
    cat > web/dist/index.html <<'HTML'
    <!doctype html>
    <html lang="en">
      <head><meta charset="utf-8" /><title>Spinoza</title></head>
      <body>This binary was built without its frontend. Run <code>just build</code>.</body>
    </html>
    HTML

run: build stop
    ./spinoza{{ exe }}

stop:
    #!/usr/bin/env bash
    set -euo pipefail
    listen='{{ addr }}'
    port="${listen##*:}"
    if ! command -v lsof > /dev/null; then
        echo "stop: lsof is missing, so nothing was checked on port $port"
        exit 0
    fi
    holders=$(lsof -ti "tcp:${port}" -sTCP:LISTEN 2>/dev/null || true)
    if [ -z "$holders" ]; then
        exit 0
    fi
    for pid in $holders; do
        command=$(ps -p "$pid" -o args= 2>/dev/null || true)
        case "$command" in
            *spinoza*)
                echo "stop: port $port was held by pid $pid, stopping it"
                kill "$pid" 2>/dev/null || true
                ;;
            *)
                echo "stop: port $port is held by pid $pid, which is not spinoza:"
                echo "  $command"
                exit 1
                ;;
        esac
    done
    for _ in $(seq 1 50); do
        if [ -z "$(lsof -ti "tcp:${port}" -sTCP:LISTEN 2>/dev/null || true)" ]; then
            exit 0
        fi
        sleep 0.1
    done
    echo "stop: port $port is still held after 5s"
    exit 1

build-desktop:
    #!/usr/bin/env bash
    set -euo pipefail
    version=$(just app-version)
    export SPINOZA_VERSION="$version"
    numeric=""
    if printf '%s' "$version" | grep -Eq '^v[0-9]+(\.[0-9]+){0,2}([-+].*)?$'; then
        numeric="${version#v}"
        numeric="${numeric%%[-+]*}"
    fi
    backup=$(mktemp)
    cp wails.json "$backup"
    trap 'mv "$backup" wails.json' EXIT
    if [ -n "$numeric" ]; then
        export PRODUCT_VERSION="$numeric"
        yq -i -o=json '.info.productVersion = strenv(PRODUCT_VERSION)' wails.json
    fi
    wails build -platform darwin/universal -tags desktop -skipbindings -trimpath -ldflags "{{ ldflags }} -X {{ version_pkg }}=$version"
    plist=build/bin/spinoza.app/Contents/Info.plist
    if [ -f "$plist" ]; then
        plutil -lint "$plist"
    fi
    just icns

build-desktop-windows arch='amd64':
    #!/usr/bin/env bash
    set -euo pipefail
    version=$(just app-version)
    export SPINOZA_VERSION="$version"
    numeric=""
    if printf '%s' "$version" | grep -Eq '^v[0-9]+(\.[0-9]+){0,2}([-+].*)?$'; then
        numeric="${version#v}"
        numeric="${numeric%%[-+]*}"
    fi
    backup=$(mktemp)
    cp wails.json "$backup"
    trap 'mv "$backup" wails.json' EXIT
    if [ -n "$numeric" ]; then
        export PRODUCT_VERSION="$numeric"
        yq -i -o=json '.info.productVersion = strenv(PRODUCT_VERSION)' wails.json
    fi
    wails build -platform windows/{{ arch }} -tags desktop -skipbindings -trimpath -ldflags "{{ ldflags }} -X {{ version_pkg }}=$version"

package-desktop-windows arch='amd64': (build-desktop-windows arch)
    #!/usr/bin/env bash
    set -euo pipefail
    version=$(just app-version)
    built=build/bin/Spinoza.exe
    if [ ! -f "$built" ]; then
        echo "package-desktop-windows: $built was not built"
        exit 1
    fi
    staged=$(mktemp -d)
    trap 'rm -rf "$staged"' EXIT
    cp "$built" "$staged/Spinoza.exe"
    cp LICENSE "$staged/LICENSE"
    mkdir -p dist/release
    archive="$PWD/dist/release/spinoza_${version}_windows_{{ arch }}_app.zip"
    rm -f "$archive"
    (cd "$staged" && zip -q -X "$archive" Spinoza.exe LICENSE)
    echo "package-desktop-windows: wrote $archive"

build-desktop-linux arch='amd64':
    #!/usr/bin/env bash
    set -euo pipefail
    version=$(just app-version)
    export SPINOZA_VERSION="$version"
    wails build -platform linux/{{ arch }} -tags desktop,webkit2_41 -skipbindings -trimpath -ldflags "{{ ldflags }} -X {{ version_pkg }}=$version"

package-desktop-linux arch='amd64': (build-desktop-linux arch)
    #!/usr/bin/env bash
    set -euo pipefail
    version=$(just app-version)
    built=build/bin/Spinoza
    if [ ! -f "$built" ]; then
        echo "package-desktop-linux: $built was not built"
        exit 1
    fi
    staged=$(mktemp -d)
    trap 'rm -rf "$staged"' EXIT
    cp "$built" "$staged/Spinoza"
    cp build/appicon.png "$staged/spinoza.png"
    cp LICENSE "$staged/LICENSE"
    mkdir -p dist/release
    archive="$PWD/dist/release/spinoza_${version}_linux_{{ arch }}_app.tar.gz"
    rm -f "$archive"
    reported=$("$built" --version)
    if [ "$reported" != "$version" ]; then
        echo "package-desktop-linux: the app reports $reported, not $version"
        exit 1
    fi
    tar --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - -C "$staged" Spinoza spinoza.png LICENSE | gzip -n > "$archive"
    echo "package-desktop-linux: wrote $archive, built from an app that reports $reported"

icns:
    #!/usr/bin/env bash
    set -euo pipefail
    bundle=build/bin/spinoza.app
    if [ ! -d "$bundle" ]; then
        echo "icns: $bundle is not built yet"
        exit 1
    fi
    work=$(mktemp -d)
    trap 'rm -rf "$work"' EXIT
    iconset="$work/spinoza.iconset"
    mkdir -p "$iconset"
    for size in 16 32 128 256 512; do
        sips -z "$size" "$size" build/appicon.png --out "$iconset/icon_${size}x${size}.png" >/dev/null
        retina=$((size * 2))
        sips -z "$retina" "$retina" build/appicon.png --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
    done
    iconutil -c icns "$iconset" -o "$bundle/Contents/Resources/iconfile.icns"
    touch "$bundle"

rund: build-desktop
    open build/bin/spinoza.app

dev-desktop:
    wails dev -tags desktop -skipbindings

dev-api: stub-assets
    go run . --addr 127.0.0.1:34115

dev-web:
    cd frontend && npm run dev

test-be name='' pkgs=go_pkgs: stub-assets
    #!/usr/bin/env bash
    set -euo pipefail
    name={{ quote(name) }}
    if [ -n "$name" ]; then
        go test -race -shuffle=on -run "$name" {{ pkgs }}
        exit 0
    fi
    go test -race -shuffle=on -covermode=atomic -coverprofile=coverage.out {{ pkgs }}
    go tool cover -func=coverage.out

kind-config tier:
    #!/usr/bin/env bash
    set -euo pipefail
    case '{{ tier }}' in
        base)
            chain=('{{ kind_dir }}/kind.yaml')
            ;;
        e2e)
            chain=('{{ kind_dir }}/kind.yaml' '{{ kind_dir }}/kind-e2e.yaml')
            ;;
        full)
            chain=('{{ kind_dir }}/kind.yaml' '{{ kind_dir }}/kind-e2e.yaml' '{{ kind_dir }}/kind-full.yaml')
            ;;
        *)
            echo "kind-config: {{ tier }} is not one of base, e2e, full"
            exit 1
            ;;
    esac
    mkdir -p {{ kind_merged }}
    yq eval-all '. as $item ireduce ({}; . *+ $item)' "${chain[@]}" > {{ kind_merged }}/{{ tier }}.yaml
    echo "kind-config: {{ kind_merged }}/{{ tier }}.yaml is ${chain[*]} merged, $(yq '.nodes | length' {{ kind_merged }}/{{ tier }}.yaml) nodes"

[private]
cluster-up tier:
    #!/usr/bin/env bash
    set -euo pipefail
    just kind-config {{ tier }}
    config={{ kind_merged }}/{{ tier }}.yaml
    if ! kind get clusters | grep -qx {{ test_cluster }}; then
        kind create cluster --name {{ test_cluster }} --config "$config" --wait 300s
    fi
    kind export kubeconfig --name {{ test_cluster }}
    wanted=$(yq '.nodes | length' "$config")
    running=$(kind get nodes --name {{ test_cluster }} | wc -l | tr -d ' ')
    if [ "$running" != "$wanted" ]; then
        echo "cluster {{ test_cluster }} runs $running nodes, not the $wanted in $config; just cluster-down first"
        exit 1
    fi
    just cluster-mirror
    kubectl --context {{ test_context }} cluster-info
    kubectl --context {{ test_context }} get nodes -L spinoza.test/pool
    kubectl --context {{ test_context }} apply -f {{ kind_dir }}/metrics-server.yaml
    kubectl --context {{ test_context }} -n kube-system rollout status deployment/metrics-server --timeout=5m
    kubectl --context {{ test_context }} wait --for=condition=Available apiservice/v1beta1.metrics.k8s.io --timeout=5m

cluster-base: (cluster-up 'base')

cluster-e2e: (cluster-up 'e2e')

cluster-full: (cluster-up 'full') cluster-gitops cluster-second

[private]
cluster-second:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! kind get clusters | grep -qx {{ test_cluster }}-second; then
        kind create cluster --name {{ test_cluster }}-second --wait 300s
    fi
    kubectl --context kind-{{ test_cluster }}-second cluster-info

[private]
cluster-gitops:
    #!/usr/bin/env bash
    set -euo pipefail
    helm --kube-context {{ test_context }} repo add fluxcd-community https://fluxcd-community.github.io/helm-charts --force-update
    helm --kube-context {{ test_context }} repo add argo https://argoproj.github.io/argo-helm --force-update
    helm --kube-context {{ test_context }} repo update
    helm --kube-context {{ test_context }} upgrade --install flux fluxcd-community/flux2 \
        --namespace flux-system --create-namespace --wait --timeout 10m
    helm --kube-context {{ test_context }} upgrade --install argocd argo/argo-cd \
        --namespace argocd --create-namespace --wait --timeout 10m \
        --set dex.enabled=false --set notifications.enabled=false --set applicationSet.enabled=true
    kubectl --context {{ test_context }} -n flux-system wait --for=condition=Available deployment --all --timeout=10m
    kubectl --context {{ test_context }} -n argocd wait --for=condition=Available deployment --all --timeout=10m

cluster-down:
    #!/usr/bin/env bash
    set -euo pipefail
    kind delete cluster --name {{ test_cluster }}
    if kind get clusters | grep -qx {{ test_cluster }}-second; then
        kind delete cluster --name {{ test_cluster }}-second
    fi

[private]
cluster-mirror:
    #!/usr/bin/env bash
    set -euo pipefail
    certs="/etc/containerd/certs.d/{{ registry_host }}"
    for node in $(kind get nodes --name {{ test_cluster }}); do
        docker exec "$node" mkdir -p "$certs"
        printf '[host."%s"]\n' '{{ registry_endpoint }}' | docker exec -i "$node" cp /dev/stdin "$certs/hosts.toml"
    done

test-integration name='': cluster-base
    #!/usr/bin/env bash
    set -euo pipefail
    export SPINOZA_TEST_CONTEXT={{ test_context }}
    name={{ quote(name) }}
    if [ -n "$name" ]; then
        go test -tags integration -count=1 -timeout 15m -run "$name" ./test/integration/...
        exit 0
    fi
    go test -tags integration -count=1 -timeout 15m -covermode=atomic -coverprofile=coverage.integration.out -coverpkg=./internal/... ./test/integration/...
    go tool cover -func=coverage.integration.out | tail -1

test-e2e name='' spec='': cluster-e2e (e2e-run 'core' name spec)

shots name='' spec='': cluster-down cluster-e2e cluster-second
    #!/usr/bin/env bash
    set -euo pipefail
    SPINOZA_E2E_TIER=shots just e2e-run shots {{ quote(name) }} {{ quote(spec) }}

test-e2e-full name='' spec='': cluster-full
    #!/usr/bin/env bash
    set -euo pipefail
    SPINOZA_E2E_TIER=full just e2e-run full {{ quote(name) }} {{ quote(spec) }}

[private]
e2e-run project name='' spec='':
    #!/usr/bin/env bash
    set -euo pipefail
    export SPINOZA_KIND_CLUSTER='{{ test_cluster }}'
    if [ ! -d frontend/node_modules ] || [ frontend/package-lock.json -nt frontend/node_modules ]; then
        npm --prefix frontend ci
    fi
    cd e2e
    if [ ! -d node_modules ] || [ package-lock.json -nt node_modules ]; then
        npm ci
    fi
    npx playwright install chromium
    run=(test --project={{ project }})
    spec={{ quote(spec) }}
    if [ -n "$spec" ]; then
        run+=("$spec")
    fi
    name={{ quote(name) }}
    if [ -n "$name" ]; then
        run+=(--grep "$name")
    fi
    npx playwright "${run[@]}"

test-fe name='' spec='':
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -d frontend/node_modules ] || [ frontend/package-lock.json -nt frontend/node_modules ]; then
        npm --prefix frontend ci
    fi
    cd frontend
    name={{ quote(name) }}
    spec={{ quote(spec) }}
    if [ -z "$name" ] && [ -z "$spec" ]; then
        npm run test:coverage
        exit 0
    fi
    run=(run)
    if [ -n "$spec" ]; then
        run+=("$spec")
    fi
    if [ -n "$name" ]; then
        run+=(-t "$name")
    fi
    npx vitest "${run[@]}"

test: test-be test-fe

test-repeat name='' pkgs=go_pkgs: stub-assets
    #!/usr/bin/env bash
    set -euo pipefail
    name={{ quote(name) }}
    if [ -n "$name" ]; then
        go test -race -shuffle=on -count=5 -run "$name" {{ pkgs }}
        exit 0
    fi
    go test -race -shuffle=on -count=5 {{ pkgs }}

fuzz-one pkg target tags='' duration='30s': stub-assets
    #!/usr/bin/env bash
    set -euo pipefail
    args=(test {{ quote(pkg) }} -run '^$' -fuzz {{ quote(target) }} -fuzztime {{ quote(duration) }} -timeout 2m)
    tags={{ quote(tags) }}
    if [ -n "$tags" ]; then
        args+=(-tags "$tags")
    fi
    go "${args[@]}"

mutation mode='default' output='dist/mutation/default.json': stub-assets
    #!/usr/bin/env bash
    set -euo pipefail
    mode={{ quote(mode) }}
    output={{ quote(output) }}
    mkdir -p "$(dirname "$output")"
    args=(unleash --output "$output" --output-statuses lc)
    if [ "$mode" = desktop ]; then
        args+=(--tags desktop)
    elif [ "$mode" != default ]; then
        echo "mutation: mode must be default or desktop" >&2
        exit 1
    fi
    gremlins "${args[@]}"

cover-gate: test-be
    go-test-coverage --config .testcoverage.yml

badges: cover-gate test-fe
    #!/usr/bin/env bash
    set -euo pipefail
    color() {
        if [ "$1" -ge 90 ]; then
            echo '#97ca00'
            return
        fi
        if [ "$1" -ge 75 ]; then
            echo '#dfb317'
            return
        fi
        echo '#e05d44'
    }
    endpoint() {
        printf '{"schemaVersion":1,"label":"%s","message":"%s%%","color":"%s"}\n' "$2" "$3" "$(color "$3")" > "dist/badges/$1"
    }
    mkdir -p dist/badges
    out=$(mktemp)
    trap 'rm -f "$out"' EXIT
    GITHUB_OUTPUT="$out" go-test-coverage --config .testcoverage.yml -o > /dev/null
    go_pct=$(grep '^total-coverage=' "$out" | cut -d = -f 2)
    web_pct=$(node -p "Math.floor(require('./frontend/coverage/coverage-summary.json').total.lines.pct)")
    endpoint coverage-go.json 'go coverage' "$go_pct"
    endpoint coverage-web.json 'web coverage' "$web_pct"
    echo "badges: go ${go_pct}%, web ${web_pct}%"

publish-badges:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -f dist/badges/coverage-go.json ] || [ ! -f dist/badges/coverage-web.json ]; then
        echo "publish-badges: dist/badges is empty, run just badges first"
        exit 1
    fi
    if [ -z "${GITHUB_REPOSITORY:-}" ]; then
        echo "publish-badges: GITHUB_REPOSITORY is unset"
        exit 1
    fi
    if [ -z "${GH_TOKEN:-}" ]; then
        echo "publish-badges: GH_TOKEN is unset"
        exit 1
    fi
    work=$(mktemp -d)
    trap 'rm -rf "$work"' EXIT
    cp dist/badges/coverage-go.json dist/badges/coverage-web.json "$work/"
    git -C "$work" init -q -b badges
    git -C "$work" add coverage-go.json coverage-web.json
    git -C "$work" -c user.name='github-actions[bot]' -c user.email='41898282+github-actions[bot]@users.noreply.github.com' commit -q -m 'chore(badges): publish coverage'
    git -C "$work" push -q --force "https://x-access-token:${GH_TOKEN}@github.com/${GITHUB_REPOSITORY}.git" badges:badges

lint-be: stub-assets
    golangci-lint run {{ go_pkgs }}
    golangci-lint run --build-tags desktop {{ go_pkgs }}
    golangci-lint run --build-tags integration ./test/...
    go vet {{ go_pkgs }}
    go vet -tags desktop {{ go_pkgs }}
    go vet -tags integration ./test/...

lint-fe:
    cd frontend && npm run lint
    cd frontend && npm run typecheck
    cd frontend && npm run format:check
    cd frontend && npm run contrast

lint-e2e:
    #!/usr/bin/env bash
    set -euo pipefail
    cd e2e
    if [ ! -d node_modules ] || [ package-lock.json -nt node_modules ]; then
        npm ci
    fi
    npm run typecheck

cm_dir := 'test/clustermode'
cm_cluster := env_var_or_default('SPINOZA_CM_CLUSTER', test_cluster + '-cm')
cm_context := 'kind-' + cm_cluster

# stand up a cluster with an ingress, keycloak and the fixtures cluster mode is verified against
cluster-mode-up:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! kind get clusters | grep -qx {{ cm_cluster }}; then
        kind create cluster --name {{ cm_cluster }} --config {{ cm_dir }}/kind.yaml --wait 300s
    fi
    kind export kubeconfig --name {{ cm_cluster }}
    kubectl --context {{ cm_context }} apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.13.1/deploy/static/provider/kind/deploy.yaml
    kubectl --context {{ cm_context }} -n ingress-nginx rollout status deployment/ingress-nginx-controller --timeout=5m
    kubectl --context {{ cm_context }} -n ingress-nginx wait --for=condition=ready pod \
        --selector=app.kubernetes.io/component=controller --timeout=5m
    for _ in $(seq 1 60); do
        if kubectl --context {{ cm_context }} -n ingress-nginx get endpoints ingress-nginx-controller-admission \
            -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null | grep -q .; then
            break
        fi
        sleep 2
    done
    mkdir -p .tmp/cm
    if [ ! -f .tmp/cm/tls.crt ]; then
        openssl req -x509 -newkey rsa:2048 -nodes -days 30 \
            -keyout .tmp/cm/tls.key -out .tmp/cm/tls.crt \
            -subj "/CN=localtest.me" \
            -addext "subjectAltName=DNS:localtest.me,DNS:spinoza.localtest.me,DNS:keycloak.localtest.me"
    fi
    kubectl --context {{ cm_context }} apply -f {{ cm_dir }}/rbac.yaml
    kubectl --context {{ cm_context }} create namespace spinoza --dry-run=client -o yaml | kubectl --context {{ cm_context }} apply -f -
    for ns in keycloak spinoza; do
        kubectl --context {{ cm_context }} create namespace "$ns" --dry-run=client -o yaml | kubectl --context {{ cm_context }} apply -f -
        kubectl --context {{ cm_context }} -n "$ns" create secret tls localtest-tls \
            --cert=.tmp/cm/tls.crt --key=.tmp/cm/tls.key --dry-run=client -o yaml \
            | kubectl --context {{ cm_context }} apply -f -
    done
    kubectl --context {{ cm_context }} -n keycloak create configmap spinoza-realm \
        --from-file=realm.json={{ cm_dir }}/realm.json --dry-run=client -o yaml \
        | kubectl --context {{ cm_context }} apply -f -
    kubectl --context {{ cm_context }} apply -f {{ cm_dir }}/keycloak.yaml
    kubectl --context {{ cm_context }} apply -f {{ cm_dir }}/shim.yaml
    kubectl --context {{ cm_context }} apply -f {{ cm_dir }}/workloads.yaml
    realm=$(shasum -a 256 {{ cm_dir }}/realm.json | cut -d' ' -f1)
    kubectl --context {{ cm_context }} -n keycloak patch deployment/keycloak --type=merge \
        -p "{\"spec\":{\"template\":{\"metadata\":{\"annotations\":{\"spinoza.test/realm\":\"$realm\"}}}}}"
    kubectl --context {{ cm_context }} -n keycloak rollout status deployment/keycloak --timeout=8m
    kubectl --context {{ cm_context }} -n keycloak rollout status deployment/kcshim --timeout=5m
    kubectl --context {{ cm_context }} -n payments rollout status deployment/web --timeout=5m
    kubectl --context {{ cm_context }} -n default rollout status deployment/other --timeout=5m

cluster-mode-image:
    #!/usr/bin/env bash
    set -euo pipefail
    version="$(just app-version)"
    named="${KIND_EXPERIMENTAL_DOCKER_NETWORK:-}"
    if [ -z "$named" ]; then
        docker build --build-arg SPINOZA_VERSION="$version" -t spinoza:cluster-mode .
    else
        # buildkit takes a network from its builder, not from the build, and the
        # default bridge is the one this host's firewall will not forward.
        builder="spinoza-$named"
        if ! docker buildx inspect "$builder" > /dev/null 2>&1; then
            docker buildx create --name "$builder" --driver docker-container \
                --driver-opt "network=$named" > /dev/null
        fi
        docker buildx build --builder "$builder" --load \
            --build-arg SPINOZA_VERSION="$version" -t spinoza:cluster-mode .
    fi
    kind load docker-image spinoza:cluster-mode --name {{ cm_cluster }}

cluster-mode-down:
    kind delete cluster --name {{ cm_cluster }}

# every cluster-mode path, against a real cluster and a real identity provider
test-cluster-mode name='': cluster-mode-up cluster-mode-image
    #!/usr/bin/env bash
    set -euo pipefail
    args=(-tags clustermode -count=1 -timeout 45m -v)
    if [ -n '{{ name }}' ]; then
        args+=(-run '{{ name }}')
    fi
    SPINOZA_CM_CONTEXT={{ cm_context }} SPINOZA_CM_CA="$PWD/.tmp/cm/tls.crt" \
        go test "${args[@]}" ./{{ cm_dir }}/...

lint-chart:
    helm lint deploy/helm/spinoza --set publicURL=https://spinoza.example.com --set auth.mode=none --set auth.allowAnonymous=true
    ! helm template spinoza deploy/helm/spinoza --namespace spinoza \
        --set publicURL=https://spinoza.example.com > /dev/null 2>&1
    ! helm template spinoza deploy/helm/spinoza --namespace spinoza \
        --set publicURL=https://spinoza.example.com \
        --set auth.mode=none > /dev/null 2>&1
    helm template spinoza deploy/helm/spinoza --namespace spinoza \
        --set publicURL=https://spinoza.example.com \
        --set auth.mode=oidc \
        --set auth.oidc.issuerURL=https://keycloak.example.com/realms/main \
        --set auth.oidc.clientID=spinoza > /dev/null
    ! helm template spinoza deploy/helm/spinoza --namespace spinoza \
        --set publicURL=https://spinoza.example.com \
        --set replicaCount=2 > /dev/null 2>&1
    helm template spinoza deploy/helm/spinoza --namespace spinoza \
        --set publicURL=https://spinoza.example.com \
        --set auth.mode=proxy \
        --set auth.proxy.sharedSecret=a-proxy-authentication-secret-that-is-long-enough \
        --set rbac.read=workloads \
        --set persistence.enabled=true \
        --set ingress.enabled=true \
        --set ingress.hosts[0].host=spinoza.example.com \
        --set ingress.hosts[0].paths[0].path=/ > /dev/null

test-release-publication:
    #!/usr/bin/env bash
    set -euo pipefail
    released=$(yq -r '.["."]' .release-please-manifest.json)
    chart=$(yq -r '.version' deploy/helm/spinoza/Chart.yaml)
    app=$(yq -r '.appVersion' deploy/helm/spinoza/Chart.yaml)
    if [ "$chart" != "$released" ] || [ "$app" != "$released" ]; then
        echo "chart=$chart appVersion=$app release=$released"
        exit 1
    fi
    yq -e '.packages."."."extra-files"[] | select(.type == "yaml" and .path == "deploy/helm/spinoza/Chart.yaml" and .jsonpath == "$.version")' release-please-config.json > /dev/null
    yq -e '.packages."."."extra-files"[] | select(.type == "yaml" and .path == "deploy/helm/spinoza/Chart.yaml" and .jsonpath == "$.appVersion")' release-please-config.json > /dev/null
    yq -e '.jobs.version.outputs.version != null' .github/workflows/release-artifacts.yaml > /dev/null
    yq -e '.jobs.image.permissions.packages == "write"' .github/workflows/release-artifacts.yaml > /dev/null
    yq -e '.jobs.image.steps[] | select(.id == "push") | ((.with.push == true) and (.with.platforms == "linux/amd64,linux/arm64"))' .github/workflows/release-artifacts.yaml > /dev/null
    yq -e '.jobs.image.steps[] | select(.id == "push") | .with.tags | test("needs.version.outputs.version")' .github/workflows/release-artifacts.yaml > /dev/null
    yq -e '.jobs.chart.needs | contains(["image"])' .github/workflows/release-artifacts.yaml > /dev/null
    yq -e '.jobs.chart.permissions.packages == "write"' .github/workflows/release-artifacts.yaml > /dev/null
    yq -e '.jobs.chart.steps[] | select(.run != null) | .run | select(test("helm package"))' .github/workflows/release-artifacts.yaml > /dev/null
    yq -e '.jobs.chart.steps[] | select(.run != null) | .run | select(test("helm push"))' .github/workflows/release-artifacts.yaml > /dev/null
    yq -e '.jobs.publish.needs | contains(["chart"])' .github/workflows/release-artifacts.yaml > /dev/null
    echo "test-release-publication: release-please, image and chart publication are linked"

image tag='spinoza:dev':
    docker build --build-arg SPINOZA_VERSION="$(just app-version)" -t {{ tag }} .

lint: lint-be lint-fe lint-e2e lint-chart

fmt-check:
    golangci-lint fmt --diff

mod-check:
    go mod tidy -diff
    go mod verify

dead:
    #!/usr/bin/env bash
    set -euo pipefail
    out=$(deadcode -test {{ go_pkgs }})
    if [ -n "$out" ]; then
        echo "$out"
        exit 1
    fi

cross:
    #!/usr/bin/env bash
    set -euo pipefail
    for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
        echo "cross: $target"
        GOOS="${target%/*}" GOARCH="${target#*/}" CGO_ENABLED=0 go build -o /dev/null .
    done
    for target in windows/amd64 windows/arm64; do
        echo "cross: $target desktop"
        GOOS="${target%/*}" GOARCH="${target#*/}" CGO_ENABLED=0 go build -tags desktop -o /dev/null .
    done

audit-be:
    govulncheck {{ go_pkgs }}

audit-fe:
    cd frontend && npm run knip
    cd frontend && npm run knip:production
    cd frontend && npm run depcheck
    cd frontend && npm run madge
    cd frontend && npm run typecov

audit: audit-be audit-fe

secrets:
    gitleaks dir . --no-banner --redact
    gitleaks git . --no-banner --redact

# the run token cookie is served over loopback http only, so Secure would keep the
# desktop webview from ever sending it back; see internal/server/guard.go
sast_excluded := 'go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure'

sast:
    semgrep scan --config .semgrep --config p/golang --config p/typescript --config p/react --exclude-rule {{ sast_excluded }} --error --quiet

# e2e/fixtures holds workloads written to be insecure so the checks have
# something to find; scanning them fails the build on findings we put there.
vulns: vulnerability-exceptions
    trivy fs --exit-code 1 --scanners secret,misconfig \
        --skip-dirs e2e/fixtures --skip-dirs test/clustermode --skip-dirs .tmp \
        --skip-files test/integration/metrics-server.yaml \
        --helm-set publicURL=https://spinoza.example.com .
    osv-scanner scan source --recursive .

vulnerability-exceptions: stub-assets
    #!/usr/bin/env bash
    set -euo pipefail
    for tags in '' desktop integration clustermode; do
        args=()
        if [ -n "$tags" ]; then
            args=(-tags "$tags")
        fi
        deps=$(go list -deps "${args[@]}" ./...)
        if grep -q '^golang.org/x/crypto/openpgp\($\|/\)' <<< "$deps"; then
            echo "vulnerability-exceptions: the $tags build imports deprecated openpgp code" >&2
            exit 1
        fi
    done
    echo "vulnerability-exceptions: no build imports deprecated openpgp code"

workflows: scoped-tools workflow-triggers
    yamllint .forgejo .github
    actionlint -config-file .forgejo/actionlint.yaml .forgejo/workflows/*.yaml
    actionlint .github/workflows/*.yaml
    zizmor --no-online-audits --config .forgejo/zizmor.yml .forgejo/workflows/*.yaml
    zizmor --no-online-audits .github/workflows/*.yaml
    test/release-tag.sh

scoped-tools:
    #!/usr/bin/env bash
    set -euo pipefail
    unscoped=$(yq -r '.jobs[].steps[] | select(.uses // "" | test("jdx/mise-action")) | .with.install_args // "UNSCOPED"' \
        .github/workflows/*.yaml .forgejo/workflows/*.yaml 2>/dev/null | grep -c UNSCOPED || true)
    if [ "$unscoped" != "0" ]; then
        echo "scoped-tools: $unscoped mise-action steps install every tool in mise.toml."
        echo "Name the tools the job runs with install_args, so an outage in a tool it"
        echo "never uses cannot fail it."
        exit 1
    fi

workflow-triggers:
    #!/usr/bin/env bash
    set -euo pipefail
    release_files=(
        .release-please-manifest.json
        CHANGELOG.md
        deploy/helm/spinoza/Chart.yaml
        wails.json
    )
    validation=(
        codeql.yaml
        commits.yaml
        e2e.yaml
        frontend.yaml
        go.yaml
        integration.yaml
        repo.yaml
        windows.yaml
    )
    for name in "${validation[@]}"; do
        workflow=".github/workflows/$name"
        if [ "$(yq -r '.concurrency.cancel-in-progress' "$workflow")" != "true" ]; then
            echo "$workflow does not cancel a superseded validation run" >&2
            exit 1
        fi
        ignored=$(yq -r '.on.pull_request.paths-ignore[]' "$workflow")
        for path in "${release_files[@]}"; do
            if ! grep -Fxq "$path" <<< "$ignored"; then
                echo "$workflow reruns for release-only change $path" >&2
                exit 1
            fi
        done
    done
    if [ "$(yq -r '.concurrency.cancel-in-progress' .github/workflows/badges.yaml)" != "true" ]; then
        echo ".github/workflows/badges.yaml can publish an obsolete measurement" >&2
        exit 1
    fi
    durable=(go-fuzz.yaml go-mutation.yaml)
    for name in "${durable[@]}"; do
        workflow=".github/workflows/$name"
        if [ "$(yq -r '.on | (has("push") and has("pull_request"))' "$workflow")" != "true" ]; then
            echo "$workflow does not run on every push and pull request" >&2
            exit 1
        fi
        if [ "$(yq -r '.on | has("schedule")' "$workflow")" != "false" ]; then
            echo "$workflow must not depend on a schedule" >&2
            exit 1
        fi
        if [ "$(yq -r '.concurrency.cancel-in-progress // false' "$workflow")" != "false" ]; then
            echo "$workflow cancels an in-progress test campaign" >&2
            exit 1
        fi
    done

hygiene:
    typos
    just editorconfig
    shellcheck install.sh test/install/container.sh test/install/uninstall.sh \
        test/install/editorconfig-name.sh packaging/render.sh
    just --unstable --fmt --check

editorconfig:
    #!/usr/bin/env bash
    set -euo pipefail
    for name in ec editorconfig-checker; do
        if command -v "$name" > /dev/null 2>&1; then
            exec "$name"
        fi
    done
    echo "no editorconfig checker on PATH; looked for ec and editorconfig-checker" >&2
    exit 1

links:
    lychee --config lychee.toml .

sbom: vulnerability-exceptions
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p dist
    syft scan dir:. --source-name spinoza --exclude './frontend/node_modules/**' --output cyclonedx-json=dist/sbom.cdx.json
    grype sbom:dist/sbom.cdx.json --fail-on medium

commits:
    #!/usr/bin/env bash
    set -euo pipefail
    from=$(node -p "try { const e = require(process.env.GITHUB_EVENT_PATH); (e.pull_request ? e.pull_request.base.sha : e.before) || '' } catch (e) { '' }")
    if [ -z "$from" ] || ! git cat-file -e "$from^{commit}" 2>/dev/null; then
        from=HEAD~1
    fi
    npx --yes --package @commitlint/cli@21.2.2 --package @commitlint/config-conventional@21.2.2 commitlint --from "$from" --to HEAD

fmt:
    golangci-lint fmt
    cd frontend && npm run format

ci-go-build: stub-assets cross
    go build ./...
    go build -tags desktop ./...

ci-go-test: cover-gate

ci-go-lint: lint-be fmt-check mod-check

ci-go-audit: stub-assets audit-be dead

ci-fe-lint: deps lint-fe

ci-fe-test: deps test-fe

ci-fe-audit: deps audit-fe

ci-fe-build: deps
    cd frontend && npm run build
    just bundle-budget

bundle-budget:
    #!/usr/bin/env bash
    set -euo pipefail
    budget=9000000
    actual=$(find web/dist/assets -type f -exec cat {} + | wc -c | tr -d ' ')
    echo "bundle: $actual bytes (budget $budget)"
    if [ "$actual" -gt "$budget" ]; then
        exit 1
    fi

release-notes:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p dist
    git-cliff --latest --strip all --output dist/RELEASE_NOTES.md

release-dist: deps
    #!/usr/bin/env bash
    set -euo pipefail
    shopt -s nullglob
    version=$(just app-version)
    export SPINOZA_VERSION="$version"
    mkdir -p dist/release
    cd frontend && npm run build && cd ..
    tar_sorts=no
    if tar --version 2>/dev/null | grep -q GNU; then
        tar_sorts=yes
    fi
    for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
        goos="${target%/*}"
        goarch="${target#*/}"
        out="dist/build/${goos}_${goarch}"
        mkdir -p "$out"
        binary="spinoza"
        if [ "$goos" = "windows" ]; then
            binary="spinoza.exe"
        fi
        GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -trimpath -ldflags "{{ ldflags }} -X {{ version_pkg }}=$version" -o "$out/$binary" .
        cp LICENSE "$out/LICENSE"
        if [ "$goos" = "windows" ]; then
            archive="$PWD/dist/release/spinoza_${version}_${goos}_${goarch}.zip"
            rm -f "$archive"
            (cd "$out" && zip -q -X "$archive" "$binary" LICENSE)
            continue
        fi
        archive="dist/release/spinoza_${version}_${goos}_${goarch}.tar.gz"
        if [ "$tar_sorts" = yes ]; then
            tar --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - -C "$out" "$binary" LICENSE | gzip -n > "$archive"
        else
            tar -cf - -C "$out" "$binary" LICENSE | gzip -n > "$archive"
        fi
    done
    if [ -n "${SPINOZA_SKIP_EXTRAS:-}" ]; then
        echo "release-dist: SPINOZA_SKIP_EXTRAS is set, so no windows desktop app, deb or rpm"
    else
        just package-desktop-windows amd64
        just package-desktop-windows arm64
        just package-linux amd64
        just package-linux arm64
    fi
    syft scan dir:dist/build --source-name spinoza --source-version "$version" --output "cyclonedx-json=dist/release/spinoza_${version}_sbom.cdx.json"
    cd dist/release && sha256sum -- *.tar.gz *.zip *.deb *.rpm > checksums.txt
    cd ../..
    just verify-checksums dist/release
    just verify-archives dist/release

package-linux arch='amd64':
    #!/usr/bin/env bash
    set -euo pipefail
    version=$(just app-version)
    numeric="${version#v}"
    numeric="${numeric%%-*}"
    if ! printf '%s' "$numeric" | grep -Eq '^[0-9]+(\.[0-9]+){2}$'; then
        echo "package-linux: $version does not carry a release number, so no deb or rpm was built"
        exit 0
    fi
    binary="dist/build/linux_{{ arch }}/spinoza"
    if [ ! -f "$binary" ]; then
        echo "package-linux: $binary is missing, run just release-dist first"
        exit 1
    fi
    export SPINOZA_ARCH='{{ arch }}'
    export SPINOZA_NUMERIC="$numeric"
    mkdir -p dist/release dist/nfpm
    cp "$binary" dist/nfpm/spinoza
    for packager in deb rpm; do
        nfpm package --config packaging/nfpm.yaml --packager "$packager" --target "dist/release/spinoza_${version}_linux_{{ arch }}.${packager}"
    done
    rm -rf dist/nfpm

verify-archives dir:
    #!/usr/bin/env bash
    set -euo pipefail
    shopt -s nullglob
    dir='{{ dir }}'
    checked=0
    for file in "$dir"/*_windows_*.zip; do
        want=spinoza.exe
        case "$file" in
            *_app.zip)
                want=Spinoza.exe
                ;;
        esac
        listing=$(unzip -l "$file")
        if ! printf '%s\n' "$listing" | grep -q " ${want}$"; then
            echo "verify-archives: $file carries no $want"
            exit 1
        fi
        checked=$((checked + 1))
    done
    for file in "$dir"/*_linux_*.tar.gz "$dir"/*_darwin_*.tar.gz; do
        want=spinoza
        case "$file" in
            *_app.tar.gz)
                want=Spinoza
                ;;
        esac
        listing=$(tar -tzf "$file")
        if ! printf '%s\n' "$listing" | grep -qx "$want"; then
            echo "verify-archives: $file carries no $want"
            exit 1
        fi
        checked=$((checked + 1))
    done
    if [ "$checked" -eq 0 ]; then
        echo "verify-archives: $dir holds no archives to check"
        exit 1
    fi
    echo "verify-archives: all $checked archives carry the binary their installer looks for"

test-verify-archives:
    #!/usr/bin/env bash
    set -euo pipefail
    out=$(mktemp -d)
    trap 'rm -rf "$out"' EXIT
    cli="$out/cli"
    app="$out/app"
    release="$out/release"
    mkdir -p "$cli" "$app" "$release"
    head -c 1048576 /dev/urandom > "$cli/spinoza"
    cp "$cli/spinoza" "$cli/spinoza.exe"
    cp "$cli/spinoza" "$app/Spinoza"
    cp "$cli/spinoza" "$app/Spinoza.exe"
    cp LICENSE "$cli/LICENSE"
    cp LICENSE "$app/LICENSE"
    tar -cf - -C "$cli" spinoza LICENSE | gzip -n > "$release/spinoza_v9.9.9_linux_amd64.tar.gz"
    tar -cf - -C "$cli" spinoza LICENSE | gzip -n > "$release/spinoza_v9.9.9_darwin_arm64.tar.gz"
    tar -cf - -C "$app" Spinoza LICENSE | gzip -n > "$release/spinoza_v9.9.9_linux_amd64_app.tar.gz"
    if command -v zip > /dev/null 2>&1; then
        (cd "$cli" && zip -q -X "$release/spinoza_v9.9.9_windows_amd64.zip" spinoza.exe LICENSE)
        (cd "$app" && zip -q -X "$release/spinoza_v9.9.9_windows_amd64_app.zip" Spinoza.exe LICENSE)
    else
        (cd "$cli" && python3 -m zipfile -c "$release/spinoza_v9.9.9_windows_amd64.zip" spinoza.exe LICENSE)
        (cd "$app" && python3 -m zipfile -c "$release/spinoza_v9.9.9_windows_amd64_app.zip" Spinoza.exe LICENSE)
    fi
    just verify-archives "$release"
    tar -cf - -C "$cli" LICENSE | gzip -n > "$release/spinoza_v9.9.9_linux_amd64.tar.gz"
    if just verify-archives "$release" > /dev/null 2>&1; then
        echo "test-verify-archives: a tarball that lost its binary still passed"
        exit 1
    fi
    echo "test-verify-archives: the check reads a whole listing and still fails an archive that lost its binary"

verify-checksums dir:
    #!/usr/bin/env bash
    set -euo pipefail
    dir='{{ dir }}'
    list="$dir/checksums.txt"
    hash_of() {
        if command -v sha256sum > /dev/null; then
            sha256sum "$1" | awk '{ print $1 }'
            return
        fi
        shasum -a 256 "$1" | awk '{ print $1 }'
    }
    if [ ! -f "$list" ]; then
        echo "verify-checksums: $list is missing"
        exit 1
    fi
    if ! awk '$2 ~ "/" { bad = 1 } END { exit bad }' "$list"; then
        echo "verify-checksums: a name in checksums.txt carries a path, install.sh matches bare filenames"
        exit 1
    fi
    checked=0
    for file in "$dir"/*.tar.gz "$dir"/*.tgz "$dir"/*.zip "$dir"/*.deb "$dir"/*.rpm; do
        if [ ! -f "$file" ]; then
            continue
        fi
        name=$(basename "$file")
        expected=$(awk -v name="$name" '$2 == name { print $1 }' "$list")
        if [ -z "$expected" ]; then
            echo "verify-checksums: $name is not listed in checksums.txt"
            exit 1
        fi
        actual=$(hash_of "$file")
        if [ "$expected" != "$actual" ]; then
            echo "verify-checksums: $name does not match the checksum listed for it"
            exit 1
        fi
        checked=$((checked + 1))
    done
    if [ "$checked" -eq 0 ]; then
        echo "verify-checksums: $dir holds no artifacts to check"
        exit 1
    fi
    echo "verify-checksums: every artifact in $dir matches checksums.txt"

test-verify-checksums:
    #!/usr/bin/env bash
    set -euo pipefail
    out=$(mktemp -d)
    trap 'rm -rf "$out"' EXIT
    artifact="$out/spinoza-9.9.9.tgz"
    printf 'chart' > "$artifact"
    if command -v sha256sum > /dev/null; then
        (cd "$out" && sha256sum "$(basename "$artifact")" > checksums.txt)
    else
        (cd "$out" && shasum -a 256 "$(basename "$artifact")" > checksums.txt)
    fi
    just verify-checksums "$out"
    printf 'changed' >> "$artifact"
    if just verify-checksums "$out" > /dev/null 2>&1; then
        echo "test-verify-checksums: a changed Helm chart still passed"
        exit 1
    fi
    echo "test-verify-checksums: Helm charts are checked and checksum mismatches fail"

wait-for-release:
    #!/usr/bin/env bash
    set -euo pipefail
    version="${SPINOZA_VERSION:-}"
    if [ -z "$version" ]; then
        exit 0
    fi
    url="https://github.com/sophotechlabs/spinoza/releases/download/$version/checksums.txt"
    for _ in $(seq 1 60); do
        if curl -fsSL -o /dev/null "$url"; then
            echo "wait-for-release: $version is downloadable"
            exit 0
        fi
        sleep 5
    done
    echo "wait-for-release: $url did not become downloadable within five minutes"
    exit 1

test-install:
    #!/usr/bin/env bash
    set -euo pipefail
    image="${IMAGE:?IMAGE is required}"
    expect="${EXPECT:-install}"
    args=(--rm)
    args+=(-e "SETUP=${SETUP:-}")
    args+=(-e "SPINOZA_VERSION=${SPINOZA_VERSION:-}")
    args+=(-v "$PWD/install.sh:/install.sh:ro")
    args+=(-v "$PWD/test/install/container.sh:/container.sh:ro")
    args+=("$image" sh /container.sh)
    if [ "$expect" = install ]; then
        docker run "${args[@]}"
        echo "test-install: $image installed and ran spinoza"
        exit 0
    fi
    set +e
    output=$(docker run "${args[@]}" 2>&1)
    code=$?
    set -e
    if [ "$code" -eq 0 ]; then
        echo "test-install: $image installed spinoza, but this image has no downloader and should have refused"
        exit 1
    fi
    if ! printf '%s' "$output" | grep -q 'neither curl nor wget is on PATH'; then
        echo "test-install: $image failed for a reason the test did not expect"
        printf '%s\n' "$output"
        exit 1
    fi
    echo "test-install: $image refused to install without curl or wget"

test-uninstall:
    sh test/install/uninstall.sh install.sh
    sh test/install/editorconfig-name.sh

test-install-host:
    #!/usr/bin/env bash
    set -euo pipefail
    sh install.sh
    export PATH="$HOME/.local/bin:$PATH"
    spinoza --version
    if [ "$(uname -s)" != Darwin ]; then
        exit 0
    fi
    app=""
    for candidate in /Applications "$HOME/Applications"; do
        if [ -d "$candidate/Spinoza.app" ]; then
            app="$candidate/Spinoza.app"
            break
        fi
    done
    if [ -z "$app" ]; then
        echo "test-install-host: the desktop app was not installed"
        exit 1
    fi
    if ! lipo -archs "$app/Contents/MacOS/Spinoza" | grep -q x86_64; then
        echo "test-install-host: $app is not universal, intel macs cannot run it"
        exit 1
    fi
    echo "test-install-host: installed $app as a universal binary"
    SPINOZA_UNINSTALL=1 sh install.sh
    if [ -e "$HOME/.local/bin/spinoza" ]; then
        echo "test-install-host: the binary is still there after the uninstall"
        exit 1
    fi
    if [ -d "$app" ]; then
        echo "test-install-host: $app is still there after the uninstall"
        exit 1
    fi
    echo "test-install-host: uninstalled the binary and the app"

smoke:
    #!/usr/bin/env bash
    set -euo pipefail
    binary="dist/build/$(go env GOOS)_$(go env GOARCH)/spinoza"
    if [ ! -x "$binary" ]; then
        echo "smoke: $binary is missing, run just release-dist first"
        exit 1
    fi
    "$binary" --version
    work=$(mktemp -d)
    printf 'apiVersion: v1\nkind: Config\n' > "$work/kubeconfig"
    KUBECONFIG="$work/kubeconfig" "$binary" --addr 127.0.0.1:34988 --token-file "$work/token" &
    pid=$!
    trap 'kill "$pid" 2>/dev/null || true; rm -rf "$work"' EXIT
    for _ in $(seq 1 50); do
        if [ -s "$work/token" ]; then
            break
        fi
        sleep 0.2
    done
    if [ ! -s "$work/token" ]; then
        echo "smoke: the token file was never written"
        exit 1
    fi
    token=$(cat "$work/token")
    for _ in $(seq 1 50); do
        code=$(curl -s -o /dev/null -w '%{http_code}' -H "X-Spinoza-Token: $token" http://127.0.0.1:34988/healthz || true)
        if [ "$code" = 200 ]; then
            break
        fi
        sleep 0.2
    done
    if [ "$code" != 200 ]; then
        echo "smoke: healthz answered $code"
        exit 1
    fi
    page=$(curl -s -H "X-Spinoza-Token: $token" http://127.0.0.1:34988/ || true)
    if printf '%s' "$page" | grep -q 'built without its frontend'; then
        echo "smoke: the binary embeds the placeholder index.html"
        exit 1
    fi
    echo "smoke: $("$binary" --version) answered healthz and served the frontend"

smoke-windows binary='spinoza.exe':
    pwsh -NoProfile -ExecutionPolicy Bypass -File test/smoke.ps1 -Binary "{{ binary }}"

smoke-desktop-windows:
    #!/usr/bin/env bash
    set -euo pipefail
    version=$(just app-version)
    go build -trimpath -tags desktop -ldflags "{{ ldflags }} -X {{ version_pkg }}=$version" -o Spinoza.exe .
    reported=$(./Spinoza.exe --version)
    rm -f Spinoza.exe
    if [ "$reported" != "$version" ]; then
        echo "smoke-desktop-windows: the desktop build reports $reported, not $version"
        exit 1
    fi
    echo "smoke-desktop-windows: the desktop build starts and reports $reported"

test-install-windows shell='pwsh':
    {{ shell }} -NoProfile -ExecutionPolicy Bypass -File test/install/windows.ps1

test-ps shell='pwsh':
    {{ shell }} -NoProfile -ExecutionPolicy Bypass -File test/pester.ps1

lint-ps:
    pwsh -NoProfile -ExecutionPolicy Bypass -File test/lint-powershell.ps1

ci-windows-smoke: build
    just smoke-windows

package-desktop: build-desktop
    #!/usr/bin/env bash
    set -euo pipefail
    version=$(just app-version)
    bundle=build/bin/spinoza.app
    if [ ! -d "$bundle" ]; then
        echo "package-desktop: $bundle was not built"
        exit 1
    fi
    staged=dist/app
    rm -rf "$staged"
    mkdir -p "$staged" dist/release
    cp -R "$bundle" "$staged/Spinoza.app"
    if ! lipo -archs "$staged/Spinoza.app/Contents/MacOS/Spinoza" | grep -q x86_64; then
        echo "package-desktop: the bundle is not universal, intel macs cannot run it"
        exit 1
    fi
    codesign --force --deep --sign - "$staged/Spinoza.app"
    codesign --verify --deep "$staged/Spinoza.app"
    ditto -c -k --keepParent "$staged/Spinoza.app" "dist/release/spinoza_${version}_darwin_app.zip"

packaging:
    SPINOZA_VERSION="$(just app-version)" sh packaging/render.sh

package-manifests:
    #!/usr/bin/env bash
    set -euo pipefail
    version=$(just app-version)
    just packaging
    mkdir -p dist/release
    archive="$PWD/dist/release/spinoza_${version}_packaging.tar.gz"
    rm -f "$archive"
    tar -cf - -C dist/packaging . | gzip -n > "$archive"
    echo "package-manifests: wrote $archive"

test-packaging:
    #!/usr/bin/env bash
    set -euo pipefail
    out=$(mktemp -d)
    trap 'rm -rf "$out"' EXIT
    SPINOZA_VERSION=v9.9.9 CHECKSUMS=test/packaging/checksums.txt OUT_DIR="$out" sh packaging/render.sh
    yq -e '.version == "9.9.9"' "$out/scoop/spinoza.json" > /dev/null
    yq -e '.architecture["64bit"].hash == "0000000000000000000000000000000000000000000000000000000000000001"' "$out/scoop/spinoza.json" > /dev/null
    yq -e '.spec.version == "v9.9.9"' "$out/krew/spinoza.yaml" > /dev/null
    yq -e '.spec.platforms | length == 6' "$out/krew/spinoza.yaml" > /dev/null
    for file in "$out"/winget/*.yaml; do
        yq -e '.PackageVersion == "9.9.9"' "$file" > /dev/null
    done
    yq -p xml -o yaml -e '.package.metadata.version == "9.9.9"' "$out/chocolatey/spinoza.nuspec" > /dev/null
    grep -q 'version "9.9.9"' "$out/homebrew/spinoza.rb"
    grep -q '0000000000000000000000000000000000000000000000000000000000000004' "$out/homebrew/spinoza.rb"
    yq -e '.bin == "spinoza.exe"' "$out/scoop/spinoza.json" > /dev/null
    yq -e '.autoupdate.hash.url | test("checksums.txt$")' "$out/scoop/spinoza.json" > /dev/null
    yq -e '.apiVersion == "krew.googlecontainertools.github.com/v1alpha2"' "$out/krew/spinoza.yaml" > /dev/null
    yq -e '[.spec.platforms[] | select(.uri and .sha256 and .bin and .selector.matchLabels.os)] | length == 6' "$out/krew/spinoza.yaml" > /dev/null
    yq -e '.PackageIdentifier == "Sophotech.Spinoza"' "$out/winget/Sophotech.Spinoza.installer.yaml" > /dev/null
    yq -e '.NestedInstallerFiles[0].RelativeFilePath == "spinoza.exe"' "$out/winget/Sophotech.Spinoza.installer.yaml" > /dev/null
    yq -e '[.Installers[] | select(.InstallerUrl and .InstallerSha256)] | length == 2' "$out/winget/Sophotech.Spinoza.installer.yaml" > /dev/null
    for file in "$out"/winget/*.yaml; do
        yq -e '.ManifestType and .ManifestVersion' "$file" > /dev/null
    done
    yq -p xml -o yaml -e '.package.metadata.id == "spinoza"' "$out/chocolatey/spinoza.nuspec" > /dev/null
    grep -q 'checksumType64 *= *.sha256.' "$out/chocolatey/tools/chocolateyinstall.ps1"
    echo "test-packaging: every manifest rendered with the fields and hashes its ecosystem needs"

publish-asset pattern:
    #!/usr/bin/env bash
    set -euo pipefail
    tag="${TAG:?TAG is required}"
    asset=$(ls dist/release/{{ pattern }})
    name=$(basename "$asset")
    gh release upload "$tag" "$asset" --clobber
    gh release download "$tag" --pattern checksums.txt --output dist/release/checksums.remote.txt --clobber
    grep -v " ${name}$" dist/release/checksums.remote.txt > dist/release/checksums.merged.txt || true
    if command -v sha256sum > /dev/null; then
        (cd dist/release && sha256sum "$name" >> checksums.merged.txt)
    else
        (cd dist/release && shasum -a 256 "$name" >> checksums.merged.txt)
    fi
    sort -k2 dist/release/checksums.merged.txt > dist/release/checksums.txt
    just verify-checksums dist/release
    gh release upload "$tag" dist/release/checksums.txt --clobber

publish-desktop: (publish-asset '*_darwin_app.zip')

publish-desktop-linux: (publish-asset '*_linux_amd64_app.tar.gz')

check: lint test

ci: ci-go-build ci-go-test ci-go-lint ci-go-audit ci-fe-lint ci-fe-test ci-fe-audit ci-fe-build secrets sast vulns workflows hygiene lint-chart links sbom

rescan: stub-assets audit-be vulns sbom links

clean:
    rm -f spinoza spinoza.exe coverage.out
    rm -rf dist frontend/dist frontend/coverage web/dist/assets web/dist/index.html
