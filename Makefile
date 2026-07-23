.PHONY: deps build run dev-api dev-web tidy vet test clean

deps:
	cd frontend && npm ci

build:
	cd frontend && npm run build
	go build -o spinoza .

run: build
	./spinoza

dev-api:
	go run . --addr 127.0.0.1:34115

dev-web:
	cd frontend && npm run dev

tidy:
	go mod tidy

vet:
	go vet ./...

test:
	go test ./...

clean:
	rm -f spinoza
	rm -rf frontend/dist web/dist/assets
	git checkout -- web/dist/index.html 2>/dev/null || true
