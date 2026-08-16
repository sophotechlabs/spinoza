export PATH := env_var('HOME') + '/go/bin:' + env_var('PATH')

go_pkgs := './internal/... .'
addr := env_var_or_default('SPINOZA_ADDR', '127.0.0.1:34115')
ldflags := '-s -w'
version_pkg := 'github.com/sophotechlabs/spinoza/internal/version.value'

default:
    @just --list

[private]
app-version:
    #!/usr/bin/env bash
    set -euo pipefail
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
    cd frontend && npm run build
    go build -trimpath -ldflags "{{ ldflags }} -X {{ version_pkg }}=$(just app-version)" -o spinoza .

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
    ./spinoza

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
    wails build -tags desktop -skipbindings -trimpath -ldflags "{{ ldflags }} -X {{ version_pkg }}=$version"
    plist=build/bin/spinoza.app/Contents/Info.plist
    if [ -f "$plist" ]; then
        plutil -lint "$plist"
    fi
    just icns

# Wails writes an icns with the retina sizes only; macOS wants the plain ones too
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

test-be: stub-assets
    go test -race -shuffle=on -covermode=atomic -coverprofile=coverage.out {{ go_pkgs }}
    go tool cover -func=coverage.out

test-integration:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! kind get clusters | grep -qx spinoza; then
        kind create cluster --name spinoza
    fi
    kubectl --context kind-spinoza cluster-info
    SPINOZA_TEST_CONTEXT=kind-spinoza go test -tags integration -count=1 -timeout 15m -covermode=atomic -coverprofile=coverage.integration.out -coverpkg=./internal/... ./test/integration/...
    go tool cover -func=coverage.integration.out | tail -1

test-integration-down:
    kind delete cluster --name spinoza

test-fe:
    cd frontend && npm run test:coverage

test: test-be test-fe

cover-gate: test-be
    go-test-coverage --config .testcoverage.yml

lint-be: stub-assets
    golangci-lint run {{ go_pkgs }}
    golangci-lint run --build-tags desktop {{ go_pkgs }}
    go vet {{ go_pkgs }}
    go vet -tags desktop {{ go_pkgs }}

lint-fe:
    cd frontend && npm run lint
    cd frontend && npm run typecheck
    cd frontend && npm run format:check
    cd frontend && npm run contrast

lint: lint-be lint-fe

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

audit-be:
    govulncheck {{ go_pkgs }}

audit-fe:
    cd frontend && npm run knip
    cd frontend && npm run depcheck
    cd frontend && npm run madge
    cd frontend && npm run typecov

audit: audit-be audit-fe

secrets:
    gitleaks dir . --no-banner --redact
    gitleaks git . --no-banner --redact

sast:
    semgrep scan --config .semgrep --config p/golang --config p/typescript --config p/react --error --quiet

vulns:
    trivy fs --exit-code 1 --scanners secret,misconfig .
    osv-scanner scan source --recursive .

workflows:
    yamllint .forgejo .github
    actionlint -config-file .forgejo/actionlint.yaml .forgejo/workflows/*.yaml
    actionlint .github/workflows/*.yaml
    zizmor --no-online-audits --config .forgejo/zizmor.yml .forgejo/workflows/*.yaml
    zizmor --no-online-audits .github/workflows/*.yaml

hygiene:
    typos
    editorconfig-checker
    just --unstable --fmt --check

docs:
    markdownlint-cli2 "**/*.md"
    lychee --config lychee.toml .

sbom:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p dist
    syft scan dir:. --source-name spinoza --output cyclonedx-json=dist/sbom.cdx.json
    grype sbom:dist/sbom.cdx.json --fail-on medium

commits:
    #!/usr/bin/env bash
    set -euo pipefail
    from=$(node -p "try { const e = require(process.env.GITHUB_EVENT_PATH); (e.pull_request ? e.pull_request.base.sha : e.before) || '' } catch (e) { '' }")
    if [ -z "$from" ] || ! git cat-file -e "$from^{commit}" 2>/dev/null; then
        from=HEAD~1
    fi
    npx --yes --package @commitlint/cli --package @commitlint/config-conventional commitlint --from "$from" --to HEAD

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
    version=$(just app-version)
    mkdir -p dist/release
    cd frontend && npm run build && cd ..
    for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
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
        tar -czf "dist/release/spinoza_${version}_${goos}_${goarch}.tar.gz" -C "$out" "$binary" LICENSE
    done
    cd dist/release && sha256sum *.tar.gz > checksums.txt

check: lint test

ci: ci-go-build ci-go-test ci-go-lint ci-go-audit ci-fe-lint ci-fe-test ci-fe-audit ci-fe-build secrets sast vulns workflows hygiene docs sbom

clean:
    rm -f spinoza coverage.out
    rm -rf dist frontend/dist frontend/coverage web/dist/assets web/dist/index.html
