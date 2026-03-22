.PHONY: build test lint fmt docker clean run dev

## build: Compile blogflow binary
build:
	go build -trimpath -ldflags="-s -w" -o bin/blogflow ./cmd/blogflow

## test: Run unit tests with race detector
test:
	go test -v -count=1 -race ./...

## lint: Run golangci-lint static analysis
lint:
	golangci-lint run ./...

## fmt: Format Go source files
fmt:
	gofumpt -w .

## docker: Build Docker image
docker:
	docker build -t blogflow .

## run: Run blogflow server locally
run: build
	./bin/blogflow serve --dev --watch --content ./data/content --theme ./data/theme

## dev: Run with live reload (requires content directory)
dev: build
	./bin/blogflow serve --dev --watch --content ./data/content

## clean: Remove build artifacts
clean:
	rm -rf bin/ dist/ public/ coverage.out coverage.html
	go clean -cache -testcache

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
