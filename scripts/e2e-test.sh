#!/usr/bin/env bash
set -euo pipefail

# E2E test suite for blogflow running via Docker Compose.
# Starts the stack, tests every public endpoint, tears it down.

BASE_URL="${BASE_URL:-http://localhost:8080}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"
PASS=0
FAIL=0

cleanup() {
  echo ""
  echo "==> Tearing down containers …"
  docker compose -f "$COMPOSE_FILE" down -v --remove-orphans 2>/dev/null || true
}
trap cleanup EXIT

# ---------- helpers --------------------------------------------------------- #

check_status() {
  local description="$1" url="$2" expected_status="$3"
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" "$url")
  if [ "$status" -eq "$expected_status" ]; then
    echo "  ✅ $description (HTTP $status)"
    PASS=$((PASS + 1))
  else
    echo "  ❌ $description — expected $expected_status, got $status"
    FAIL=$((FAIL + 1))
  fi
}

check_body() {
  local description="$1" url="$2" expected_status="$3" body_pattern="$4"
  local status body
  body=$(curl -s -w "\n%{http_code}" "$url")
  status=$(echo "$body" | tail -n1)
  body=$(echo "$body" | sed '$d')
  if [ "$status" -ne "$expected_status" ]; then
    echo "  ❌ $description — expected HTTP $expected_status, got $status"
    FAIL=$((FAIL + 1))
    return
  fi
  if echo "$body" | grep -qi "$body_pattern"; then
    echo "  ✅ $description (HTTP $status, body matches \"$body_pattern\")"
    PASS=$((PASS + 1))
  else
    echo "  ❌ $description — body missing \"$body_pattern\""
    FAIL=$((FAIL + 1))
  fi
}

check_content_type() {
  local description="$1" url="$2" expected_status="$3" ct_pattern="$4"
  local status ct
  status=$(curl -s -o /dev/null -w "%{http_code}" "$url")
  ct=$(curl -sI "$url" | grep -i '^content-type:' | tr -d '\r')
  if [ "$status" -ne "$expected_status" ]; then
    echo "  ❌ $description — expected HTTP $expected_status, got $status"
    FAIL=$((FAIL + 1))
    return
  fi
  if echo "$ct" | grep -qi "$ct_pattern"; then
    echo "  ✅ $description (HTTP $status, content-type matches \"$ct_pattern\")"
    PASS=$((PASS + 1))
  else
    echo "  ❌ $description — content-type \"$ct\" missing \"$ct_pattern\""
    FAIL=$((FAIL + 1))
  fi
}

# ---------- start stack ---------------------------------------------------- #

echo "==> Building and starting containers …"
docker compose -f "$COMPOSE_FILE" up -d --build

echo "==> Waiting for service to be ready …"
RETRIES=30
until curl -sf "${BASE_URL}/healthz" >/dev/null 2>&1; do
  RETRIES=$((RETRIES - 1))
  if [ "$RETRIES" -le 0 ]; then
    echo "❌ Service did not become healthy in time"
    docker compose -f "$COMPOSE_FILE" logs
    exit 1
  fi
  sleep 1
done
echo "==> Service is healthy"

# ---------- tests ---------------------------------------------------------- #

echo ""
echo "==> Running endpoint tests …"

check_status   "GET /healthz → 200"                                 "$BASE_URL/healthz"                              200
check_status   "GET /readyz → 200"                                  "$BASE_URL/readyz"                               200
check_body     "GET / → 200, contains post titles"                  "$BASE_URL/"                                     200 "Hello"
check_body     "GET /posts/hello-world → 200, contains Hello"       "$BASE_URL/posts/hello-world"                    200 "Hello"
check_body     "GET /posts/markdown-features → 200, has chroma"     "$BASE_URL/posts/markdown-features"              200 "chroma"
check_status   "GET /posts/getting-started-with-blogflow → 200"     "$BASE_URL/posts/getting-started-with-blogflow"  200
check_status   "GET /pages/about → 200"                             "$BASE_URL/pages/about"                          200
check_status   "GET /tags/blogflow → 200"                           "$BASE_URL/tags/blogflow"                        200
check_content_type "GET /feed.xml → 200, xml content-type"          "$BASE_URL/feed.xml"                             200 "xml"
check_content_type "GET /sitemap.xml → 200, xml content-type"       "$BASE_URL/sitemap.xml"                          200 "xml"
check_body     "GET /metrics → 200, has blogflow metric"            "$BASE_URL/metrics"                              200 "blogflow_http_requests_total"
check_status   "GET /nonexistent → 404"                             "$BASE_URL/nonexistent"                          404

# ---------- summary -------------------------------------------------------- #

TOTAL=$((PASS + FAIL))
echo ""
echo "==> Results: $PASS/$TOTAL passed, $FAIL failed"

if [ "$FAIL" -gt 0 ]; then
  echo "❌ E2E tests FAILED"
  exit 1
fi

echo "✅ All E2E tests passed"
exit 0
