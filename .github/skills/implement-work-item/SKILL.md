---
name: implement-work-item
description: >-
  Implements a GitHub issue end-to-end: verifies a design document exists, dispatches specialist
  agent personas to write code and tests, runs a build-test-fix loop until all unit and functional
  tests pass locally, opens a PR, and runs a custom review-fix-test loop (via `review-pr`) that
  re-validates tests after each fix round. Chains into the design-doc skill when no design document exists.
---

# Implement Work Item Skill — Orchestration Instructions

Implement a GitHub issue by writing production code and tests, validating the build, and opening a reviewed pull request. Read all supporting files before beginning:

- `.github/skills/implement-work-item/service-agent-map.md` — maps component labels to agent personas and checklists
- `.github/skills/implement-work-item/build-test-config.md` — build and test configuration per language
- `.github/skills/review-pr/agent-map.md` — file-pattern-to-agent mapping (used by fallback rules)
- `.github/skills/review-fix-loop/SKILL.md` — overview of review-fix loop; consensus and round structure referenced in Phase 8
- `.github/skills/review-fix-loop/dismissal-rules.md` — dismissal and consensus rules applied in Phase 8 Step B
- `.github/skills/review-pr/SKILL.md` — the PR review skill invoked during Phase 8
- `.github/skills/design-doc/SKILL.md` — the design-doc skill invoked in Phase 2 if no design doc exists

---

## Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `max_build_attempts` | 5 | Maximum build-fix iterations before aborting |
| `max_test_fix_rounds` | 5 | Maximum test-fix iterations per test stage (unit, functional) before aborting |
| `require_design_doc` | true | Abort if no design doc exists and auto-generation is declined |
| `auto_generate_design_doc` | true | Invoke the `design-doc` skill when no design doc is found |

---

## Phase 1: Issue Analysis & Readiness Check

### 1.1 Identify the Source Issue

Determine the GitHub issue that defines the work to be implemented:

- **Issue number provided explicitly**: Use `issue_read` with method `get` to fetch the issue title, body, labels, and linked issues. Also fetch comments with method `get_comments` and sub-issues with method `get_sub_issues`.
- **Issue URL provided**: Extract the owner, repo, and issue number from the URL, then fetch as above.
- **No issue provided**: Ask the user to provide an issue number or URL. This skill requires a source issue to proceed.

### 1.2 Validate Issue Readiness

Check that the issue meets the readiness criteria defined in `docs/engineering/issue-hierarchy-and-lifecycle.md`:

1. **Required labels**: The issue must have at least one `type:*` label, one `priority:*` label, and one `component:*` label. If any are missing, list what's missing and ask the user whether to proceed or abort.
2. **Acceptance criteria**: The issue body must contain testable acceptance criteria (checkboxes, numbered criteria, or a clearly marked section). If absent, ask the user to provide them before proceeding.
3. **Size**: Check the issue's size field or `size:*` label. Reject `XL` (> 5 days) — issues must be decomposed into smaller units per the issue-hierarchy rules. S, M, and L are acceptable.
4. **Not blocked**: Check for `status:blocked` label and `blocked-by #NNN` references in the issue body. If dependencies are unresolved, abort with a list of blocking issues.
5. **Not a duplicate**: Check if any open PRs already reference this issue. If a PR exists, warn and ask whether to proceed.

### 1.3 Extract Implementation Context

From the issue, extract:

1. **Component name**: Derive from the issue title or `component:*` label.
2. **Component domain**: The `component:*` label value (e.g., `server`, `content-pipeline`, `theme-engine`).
3. **Issue type**: The `type:*` label (feature, bug, spike, tech-debt).
4. **Acceptance criteria**: Parse into a structured list for test mapping in Phase 5.

### 1.4 Output Readiness Summary

```
🔍 Issue Readiness Check
━━━━━━━━━━━━━━━━━━━━━━━━
Issue:       #NNN — [Title]
Type:        [type:feature | type:bug | ...]
Priority:    [priority:p0–p3]
Component:   [component:XXX]
Size:        [S | M | L]
Status:      ✅ Ready | ❌ [reason]

Acceptance Criteria: N items found
Blocking Issues:     None | [list]
Existing PRs:        None | [list]
```

