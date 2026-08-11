export PATH := env_var('HOME') + '/go/bin:' + env_var('PATH')

go_pkgs := './internal/... .'
ldflags := '-s -w'

default:
    @just --list

deps:
    cd frontend && npm ci

tidy:
    go mod tidy

build:
    cd frontend && npm run build
    go build -trimpath -ldflags '{{ ldflags }}' -o spinoza .

run: build
    ./spinoza

build-desktop:
    wails build -tags desktop -skipbindings -trimpath -ldflags '{{ ldflags }}'

rund: build-desktop
    open build/bin/spinoza.app

dev-desktop:
    wails dev -tags desktop -skipbindings

dev-api:
    go run . --addr 127.0.0.1:34115

dev-web:
    cd frontend && npm run dev

test-be:
    go test -race -shuffle=on -covermode=atomic -coverprofile=coverage.out {{ go_pkgs }}
    go tool cover -func=coverage.out

test-integration:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! kind get clusters | grep -qx spinoza; then
        kind create cluster --name spinoza
    fi
    kubectl --context kind-spinoza cluster-info
    SPINOZA_TEST_CONTEXT=kind-spinoza go test -tags integration -count=1 -timeout 15m ./test/integration/...

test-integration-down:
    kind delete cluster --name spinoza

test-fe:
    cd frontend && npm run test:coverage

test: test-be test-fe

cover-gate: test-be
    go-test-coverage --config .testcoverage.yml

lint-be:
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
    yamllint .forgejo
    actionlint -config-file .forgejo/actionlint.yaml .forgejo/workflows/*.yaml
    zizmor --no-online-audits --config .forgejo/zizmor.yml .forgejo/workflows/*.yaml

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
    from=$(node -p "try { require(process.env.GITHUB_EVENT_PATH).before || '' } catch (e) { '' }")
    if [ -z "$from" ] || ! git cat-file -e "$from^{commit}" 2>/dev/null; then
        from=HEAD~1
    fi
    npx --yes --package @commitlint/cli --package @commitlint/config-conventional commitlint --from "$from" --to HEAD

fmt:
    golangci-lint fmt
    cd frontend && npm run format

ci-go-build: cross
    go build ./...
    go build -tags desktop ./...

ci-go-test: cover-gate

ci-go-lint: lint-be fmt-check mod-check

ci-go-audit: audit-be dead

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
    version="${GITHUB_REF_NAME:-dev}"
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
        GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -trimpath -ldflags "{{ ldflags }} -X github.com/sophotechlabs/spinoza/internal/version.value=$version" -o "$out/$binary" .
        cp LICENSE "$out/LICENSE"
        tar -czf "dist/release/spinoza_${version}_${goos}_${goarch}.tar.gz" -C "$out" "$binary" LICENSE
    done
    cd dist/release && sha256sum *.tar.gz > checksums.txt

check: lint test

ci: ci-go-build ci-go-test ci-go-lint ci-go-audit ci-fe-lint ci-fe-test ci-fe-audit ci-fe-build secrets sast vulns workflows hygiene docs sbom

clean:
    rm -f spinoza coverage.out
    rm -rf dist frontend/dist frontend/coverage web/dist/assets web/dist/index.html
