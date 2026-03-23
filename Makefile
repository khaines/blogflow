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