---

## Phase 2: Design Doc Verification

### 2.1 Search for Existing Design Doc

Search `docs/engineering/design/` for a design document matching the component:

- Search by component name: `docs/engineering/design/**/*<component-slug>*.md`
- Check the issue body for a direct link to a design doc.

### 2.2 Handle Design Doc State

- **Design doc exists**: Read the full document. Extract the following sections for use as implementation context:
  - §2 Logical Architecture — overall structure, boundaries, data flow. This section contains subsections for the data model/schema, API surface, dependencies, and related considerations.
  - §3 Functional Test Scenarios — happy-path, edge cases, integration boundaries, acceptance criteria mapping.
  - §5 Security — authentication/authorization model, data classification, input validation.
  - §7 Observability — logging, metrics, and tracing instrumentation plans.
  - §10 References — ADRs, requirements, and checklists.

  Note: Subsection numbering within each top-level section may vary between design documents. Locate content by topic heading (e.g., "Data Model", "API Surface") rather than relying on specific subsection numbers like §2.4 or §2.5.

- **No design doc exists and `auto_generate_design_doc` is true**: Invoke the `design-doc` skill with the same issue number. Wait for the skill to complete (it will generate the doc, open a PR, and run review-fix-loop). After completion:
  1. Wait for the design-doc PR to merge into `main`. If the PR is still open, ask the user to merge it before proceeding.
  2. Pull `main` to ensure the implementation branch (created in Phase 4) includes the design document.
  3. Read the merged design doc.
  If the design-doc skill fails or aborts, output a diagnostic message and abort the implement-work-item skill. Do not proceed to Phase 3 without a design document.

- **No design doc exists and auto-generation is declined**: Abort with a message explaining that a design doc is required. Suggest running the `design-doc` skill first.

### 2.3 Output Design Context Summary

```
📐 Design Document Context
━━━━━━━━━━━━━━━━━━━━━━━━━━
Design Doc:     docs/engineering/design/<path>
Status:         Approved | In Review | Draft
Architecture:   [brief summary from §2 Architecture overview]
API Surface:    [N endpoints/methods from §2 API Surface section]
Data Model:     [N entities from §2 Data Model section]
Test Scenarios: [N scenarios from §3]
Security:       [key concern from §5]
```

---

## Phase 3: Agent Selection & Context Assembly

### 3.1 Determine Implementation Agent

Read the service-agent-map (`.github/skills/implement-work-item/service-agent-map.md`) and match the issue's `component:*` label to determine:

- **Primary implementation agent**: The specialist persona that writes the code.
- **Secondary agents**: Additional personas needed for multi-domain work.
- **Language/framework**: Go (BlogFlow is a Go-only codebase).
- **Build and test commands**: From the build-test-config.

If the `component:*` label does not match any entry in the service-agent-map, apply the **Fallback Rules** defined in `service-agent-map.md` §Fallback Rules: infer from design doc file patterns (matched against the review-pr `agent-map.md`), then from issue type, and as a last resort, ask the user.

### 3.2 Load Engineering Context

Load the following reference documents for the implementation agent:

- **Always load**:
  - The design document (from Phase 2)
  - Go coding standards: `docs/engineering/checklists/03a-go-coding-standards.md`
  - Testing checklists: `docs/engineering/checklists/04a-unit-testing-checklist.md`, `docs/engineering/checklists/04b-integration-testing-checklist.md`
  - Security checklist: `docs/engineering/checklists/05-security-checklist.md`

- **Load if applicable** (based on service-agent-map checklists and design doc):
  - Logging checklist: `docs/engineering/checklists/09a-logging-checklist.md` (if listed in the service-agent-map or §7 defines logging)
  - Metrics checklist: `docs/engineering/checklists/09b-metrics-checklist.md` (if listed in the service-agent-map or §7 defines metrics)
  - Tracing checklist: `docs/engineering/checklists/09c-distributed-tracing-checklist.md` (if listed in the service-agent-map or §7 defines tracing)
  - ADRs referenced in the design doc's §10

- **Load existing code context**: If the component directory already exists (e.g., `cmd/<component>/`, `internal/<component>/`), read existing files to understand patterns, imports, and conventions already established.

