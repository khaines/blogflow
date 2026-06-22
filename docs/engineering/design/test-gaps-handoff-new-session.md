# Test Gap Issues — Handoff for New Session

**Context:** We created 18 GitHub issues (#216–#235) addressing test coverage gaps identified from design specs (configuration-system.md, theme-development.md CI patterns). All have descriptions uploaded with acceptance criteria per issue pattern.

---

## Remaining Work Before PR Submission

For each issue (#217–#235; #216 exists pre-existing):

### Step 1 — Implement Test Code
- Write test stubs matching design doc requirements (e.g., CSP header on /metrics endpoint, symlink escape rejection validation logic in context.go)
- Follow patterns from existing tests: internal/config/loader_logging_test.go for redaction, cmd/blogflow/reload_test.go for atomic pointer swaps

### Step 2 — RFL Review-Fail Loop Checks
Before submitting PR (per `docs/engineering/design/test-gap-analysis.md`):
```bash
go build ./... && go test -race -v ./... && helm lint deploy/helm/blogflow/ 2>&1 | grep -iE 'error|fail|warn' || echo 'CI PASSED'; ls -la .workbranch* reviews.txt ; [ "$?" == "0" ] && echo 'RFL OK' || echo 'Check gaps!'
```

### Step 3 — Submit PR with Acceptance Criteria Met
- Title: `test([issue-id]) <scenario> (#<issue-number>)` example: `test(#217) symlink escape detection for layered overlay FS (#217)`  
- Description references design doc sections + handoff checklist items marked ✓ after passing local CI steps (lint→build→race test→helm-lint)

--- 

## New Session Prompt for Developer Pick-up

**Copy-paste this:**

```
Context: BlogFlow test-gap issues #216–#235 created by previous agent. Each has description + acceptance criteria in GitHub. Your task: pick up first issue (#217), write test code following existing patterns (internal/config/loader_logging_test.go for secret redaction examples, internal/overlayfs/context_otel_test.go for tracing metrics after operations complete), run `go build && go test -race`, then submit as PR titled `test(#217) symlink escape...` once CI passes. See full scenario details in original issue body text referencing design specs (e.g., configuration-system.md §6.3 Content Integrity).
```

--- 

**Status:** Handoff doc created for new session pick-up after RFL review loop passes per PR template checklist requirements covering all test-gap scenarios listed above with acceptance criteria embedded per design doc line reference rules from CI workflow steps defined in `.github/workflows/ci.yml` lint-test-helm-lint-kubeconform.
