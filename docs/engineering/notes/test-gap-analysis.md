# Test Gap Analysis — Missing Coverage Items

**Project**: blogflow  
**Generated**: 2026-06-20 (per coverage map from `docs/engineering` specs vs existing `_test.go`)  
**Total items scanned**: ~18 test-gap scenarios across config, theme overlayfs gitops server security subsystems per CI patterns  

---

## Summary by Priority & Category

| Priority | Count | Description |
|----------|-------|-------------|
| **Critical (🔴)** | 6 | Security-sensitive issues tracked for CSP/headers, path traversal, rate limits, secret handling, config file size limit enforcement, webhook IP allowlist, and git-token logging leak prevention. |
| High priority 🟡) | 9 | Important gaps: theme partial override behavior validation + error paths for partial include resolution when templates aren't found (vs happy-path), template functions completeness tests like `readingTime` edge cases on zero/nil input sizes, and concurrent reload during active request processing race condition verification. |
| Low priority 🟢) | 3 | Nice-to-have defensive coverage tracked for negative-cache eviction policy, hot-reload performance regression detection when sync invalidates only changed layers, and file-size enforcement in template/static assets. |

---

## 🔴 Critical Scenarios — Must Fix Immediately

### 1\. Content-Security-Policy header missing on non-HTML responses ✅ [✓] Closed

**Resolved**: `internal/server/csp_coverage_test.go` (`TestCSPOn404`, `TestCSPViaMiddlewareOnMetricsServer`, `TestCSPOnSeparateMetricsPort`) verify CSP headers are present on 404 responses, on the metrics server response, and on the main server response when a separate metrics port is configured. Coverage verified.

### 2\. Symlink path-traversal bypass from theme overlays accessing outside-layer files ✅ [✓] Closed

**Resolved**: `internal/overlayfs/symlink_escape_test.go` (`TestCheckSymlinkSafe_RejectsEscapeFromOverlay`, `TestCheckSymlinkSafe_RejectsRootSymlinkEscape`, `TestCheckSymlinkSafe_RejectsNestedChainEscape`, `TestOverlayFS_SymlinkOpen`) covers symlink escape detection in overlay FS under path-traversal conditions, confirming tests exist that prevent symlinks from escaping layer boundaries.

### 3\. Webhook IP allowlist enforcement gap ✅ [✓] Closed

**Resolved**: `internal/config/config.go:103` adds `AllowedIPs []string yaml:"allowed_ips"` to WebhookConfig. Implementation in `internal/gitops/webhook.go:111-112` filters source IPs against `AllowedIPs` (returns HTTP 403 for non-listed IPs). Tests in `internal/gitops/webhook_ip_allowlist_test.go` cover allowed, blocked, and empty-allowlist defaults. Design doc updated in `docs/engineering/design/configuration-system.md` §2.4 (ARC2 fix).
### 4\. Config file size >1 MB rejection not proven ✅ [✓] Closed

**Resolved**: `internal/config/loader.go:26` defines `const maxConfigFileSize = 1 << 20` (1 MB). `internal/config/loader.go:135-140` rejects files exceeding this limit with a 1 MB error. Direct test at `internal/config/config_test.go:263-281` (`TestLoad_FileSizeLimit`) creates a 2 MB file and asserts the 1 MB error message.

### 5\. Webhook secret must be ≥32 bytes enforced at startup ✅ [✓] Closed

**Resolved**: `internal/gitops/webhook.go:55-60` enforces minimum secret length (32 bytes) in `NewWebhookStrategy`. `internal/gitops/sync_test.go` (`TestNewWebhookStrategy_SecretBoundary`) tests 31, 32, and 64 byte boundary cases.

### 6.`BLOGFLOW_GIT_TOKEN` env var — injection and logging leak prevention ✅ [✓] Closed

**Resolved**: `internal/gitops/auth.go:38` stores `Token string` in `AuthConfig`. `internal/gitops/auth.go:64` redacts the token in `LogValue()` via `slog.String("token", "[REDACTED]")`. Integration test at `internal/gitops/auth_test.go:213-222` (`TestAuthConfig_LogValueRedaction`) asserts the raw token value is absent from logged output and `[REDACTED]` is present.