### 3.3 Output Agent Selection Summary

```
🤖 Agent Selection
━━━━━━━━━━━━━━━━━━
Primary Agent:   [agent persona name]
Language:        Go
Build Command:   [command]
Unit Test:       [command]
Functional Test: [command]
Checklists:      [list of loaded checklists]
ADRs:            [list of loaded ADRs]
Existing Code:   [N files in component directory | New component]
```

---

## Phase 4: Implementation

### 4.1 Create Worktree Branch

Create a git worktree for the implementation work:

```bash
COMPONENT="<component-label>"
SLUG="<issue-slug>"
BRANCH_NAME="feat/${COMPONENT}/${SLUG}"
git worktree add /tmp/blogflow-impl-${SLUG} -b "$BRANCH_NAME" main
```

### 4.2 Dispatch Implementation Agent

Using the selected agent persona, implement the issue by translating the design document into working code. Provide the agent with:

1. **The full design document** — this is the primary technical specification.
2. **The issue body** — for acceptance criteria and business context.
3. **The coding standards checklist** — for language-specific conventions.
4. **Existing code** — for pattern consistency with the component.

The agent must:

- **Follow the design doc's architecture** (§2): create files in the correct directories, respect component boundaries, use the specified communication patterns.
- **Implement the API surface** from §2 (API Surface subsection): define HTTP endpoints with the specified request/response shapes, error codes, and rate limits.
- **Implement the data model** from §2 (Data Model subsection): create database schemas, models, or structs as specified.
- **Add structured logging** per §7 (Logging subsection) and checklist 09a: use slog (Go), include request_id and trace_id in context.
- **Add metrics** per §7 (Metrics subsection) and checklist 09b: instrument with RED metrics (Rate, Errors, Duration) using the metric names defined in the design doc.
- **Add tracing** per §7 (Tracing subsection) and checklist 09c: create spans for significant operations, propagate trace context across component boundaries.
- **Follow security requirements** from §5: implement auth/authz, input validation, content integrity checks as specified.

### 4.3 Multi-Domain Implementation

If the issue spans multiple domains (e.g., Go backend + HTML templates):

1. **Backend first**: Dispatch the backend agent to implement the Go logic. This establishes the contracts.
2. **Templates second**: Dispatch the front-end agent to implement template and theme changes, consuming the backend contracts from step 1.
3. Each agent works in the same worktree branch.

---

## Phase 5: Test Writing

### 5.1 Dispatch Test Writing

Using the same implementation agent, write tests:

**Unit tests** (from design doc §3 — happy-path and edge-case scenarios):

- One or more test per happy-path scenario defined in §3
- One or more test per edge case / error scenario defined in §3
- Table-driven tests preferred for Go (per 03a coding standards)
- Follow the unit testing checklist (04a): scope boundaries, isolation, assertions, coverage targets

**Functional / integration tests** (from design doc §3 — integration boundaries and acceptance criteria):

- Tests for integration boundaries defined in §3
- Use Testcontainers for database, cache, and message broker dependencies (per 04b checklist)
- Mock external services; use real instances for internal dependencies where feasible

**Acceptance criteria mapping**:

- Every acceptance criterion from the issue must map to at least one test
- If a criterion cannot be tested locally (e.g., requires production infrastructure), flag it in the completion report and add a `// TODO: requires staging environment` comment

### 5.2 Test File Placement

Follow project conventions:

- Go: `*_test.go` in the same package (unit), `test/integration/` or `*_integration_test.go` (functional)

---

## Phase 6: Build-Test-Fix Loop

This phase ensures the implementation compiles and all tests pass before opening a PR. Read the build-test-config (`.github/skills/implement-work-item/build-test-config.md`) for commands and configuration.

### 6.1 Build

Run the build command for the project:

```bash
cd /tmp/blogflow-impl-<slug>
go build ./...
```

- **Build succeeds** → proceed to §6.2.
- **Build fails** → enter build-fix loop.

**Build-Fix Loop**:

1. Parse the build error output. Extract: file, line, error message, error type (syntax, type, import, linker).
2. Dispatch the implementation agent with the error output and instructions to fix only the compilation errors.
3. Re-run the build.
4. Repeat up to `max_build_attempts` (default: 5).
5. If the build still fails after max attempts → **abort**. Output a diagnostic report:

```
❌ Build Failed — Aborting
━━━━━━━━━━━━━━━━━━━━━━━━━
Attempts:    N / max_build_attempts
Last Error:  [error summary]
Files:       [list of files with errors]

Recommendation: Review the design doc and implementation for mismatches.
```

### 6.2 Unit Tests

Run the unit test command:

```bash
go test -v -count=1 -race ./...
```

- **All tests pass** → proceed to §6.3.
- **Tests fail** → enter test-fix loop.

**Test-Fix Loop**:

1. Parse the test output. Extract: test name, file, line, assertion, expected value, actual value, error message.
2. Classify each failure:
   - **Implementation bug**: The test expectation is correct but the code doesn't satisfy it. Fix the implementation.
   - **Test bug**: The test expectation is incorrect (e.g., wrong expected value, setup error). Fix the test.
   - **Heuristic**: If the test was written from the design doc's acceptance criteria, prefer fixing the implementation. If the test contradicts the design doc, prefer fixing the test.
3. Dispatch the implementation agent with the failure details and classification.
4. Re-run the build (to catch any new compilation errors from the fix), then re-run unit tests.
5. Repeat up to `max_test_fix_rounds` (default: 5).
6. If tests still fail after max rounds → **abort** with diagnostic report.

### 6.3 Functional Tests

Run the functional / integration test command:

```bash
go test -v -count=1 -race -tags=integration ./...
```

Apply the same test-fix loop logic as §6.2. Functional tests may require:

- Starting dependent services via Testcontainers or docker-compose
- Setting environment variables for test configuration
- Longer timeouts (configure via build-test-config)

### 6.4 Output Build-Test Summary

```
✅ Build & Test — All Green
━━━━━━━━━━━━━━━━━━━━━━━━━━━
Build:           ✅ Passed (attempt N)
Unit Tests:      ✅ N passed (fix rounds: N)
Functional Tests:✅ N passed (fix rounds: N)
Total Duration:  ~N minutes
```

---

## Phase 7: Git Workflow & PR

### 7.1 Commit Changes

Stage and commit all changes with a descriptive message:

```bash
cd /tmp/blogflow-impl-<slug>
git add -A
git commit -m "feat(<component>): <brief summary from issue title>

Implements <component> per design doc and issue #NNN.
Includes unit and functional tests for all acceptance criteria.

- <key implementation detail 1>
- <key implementation detail 2>
- <key implementation detail 3>

Refs #NNN

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

### 7.2 Push and Open PR

Push the branch and create a PR:

```bash
git push -u origin "$BRANCH_NAME"
```

Create the PR with:

- **Title**: `feat(<component>): <summary from issue title>`
- **Body**:
  ```markdown
  ## Summary

  Implements [component name] per the [design document](link) and issue #NNN.

  ### Changes
  - [Key change 1]
  - [Key change 2]
  - [Key change 3]

  ### Acceptance Criteria
  - [x] Criterion 1
  - [x] Criterion 2
  - [x] Criterion 3

  ### Test Coverage
  - Unit tests: N tests covering happy-path, edge cases
  - Functional tests: N tests covering integration boundaries

  ### Design Document
  - [Component Design Doc](link)

  ### Build & Test Results
  - Build: ✅ Passed
  - Unit tests: ✅ N passed
  - Functional tests: ✅ N passed
  ```
- **Labels**: Copy labels from the source issue
- **Linked issue**: `Refs #NNN`

---

## Phase 8: Review-Fix-Test Loop

This phase implements a custom review-fix-test loop rather than delegating directly to the `review-fix-loop` skill, because test re-validation must occur between each fix-and-review cycle. The autonomous `review-fix-loop` does not support injecting external commands between its internal rounds.

### 8.1 Review-Fix-Test Cycle

Repeat the following cycle until termination (§8.2):

**Step A — Review**: Invoke the `review-pr` skill on the PR. Collect findings.

**Step B — Evaluate**: Apply the review-fix-loop's dismissal and consensus rules (from `.github/skills/review-fix-loop/dismissal-rules.md`) to filter actionable findings. If no actionable findings remain, terminate with success.

