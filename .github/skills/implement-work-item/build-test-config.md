# Implement Work Item — Build & Test Configuration

This file defines the build and test commands, thresholds, and failure-parsing patterns used by the `implement-work-item` skill during Phase 6 (build-test-fix loop) and Phase 8 (post-review-fix test validation).

---

## Language Configuration

### Go

| Command | Purpose | Default |
|---------|---------|---------|
| **Build** | `go build ./...` | Compile all packages |
| **Lint** | `golangci-lint run ./...` | Static analysis (run before tests, fix warnings) |
| **Unit tests** | `go test -v -count=1 -race ./...` | Run all tests with race detector |
| **Functional tests** | `go test -v -count=1 -race -tags=integration ./...` | Run integration-tagged tests |
| **Coverage** | `go test -coverprofile=coverage.out -covermode=atomic ./...` | Generate coverage report |

**Go-specific notes:**

- Use `-count=1` to disable test caching during the fix loop (stale cache masks regressions).
- Use `-race` to detect data races at test time.
- Functional tests use the `integration` build tag. Tests requiring external services (database, cache, message broker) must be tagged `//go:build integration`.
- The build command includes `./...` to compile all packages, not just the component being implemented. This catches import errors in dependent packages.

---

## Loop Configuration

### Build-Fix Loop

| Parameter | Default | Description |
|-----------|---------|-------------|
| `max_build_attempts` | 5 | Maximum build-fix iterations. Each iteration: fix → rebuild. |
| `build_timeout` | 120s | Maximum time for a single build command. Kill and abort if exceeded. |

### Test-Fix Loop

| Parameter | Default | Description |
|-----------|---------|-------------|
| `max_test_fix_rounds` | 5 | Maximum test-fix iterations per stage (unit, functional). Each round: fix → rebuild → retest. |
| `unit_test_timeout` | 300s | Maximum time for unit test suite. |
| `functional_test_timeout` | 600s | Maximum time for functional test suite. |
| `prefer_fix_implementation` | true | When a test fails, prefer fixing implementation over changing test expectations. |

### Post-Review Test Validation (Phase 8)

| Parameter | Default | Description |
|-----------|---------|-------------|
| `retest_after_review_fix` | true | Re-run build + tests after each review-fix round. |
| `max_review_regression_fixes` | 3 | Maximum attempts to fix a test regression caused by a review fix before reverting. |

---

## Failure Parsing Patterns

### Go Build Errors

```
Pattern: <file>:<line>:<col>: <message>
Example: internal/server/handler.go:42:15: undefined: ContentStore

Extract:
  - file: internal/server/handler.go
  - line: 42
  - col: 15
  - message: undefined: ContentStore
  - type: compilation
```

### Go Test Failures

```
Pattern: --- FAIL: <TestName> (<duration>)
Followed by: <file>:<line>: <assertion message>
Example:
    --- FAIL: TestRenderPost (0.01s)
        handler_test.go:58: expected status 200, got 404

Extract:
  - test_name: TestRenderPost
  - file: handler_test.go
  - line: 58
  - message: expected status 200, got 404
  - duration: 0.01s
  - type: assertion_failure
```

### Go Race Condition

```
Pattern: WARNING: DATA RACE
Followed by: goroutine N at <file>:<line>

Extract:
  - type: data_race
  - goroutines: [list of goroutine locations]
  - severity: high (always fix, never ignore)
```

---

## Fix Classification Heuristics

When a test fails, classify the failure to determine what to fix:

| Signal | Classification | Action |
|--------|---------------|--------|
| Test expects a value from the design doc's acceptance criteria | **Implementation bug** | Fix the implementation to match the expected value |
| Test expects a value not mentioned in the design doc or issue | **Test bug** | Fix the test to align with the design doc |
| Test setup fails (missing mock, database error, timeout) | **Test infrastructure** | Fix the test setup, not the implementation |
| Race condition detected | **Implementation bug** | Always fix the implementation; never suppress race detection |
| Import or type error in test file | **Test compilation** | Fix the test file |
| Import or type error in implementation file | **Implementation compilation** | Fix the implementation file |
| Multiple tests fail with the same root cause | **Shared implementation bug** | Fix the root cause once, re-run all affected tests |

---

## Coverage Targets

Reference the unit testing checklist (04a) for coverage expectations:

| Layer | Target | Notes |
|-------|--------|-------|
| Business logic | ≥ 90% | Core domain logic, validation, calculations |
| API handlers | ≥ 80% | Request handling, error responses |
| Infrastructure | ≥ 60% | Database queries, cache operations, external clients |

Coverage is tracked but not a hard gate for the build-test-fix loop. Low coverage is flagged in the completion report and addressed during the review-fix-test loop (Phase 8).