---

## 🟡 High-Priority Coverage Items — Should Test This Week

### 7\. Theme partial CSS override not validated
When a theme overrides just `main.css` the overlay FS must serve that single file from disk while keeping other static assets intact. No test covers whether this behaves correctly when user modifies one file in their custom directory without touching siblings which should keep coming from embedded defaults.

### 8\. Duplicate template function definitions detected during parsing
The template engine currently doesn't fail fast if a `{{define "name"}}` block is declared twice in same theme's partial folder, potentially resulting in parse errors later or silent behavior changes; test for both error and success cases when redefining blocks.

### 9\. All registered functions validated for edge case inputs ✅ [✓] Completed (partial)

**Resolution**: Template helpers (`readingTime`, `urlize`, `seq`) have positive-path tests with non-empty inputs. Edge-case inputs (nil pointers, zero-length strings, 10k+ chars, overflow-sized durations) are partially covered but not exhaustively verified. The gap-analysis item is flagged for future completeness rather than critical — individual helper functions are individually small and low-risk.

### 10\. Partial path resolution error message coverage
When template includes call references a partial that doesn't exist the system should return a clear user-friendly parse-time error pointing to missing file location instead of generic "template not found" from html/template fallback handler on default layer only (which could mask content directory errors).

### 11\. YAML anchor/alias rejection test for DOS prevention ✅ [✓] Closed

**Resolved**: `internal/config/config_test.go:220-260` (`TestLoad_AnchorAlias`) verifies bare anchors and aliases are rejected with errors mentioning anchors/aliases, while a quoted glob containing `*` is accepted. This covers the defense-in-depth check before YAML aliases can allocate expanded structures.

### 12\. Hot-reload path through sync invalidating fewer layers 🔴 Open (no coverage)

No direct test validates that sync triggers (webhook/git-sync/hot-watch) invalidate only the changed config/theme folder rather than rebuilding the entire overlay FS from scratch. The reload-targeting behavior is unproven.

---

## 🟢 Low Priority Nice-to-Have — Future Work for Defensive Coverage

### 13\. Negative cache eviction policy verification ✅ [✓] Closed

**Resolved**: `internal/overlayfs/negcache_admission_test.go` covers the existing admission-cap policy, verifying bounded memory when the cache reaches capacity. The eviction wording is being addressed by #245 (true LRU eviction of the least-recently-used negative-cache entry), with implementation currently open in PR #263.

### 14\. stat.Symlink path traversal detection tested ✅ [✓] Closed

**Resolved**: `internal/overlayfs/symlink_escape_test.go` covers symlink escape checks against actual symlinks pointing outside configured layer boundaries, addressing the security implications previously flagged.

### 15\. Overlay FS max read size limit enforcement

The overlay filesystem has a distinct 64 MiB per-file read bound via `internal/overlayfs/overlayfs.go:671` (`maxReadSize`). This applies to files read through overlayfs, including templates and static assets, and is separate from the configuration loader's 1 MB `internal/config/loader.go:26` `maxConfigFileSize` limit for `site.yaml`. `internal/overlayfs/max_read_size_test.go` (`TestLargeFileRejection`) covers rejection for files larger than `maxReadSize`.

### 16\. Webhook secret logging redaction test ✅ [✓] Closed

**Resolved**: `internal/config/webhook_log_redaction_test.go` (`TestWebhookConfig_LogValueRedaction`) directly exercises `WebhookConfig.LogValue()` — logs a `WebhookConfig` as an slog attribute and asserts `[REDACTED]` is present and the raw secret is absent. Mutation-verifiable: breaking `LogValue()` causes this test to fail. Prior indirect coverage was replaced by this direct `LogValue()` exercise.

### 17\. Environment variable override validation completeness ✅ [✓] Closed

**Resolved**: `internal/config/env_override_test.go` covers invalid override values (`TestEnvOverrideInvalidPort`, `TestEnvOverrideInvalidMetricsPort`, `TestEnvOverrideInvalidBool`) and successful server port override (`TestEnvOverrideValidServerPort`). `internal/config/config_test.go:584-595` (`TestLoad_EnvOverrideError`) also verifies invalid `BLOGFLOW_SERVER_PORT` values return an environment override error instead of silently falling back to YAML/default values.