**Step C — Fix**: Dispatch the appropriate agent persona(s) to fix actionable findings. The agent selection follows the same routing used in Phase 3 (primary agent for implementation fixes, secondary agents for domain-specific fixes).

**Step D — Build-Test Validation**: Re-run the full build-test sequence from Phase 6:

1. Run the build command. If it fails, fix the build error before continuing.
2. Run unit tests. If any fail, classify the failure:
   - **Review fix broke existing test** → revert the review fix that caused the regression (up to `max_review_regression_fixes` attempts per round, default: 3).
   - **Review fix correctly exposed a bug** → fix the implementation.
3. Run functional tests. Apply the same regression classification.

**Step E — Commit & Push**: Commit all fixes with message:

```
Review fixes (Round N): Address N findings

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

Push to the PR branch.

**Step F — Loop**: Return to Step A for the next review round.

### 8.2 Termination

The review-fix-test loop terminates when:

- No new actionable findings AND all tests pass → **success**
- Target rating achieved AND all tests pass → **success**
- Max rounds reached (use the review-fix-loop's `max_rounds` default of 5) → **stop** with current state (tests must still be green)
- A review fix cannot be reconciled with passing tests after `max_review_regression_fixes` attempts → **stop** and revert the problematic fix

---

## Phase 9: Completion Report

After all phases complete, output a final summary:

```
🚀 Implementation Complete
━━━━━━━━━━━━━━━━━━━━━━━━━
Issue:       #NNN — [Title]
Component:   [Component Name]
Branch:      feat/<component>/<slug>
PR:          #NNN
Design Doc:  docs/engineering/design/<path>

Implementation:
  Files created:     N
  Files modified:    N
  Lines added:       N

Tests:
  Unit tests:        N passed, 0 failed
  Functional tests:  N passed, 0 failed
  Build-fix rounds:  N
  Test-fix rounds:   N

Review:
  Review rounds:     N
  Final rating:      ⭐⭐⭐⭐⭐

Acceptance Criteria:
  ✅ Criterion 1
  ✅ Criterion 2
  ✅ Criterion 3
  ⚠️ Criterion 4 — requires staging validation

Commits:
  abc1234 — feat(server): implement content rendering
  def5678 — Review fixes (Round 1): Address N findings
  ghi9012 — Review fixes (Round 2): Address N findings
```

### 9.1 Worktree Cleanup

After the completion report, remove the worktree:

```bash
git worktree remove /tmp/blogflow-impl-<slug>
```

---

## Important Notes

- **Design doc is required.** Every implementation must have a design document. If none exists, the skill invokes the `design-doc` skill to generate one. This ensures architecture, security, and observability are considered before code is written.
- **XL issues are rejected.** Issues sized XL (> 5 days) must be decomposed into smaller units per the issue-hierarchy rules. The skill will not attempt to implement an XL issue.
- **Tests must pass before PR.** The build-test-fix loop (Phase 6) is a hard gate. No PR is opened until the build is green and all unit + functional tests pass locally.
- **Review fixes must not break tests.** Phase 8 implements a custom review-fix-test loop (not a direct delegation to `review-fix-loop`) that re-runs build + tests after every fix round. If a fix introduces a regression, the fix is reverted rather than the test being weakened.
- **Prefer fixing implementation over tests.** When a test fails, assume the test expectation (derived from the design doc and acceptance criteria) is correct. Fix the implementation first. Only fix the test if it demonstrably contradicts the design doc or issue requirements.
- **Multi-domain sequencing.** For issues spanning Go backend and HTML templates, implement backend first to establish contracts, then templates consuming those contracts. Never implement templates against assumed contracts.
- **Issue lifecycle.** The skill does not move the issue status on the project board (this may require GitHub Actions). However, it links the PR to the issue via `Refs #NNN` so the issue can be tracked through the PR's lifecycle.
- **Worktree cleanup.** Always remove the worktree after completion or abort. This prevents stale worktrees from accumulating.
- **Abort is acceptable.** If the build or tests cannot be fixed within the configured max attempts, the skill aborts cleanly with a diagnostic report. This is preferable to opening a PR with broken code.
