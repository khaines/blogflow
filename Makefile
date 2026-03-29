.PHONY: build test lint fmt docker clean run dev smoke-test e2e k8s-lint docs-site

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

## smoke-test: Run container smoke tests locally (requires docker)
smoke-test: docker
	@set -e; \
	CONTAINER="blogflow-smoke-$$$$"; \
	PASS=0; FAIL=0; \
	cleanup() { docker rm -f "$$CONTAINER" >/dev/null 2>&1 || true; }; \
	trap cleanup EXIT; \
	docker run -d --name "$$CONTAINER" -p 0:8080 blogflow; \
	PORT=$$(docker port "$$CONTAINER" 8080 | head -1 | cut -d: -f2); \
	BASE="http://localhost:$$PORT"; \
	echo "⏳ Waiting for container to be healthy..."; \
	for i in $$(seq 1 20); do \
		if curl -sf --max-time 2 "$$BASE/healthz" >/dev/null 2>&1; then \
			echo "✅ Container healthy"; break; \
		fi; \
		if [ "$$i" -eq 20 ]; then \
			echo "❌ Container failed to become healthy"; docker logs "$$CONTAINER"; exit 1; \
		fi; \
		sleep 0.5; \
	done; \
	check() { \
		local url="$$1" expected_status="$$2" body_match="$$3" label="$$4"; \
		local response status body; \
		response=$$(curl -s -w '\n%{http_code}' --max-time 5 --connect-timeout 3 "$$url"); \
		status=$$(echo "$$response" | tail -1); \
		body=$$(echo "$$response" | sed '$$d'); \
		if [ "$$status" != "$$expected_status" ]; then \
			echo "❌ $$label — expected $$expected_status, got $$status"; \
			FAIL=$$((FAIL + 1)); return; \
		fi; \
		if [ -n "$$body_match" ] && ! echo "$$body" | grep -qi "$$body_match"; then \
			echo "❌ $$label — response body missing '$$body_match'"; \
			FAIL=$$((FAIL + 1)); return; \
		fi; \
		echo "✅ $$label"; PASS=$$((PASS + 1)); \
	}; \
	echo ""; echo "🧪 Running smoke tests..."; \
	check "$$BASE/healthz"     200 "ok"       "GET /healthz → 200"; \
	check "$$BASE/readyz"      200 "ready"    "GET /readyz → 200"; \
	check "$$BASE/"            200 "BlogFlow" "GET / → 200 (home page)"; \
	check "$$BASE/feed.xml"    200 "xml"      "GET /feed.xml → 200 (feed)"; \
	check "$$BASE/metrics"     200 "go_"      "GET /metrics → 200 (prometheus)"; \
	check "$$BASE/nonexistent" 404 ""         "GET /nonexistent → 404"; \
	echo ""; echo "Results: $$PASS passed, $$FAIL failed"; \
	if [ "$$FAIL" -gt 0 ]; then docker logs "$$CONTAINER"; exit 1; fi

## e2e: Run end-to-end tests via Docker Compose
e2e:
	./scripts/e2e-test.sh

## clean: Remove build artifacts
clean:
	rm -rf bin/ dist/ public/ coverage.out coverage.html
	go clean -cache -testcache

## docs-site: Run the in-repo documentation site locally
docs-site: build
	./bin/blogflow --dev --content ./site --config ./site/config

## k8s-lint: Validate K8s manifests and Helm chart with kubeconform
k8s-lint:
	kubeconform -strict -summary -ignore-filename-pattern 'kustomization.yaml' examples/k8s/
	helm template blogflow deploy/helm/blogflow/ | kubeconform -strict -summary
	helm template blogflow deploy/helm/blogflow/ --set sync.strategy=webhook --set sync.webhook.secret=ci-placeholder | kubeconform -strict -summary
	helm template blogflow deploy/helm/blogflow/ --set sync.strategy=sidecar --set sync.sidecar.repo=https://github.com/example/content.git | kubeconform -strict -summary
	helm template blogflow deploy/helm/blogflow/ --set ingress.enabled=true --set pdb.enabled=true | kubeconform -strict -summary

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