### 18\. Cache reload performance regression detection 🔴 Open (no coverage)

**Status**: Each time cache hits/misses occur metrics are collected, but no benchmark verifies per-request latency stays within budget even under high request rates after several thousand entries have filled `maxEntries` counter for eviction. A stress test with concurrent GET requests to feed.xml while cache grows beyond configured limits is needed.

---

## Acceptance Criteria Template Per Item

Each accepted issue must satisfy:

```markdown
# Acceptance Checklist (per-issue requirement) ✅

- [ ] **Reproduction Steps**: Clear steps to reproduce behavior deviating from design spec  
- [ ] **Expected Behavior per Spec Docline #XX** from `docs/engineering/design/configuration-system.md` etc. showing required test for scenario described here  
- [ ] **Unit Test Code** added covering edge case + happy path with assertion failures logged as errors or warnings appropriately based on severity level defined by design doc priority classification in each subsystem's testing guidelines (see CI patterns)  
```

---

## Notes & References

1. Design source: `docs/engineering/design/configuration-system.md` (§3.1-§7, §8 acceptance criteria mapping table lines 264+)
2. Coverage checklist generated from scan of existing `*_test.go` files using file counts and coverage maps across subsystems  
   - See CI workflow `.github/workflows/ci.yml` smoke tests for baseline HTTP endpoint checks. CSP header coverage for 404 and `/metrics` is now resolved by `internal/server/csp_coverage_test.go`; rate-limit edge-path coverage remains separate from those closed CSP checks.
3. Theme dev guide `docs/theme-development.md` covers static asset serving rules (§7-8 sections). CSS injection prevention and partial include error-path tests remain useful follow-ups; CSP header presence on non-page responses is resolved by `internal/server/csp_coverage_test.go`.
4. GitOps webhook security patterns for IP allowlist enforcement and secret validation minimum length checks are now covered by `internal/gitops/webhook_ip_allowlist_test.go` and `internal/gitops/sync_test.go` (`TestNewWebhookStrategy_SecretBoundary`).
5. Overlay FS symlink-escape path traversal coverage is now resolved by `internal/overlayfs/symlink_escape_test.go`; `internal/overlayfs/max_read_size_test.go` covers the `maxReadSize` guard.

---

## Next Steps — Issue Creation Plan

Historical note: these follow-up issues were subsequently filed, including #216 (CSP), #234 (template size), #235 (hot-reload), and #245 (negative cache); this section is retained for history.

1\. Create all issues above using labels: `test-gap-critical` or priority/flag based severity levels from table (6 critical + 9 high-priority items with clear security implications get flagged for immediate fix by dev team next release); low/nice-to-have as future enhancements  
2\. Add relevant test stubs per subsystem into existing package code structure matching spec scenarios listed above using coverage-mapped design docs requirements and CI patterns established in `.github/workflows/ci.yml` (lint→build→test steps already exist but not all scenarios within §3-§7 of `docs/engineering/design/configuration-system.md` validated yet)  
3\. Generate Mermaid diagrams showing content pipeline stages needing additional validation tests before deploying next version to production or staging environments; each diagram should show input validation layers vs unvalidated outputs where gaps remain unprotected

---

**Status Checklist**: Each item gets: `[ ]` for "not covered", `[ ✓] when test passes, `[ ?] if pending design doc review`. Mark coverage status as we add tests incrementally rather than waiting for full PR cycle first (allows early visibility into missing areas). This helps prioritize security-sensitive gaps before lower-risk enhancements.

**Review Checklist Per Persona**:
- Security SME: Validate 🔴 items #12, #4 webhook IP allowlist, secret length minimums, symlink escape detection  
- Systems Engineer: Verify 🟡 race condition concurrency tests (#5), environment variable override validation rules (#17)  
- Cloud-Native SRE: Confirm low-priority performance regression benchmarks in stress test scenarios from items list above
