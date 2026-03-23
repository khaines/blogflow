.PHONY: build test lint fmt docker clean run dev

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

## build: Compile blogflow binary
build:
	go build -trimpath -ldflags="$(LDFLAGS)" -o bin/blogflow ./cmd/blogflow

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
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg DATE=$(DATE) -t blogflow .

## run: Run blogflow with defaults only (no external content needed)
run: build
	./bin/blogflow --dev

## dev: Run with local content directory for development
dev: build
	./bin/blogflow --dev $(if $(wildcard ./data/content),--content ./data/content) $(if $(wildcard ./data/theme),--theme ./data/theme)

## clean: Remove build artifacts
clean:
	rm -rf bin/ dist/ public/ coverage.out coverage.html
	go clean -cache -testcache

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
