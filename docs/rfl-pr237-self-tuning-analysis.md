# PR #237 RFL Self-Tuning Analysis

> **Date**: 2026-06-22
> **Reviewer**: Local RFL council (qwen3.6:35b-256k)
> **Subject**: Self-tuning for findings missed during PR #237 review

## 1. Summary

Our local RFL council converged to 5/5 APPROVE (5/5, 5/5, 4/5, 5/5 + advisory). An independent frontier council (Opus-4.8, GPT-5.4, Gemini-3.1) found 8 High + 7 Medium findings. **Every finding was missed by our council.** This analysis verifies three findings independently, root-causes why each seat missed them, and proposes durable prompt fixes.

## 2. Independent Verification of Three Critical Findings

### H1: IP Allowlist Bypass (confirmed)

**Evidence**: `sync.go:47` creates `NewWebhookStrategy(cfg.Webhook, reloader, logger)` — no `SetIPResolver` call anywhere in the PR. `webhook.go:221-225` shows that when `ipResolver` is `nil`, `resolveIP()` falls back to `remoteIP()` which returns the leftmost `X-Forwarded-For` token **unconditionally**.

**Impact**: An attacker bypasses the entire IP allowlist by setting `X-Forwarded-For: <allowed-ip>`. The allowlist config value is read, checked against the wrong source.

**Verified**: Yes

### H4: Negative Cache Eviction Tests Always See 0 (confirmed)

**Evidence**: Both tests use `WithLayerNames(["defaults"])` or `WithLayerNames(["test"])` — a single layer. The production code at `overlayfs.go:239` only writes to `negCache` for layer index `i > 0`. With a single layer, there is no non-base layer to miss. negCacheCount stays 0.

**Impact**: The test asserts "cache bounded at < 100K entries" when the count is actually 0 (no negative cache entries ever written). The test passes even if the negative cache feature is deleted.

**Verified**: Yes

### H7: Dead Type Assertions in readingTime Tests (confirmed)

**Evidence**: `theme.go:194` registers `readingTime` as `func(content any) int`. The test at `funcmap_edge_test.go:44` and `:120` asserts `fm["readingTime"].(func(string) int)`. Since the type assertion fails (`ok=false`), the entire test body is **silently skipped**.

**Impact**: If `readingTime` were deleted, this test would still pass (empty if body). Even the "100k spaceless chars = 1 word, not ~500" test that would FAIL silently passes because it's nested inside the failing type assertion.

**Verified**: Yes

## 3. Root Cause Analysis: Why Each Seat Missed the Findings

### rfl-security missed: H1 (IP bypass), H8 (symlink stub), M1 (auth ordering)

**Why**: The security seat's mandate says "evaluate against your domain's standards" — but the standards are not operationalized as **concrete checks**. The spec doesn't include a step that says:

> "For every security control (allowlist, HMAC, symlink check), trace the input back to its construction point. Does the safe path exist AND get called, or is it dead?"

