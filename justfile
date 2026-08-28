export PATH := if os() == 'windows' { env_var('PATH') } else { env_var('HOME') + '/go/bin:' + env_var('PATH') }

exe := if os() == 'windows' { '.exe' } else { '' }
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
    for target in windows/amd64 windows/arm64; do
        echo "cross: $target desktop"
        GOOS="${target%/*}" GOARCH="${target#*/}" CGO_ENABLED=0 go build -tags desktop -o /dev/null .
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

# the run token cookie is served over loopback http only, so Secure would keep the
# desktop webview from ever sending it back; see internal/server/guard.go
sast_excluded := 'go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure'

sast:
    semgrep scan --config .semgrep --config p/golang --config p/typescript --config p/react --exclude-rule {{ sast_excluded }} --error --quiet

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
    ec
    shellcheck install.sh test/install/container.sh test/install/uninstall.sh packaging/render.sh
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
        if ! unzip -l "$file" | grep -q " ${want}$"; then
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
        if ! tar -tzf "$file" | grep -qx "$want"; then
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
    for file in "$dir"/*.tar.gz "$dir"/*.zip "$dir"/*.deb "$dir"/*.rpm; do
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

test-uninstall:
    sh test/install/uninstall.sh install.sh

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

ci: ci-go-build ci-go-test ci-go-lint ci-go-audit ci-fe-lint ci-fe-test ci-fe-audit ci-fe-build secrets sast vulns workflows hygiene docs links sbom

rescan: stub-assets audit-be vulns sbom links

clean:
    rm -f spinoza spinoza.exe coverage.out
    rm -rf dist frontend/dist frontend/coverage web/dist/assets web/dist/index.html
