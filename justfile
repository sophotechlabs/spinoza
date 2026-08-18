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
    wails build -platform darwin/universal -tags desktop -skipbindings -trimpath -ldflags "{{ ldflags }} -X {{ version_pkg }}=$version"
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

test-repeat: stub-assets
    go test -race -shuffle=on -count=5 {{ go_pkgs }}

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
    go vet {{ go_pkgs }}
    go vet -tags desktop {{ go_pkgs }}
    go vet -tags integration ./test/...

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
    shellcheck install.sh test/install/container.sh
    just --unstable --fmt --check

docs:
    markdownlint-cli2 "**/*.md"

links:
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
    version=$(just app-version)
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
    syft scan dir:dist/build --source-name spinoza --source-version "$version" --output "cyclonedx-json=dist/release/spinoza_${version}_sbom.cdx.json"
    cd dist/release && sha256sum -- *.tar.gz *.zip > checksums.txt
    cd ../..
    just verify-checksums dist/release

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
    for file in "$dir"/*.tar.gz "$dir"/*.zip; do
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

publish-desktop:
    #!/usr/bin/env bash
    set -euo pipefail
    tag="${TAG:?TAG is required}"
    asset=$(ls dist/release/*_darwin_app.zip)
    gh release upload "$tag" "$asset" --clobber
    gh release download "$tag" --pattern checksums.txt --output dist/release/checksums.remote.txt --clobber
    grep -v "_darwin_app.zip$" dist/release/checksums.remote.txt > dist/release/checksums.merged.txt || true
    (cd dist/release && shasum -a 256 "$(basename "$asset")" >> checksums.merged.txt)
    sort -k2 dist/release/checksums.merged.txt > dist/release/checksums.txt
    just verify-checksums dist/release
    gh release upload "$tag" dist/release/checksums.txt --clobber

check: lint test

ci: ci-go-build ci-go-test ci-go-lint ci-go-audit ci-fe-lint ci-fe-test ci-fe-audit ci-fe-build secrets sast vulns workflows hygiene docs links sbom

rescan: stub-assets audit-be vulns sbom links

clean:
    rm -f spinoza coverage.out
    rm -rf dist frontend/dist frontend/coverage web/dist/assets web/dist/index.html