The security seat saw that `SetIPResolver` method **exists** on `WebhookStrategy` and assumed it was wired (it's a setter that could be called post-construction, so it looked complete). It did **not** check that `sync.go` never calls it.

**Procedure Gap**: No "call-graph verification" step. No "does the test invoke production code, or just stdlib?" check. The spec says "Read the actual diff/changed files" — but the new allowlist tests only test `X-Forwarded-For` paths, never the production `RemoteAddr` path.

### rfl-systems missed: H2-H7 (all vacuous tests)

**Why**: The systems seat mandate says "Go implementation correctness" — but doesn't include specific **test-efficacy heuristics**. The spec didn't include:

1. **Mutation test of intent**: "Would this fail if the production feature were deleted?"
2. **Production-code tracing**: "Does the test call the real function, or test-local stubs/stdlib?"
3. **Type-signature liveness**: "When a test casts to `func(X)`, confirm the production registration matches `func(Y)`"
4. **Threshold reachability**: "Can the inputs actually reach the bound? (e.g., 26 keys vs 100K cap)"
5. **Dead assertion detection**: "Does a failing type assertion silently skip the test body?"

Without these, the systems seat evaluates test files by **reading the test**, not **mutating its intent**. It sees "the test calls `ReadFile` and checks `negCacheCount`" and concludes "coverage is exercised" — missing that the negCache is never written because the precondition (2+ layers) isn't met.

Also: the systems seat sees `funcmap_edge_test.go` has `fm["readingTime"](...)` and assumes the type assertion succeeds — never checking that `theme.go:194` uses `func(any) int`.

### rfl-sre missed: H2-H4, M2, M4, L1 (vacuous tests + metrics test hitting wrong mux)

**Why**: The SRE seat mandate focuses on "SLOs, blast-radius, resource limits, runbooks" — none of which are relevant to false test coverage, dead assertions, or CSP-header-on-metrics. The seat has no checklist for:

1. **Metrics-server verification**: When the PR changes the metrics port handling, verify the test targets `s.metricsServer`, not `s.httpServer`
2. **Assertion liveness**: Flag `_ = resp.Code` with no later assertion
3. **Tautology detection**: `context.Background().Err() != nil` is always `nil` — flag tautologies

### rfl-architect missed: M5 (config drift), M6 (gap-analysis inaccuracy)

**Why**: The architect mandate covers "system architecture, overlay FS, content pipeline topology, resource/quota design" — not config-field naming or design-doc accuracy against implementation. The spec focuses on **architecture** not **conformance**.

### rfl-privacy missed: M7 (ephemeral docs in design directory)

**Why**: The privacy seat focuses on "privacy/compliance" — document hygiene is tangential. The spec lacks any check for "are design docs following the template?"

## 4. Proposed Prompt Edits

### rfl-security.md — Add 4 concrete checks

**Add to the "Method" section after step 2:**

```markdown
## Additional Security Checks
### 4.1 Security-Control Input Tracing (REQUIRED)
For every allow/deny/skip control in the PR (IP allowlist, HMAC, symlink check, event filtering, branch filtering):
- Trace the input **back to its construction point**. Does the safe path exist AND get called, or is it dead?
- Example: `SetIPResolver` exists on `WebhookStrategy` → verify a caller actually invokes it. Existence ≠ usage.
- Example: `checkSymlinkSafe()` exists → verify a test exercises it with actual symlinks, not just prose.

### 4.2 Authentication-Before-Processing Order (REQUIRED)
For any handler that branches on attacker-controlled headers (X-GitHub-Event, X-Forwarded-For, etc.):
- Verify HMAC/signature verification happens **before** any branching on request content.
- Flag any differentiated error responses that leak information to unauthenticated callers.

### 4.3 Test-Driven Security Verification
When new security controls have new tests:
- Does the test exercise the **safe** input path (not just the spoofable one)?
- Would the test fail if the production control were removed?
- Does the test cover the bypass vector (e.g., XFF spoof when RemoteAddr is the intended input)?

### 4.4 Dead Config Pattern Detection
- If a setter/config option exists (e.g., `SetIPResolver`, `AllowedEvents`) that isn't wired in the construction path, flag it as **dead config** (defense-in-depth gap).
```

### rfl-systems.md — Add 5 concrete test-efficacy checks

**Add to the "Method" section after step 2:**

```markdown
## Additional Systems Checks
### 4 Test-Efficacy Heuristics (REQUIRED for all test files)

For **every** new or modified test file, apply these checks before approving:

#### 4.1 Mutation Test of Intent
Ask: "Would this test fail if the production feature it claims to cover were deleted or inverted?"
- If YES (test would still pass) → false coverage → MAJOR finding
- Sub-checks: does the test call production code or only stdlib/test-local vars?
  - `fstest.MapFS` round-trip ≠ overlay FS override
  - `sync.Mutex` ≠ engine's atomic swap
  - `html/template.New().Parse()` ≠ theme loader's duplicate detection
  - `context.Background().Err() != nil` is always `nil` → tautology

#### 4.2 Production-Code Tracing
For every test: trace the call chain from `t.Run` to the **real** production function.
- Does it call the BlogFlow function from the package being tested?
- Or does it call stdlib, `testing/fstest.MapFS`, local structs, or test-local mocks?
- Example: `css_override_test.go` writes to `fstest.MapFS` and reads back → tests MapFS, not OverlayFS

#### 4.3 Type-Signature Liveness
For every test that does `fn := fm["name"].(func(...))`:
- Verify the asserted signature matches the production registration: `grep -n "name.*func" *.go`
- If signatures don't match → the `ok` check is `false` → entire test body is silently skipped
- Even assertions that would FAIL are skipped

#### 4.4 Threshold Reachability
For tests involving limits/caps/evictions:
- Compute whether inputs can actually reach the bound
- Example: 26 distinct keys vs a 100K cap → never triggers eviction
- Example: negCache only writes for layer index `i > 0` → needs ≥2 layers
- Example: precondition for cache hit vs miss: does the test create the right conditions?

#### 4.5 Assertion Liveness
- Flag tautologies: `context.Background().Err() != nil` is always `nil`
- Flag `_ = err` with no later assertion
- Flag `t.Log` as the only verification (not `t.Error`/`t.Fatal`)
- Flag `"all non-5xx"` claims when no response code is checked
```

### rfl-sre.md — Add 3 concrete checks

**Add to the "Method" section after step 2:**

```markdown
## Additional SRE Checks
### 4 Metrics-Path Verification (REQUIRED when port/metrics changes)
When the PR changes metrics endpoint handling (port, middleware, CORS):
- Does the test exercise `s.metricsServer.Handler` with `GET /metrics`, not `s.httpServer`?
- Would the test pass if the metrics-side changes were reverted?
- Verify recovery middleware is retained on all routes (not just primary mux)

### 5 Assertion-Liveness Check
For every test's assertion:
- Flag `_ = resp.Code` or `_ = response` with no later assertion as non-verification
- Flag `t.Log` as insufficient when `t.Error`/`t.Fatal` would be expected
- Flag assertions that check constants vs behavioral properties (e.g., "reads < 64MB" constant vs actual rejection test)

### 6 Config-Field Naming Conformance
When the PR changes config fields:
- Compare field names/types against the governing design doc (e.g., `allowed_ips []string` vs documented `ip_allowlist bool`)
- Flag any divergence between implementation and spec
```

### rfl-architect.md — Add 2 concrete checks

**Add to the "Method" section after step 2:**

```markdown
## Additional Architect Checks
### 4 Design-Doc Conformance
When implementation adds/changes fields, types, or behaviors:
- Compare against the governing design doc (e.g., `test-gap-analysis.md`, `configuration-system.md`)
- Flag items claimed as "closed" in the PR that have no test or prod-code coverage
- Flag items in the gap-analysis doc that **already** have tests (doc is lying)
- If items are marked "stub per design spec" → confirm this is acceptable per the spec

### 5 PR Claims Verification
For every claim in the PR title/description:
- "complete test gap analysis" → verify each gap has a real test (not false coverage)
- "fixes #N, #M" → verify each issue has corresponding prod code + test, not just a log line
```

### rfl-privacy.md — Add 1 concrete check

**Add to the "Method" section after step 2:**

```markdown
## Additional Privacy Checks
### 4 Design-Dir Hygiene
When the PR modifies `docs/engineering/design/`:
- Are new files following `000-template.md`? Or are they ephemeral session notes?
- Flag files with agent-specific language ("previous agent", "copy-paste this", RFL terminology)
- Flag files with absolute paths, session context, or cross-references to non-production artifacts
```

### rfl-review.md (orchestrator) — Add convergence gates

**Add after "compute the result" section:**

```markdown
## Required Per-Seat Output (enforced by orchestrator)
Each seat MUST include explicit evidence for their verdict:
- "I verified test X by reading file Y and confirming Z" — or "I did not verify test X because..."
- For security seats: explicit statement about whether the safe path is wired
- For systems/SRE seats: explicit statement about whether vacuous/false-coverage tests were checked
- **Seat is not permitted to approve without explicitly checking for:**
  1. vacuous/dead tests (would it fail if the feature were deleted?)
  2. type-signature liveness (does the test cast match the production registration?)
  3. security-control wire-up (does the setter get called, or is it dead config?)
  4. metrics-path verification (does the test hit the right mux/port?)
  5. threshold reachability (can the test inputs actually trigger the code path?)
```

## 5. Self-Evaluation of Proposed Edits

### Do the edits close the gaps found in PR #237?

| Finding | Original Gap | New Check | Closes? |
|---------|-------------|-----------|---------|
| H1 (IP bypass) | No "call-chain verification" for setters | Security 4.1 | ✓ |
| H2-H7 (vacuous tests) | No test-efficacy heuristics | Systems 4.1-4.5 | ✓ |
| H8 (symlink stub) | Security checked existence, not usage | Security 4.1 "dead config" | ✓ |
| M1 (auth ordering) | No auth-order check | Security 4.2 | ✓ |
| M2 (wrong mux) | No port/metrics-specific check | SRE 4 | ✓ |
| M3 (32-byte boundary) | No "does test reach the threshold" check | Systems 4.4 | ✓ |
| M4 (no assertions) | No assertion-liveness check for SRE | SRE 5 | ✓ |
| M5 (config drift) | No config-field naming conformance | SRE 6 | ✓ |
| M6 (doc inaccuracy) | No PR-claims verification | Architect 5.1 | ✓ |
| M7 (ephemeral docs) | No design-dir hygiene | Privacy 4 | ✓ |
| L1 (t.Log not t.Error) | No assertion-liveness check | SRE 5 | ✓ |
| L5 (constant check) | No "behavioral vs constant" check | SRE 5 | ✓ |

**All 15 findings are covered by at least one new required check.**

### Edge cases the edits don't cover

1. **Low-signal false negatives**: If tests call real production functions with valid inputs but the code has a logic bug (e.g., `readingTime` counts bytes not words), the existing checks may miss it. The proposed checks address false coverage but not subtle correctness errors.

2. **Performance regressions**: The PR's performance tests use `t.Log` for thresholds, not `t.Error`. The new "assertion liveness" check addresses this, but only if the reviewer explicitly invokes it.

3. **Cross-file dependencies**: Some findings (like H1) span files (setter exists in one file, not called in another). The orchestrator enforces per-seat evidence but doesn't explicitly require cross-file tracing.

### Do the edits introduce new risks?

- **False positives**: The "vacuous test" check is binary (call production code → yes/no). Tests that use `fstest` intentionally (e.g., "test MapFS behavior") might be flagged when they shouldn't be. **Mitigation**: The check says "Would this fail if deleted?" — a MapFS round-trip would still pass → flagged correctly.
- **Review fatigue**: Adding 5-6 mandatory checks per seat could cause review degradation over time. **Mitigation**: Keep the checks as concrete examples, not an exhaustive checklist. The spirit (question assumptions) matters more than the checklist.

## 6. Conclusion

Our local RFL council converged to APPROVE on a PR that failed an independent frontier council at 1/5. The root cause was **over-reliance on green CI and insufficient procedural rigor** — specifically:

1. **Our specs don't require mutation testing** (would the test fail if the feature were deleted?)
2. **Our specs don't require call-chain verification** for setters/config (would we assume `SetIPResolver` being called because it exists?)
3. **Our specs don't require type-signature liveness checks** (would we notice a cast silently fails?)
4. **Our specs don't require threshold reachability analysis** (would we accept 26 keys vs 100K cap?)
5. **Our specs don't require cross-file verification** (does the test hit the right mux?)

The proposed edits add these as **mandatory checks** per seat. They close all 15 missed findings and introduce no significant additional risks.

**Recommendation**: Merge these edits into the RFL seat specs before the next council review. Additionally, consider adding a "red team pass" step where one seat specifically reviews for the kinds of findings we missed: vacuous tests, dead assertions, false coverage, and cross-file misconfiguration.

```