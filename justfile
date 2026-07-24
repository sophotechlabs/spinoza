default:
    @just --list

deps:
    cd frontend && npm ci

tidy:
    go mod tidy

build:
    cd frontend && npm run build
    go build -o spinoza .

run: build
    ./spinoza

build-desktop:
    wails build -tags desktop -skipbindings

dev-desktop:
    wails dev -tags desktop -skipbindings

dev-api:
    go run . --addr 127.0.0.1:34115

dev-web:
    cd frontend && npm run dev

test-be:
    go test -race -covermode=atomic -coverprofile=coverage.out ./internal/... .
    go tool cover -func=coverage.out

test-fe:
    cd frontend && npm run test:coverage

test: test-be test-fe

lint-be:
    golangci-lint run ./...
    golangci-lint run --build-tags desktop ./...
    go vet ./...
    go vet -tags desktop ./...

lint-fe:
    cd frontend && npm run lint
    cd frontend && npm run typecheck
    cd frontend && npm run format:check

lint: lint-be lint-fe

audit-be:
    govulncheck ./internal/... .

audit-fe:
    cd frontend && npm run knip
    cd frontend && npm run depcheck
    cd frontend && npm run madge

audit: audit-be audit-fe

fmt:
    golangci-lint fmt
    cd frontend && npm run format

check: lint test

clean:
    rm -f spinoza coverage.out
    rm -rf frontend/dist frontend/coverage web/dist/assets web/dist/index.html
