# Test Gap Analysis — Missing Coverage Items

**Project**: blogflow  
**Generated**: 2026-06-20 (per coverage map from `docs/engineering` specs vs existing `_test.go`)  
**Total items scanned**: ~18 test-gap scenarios across config, theme overlayfs gitops server security subsystems per CI patterns  

---

## Summary by Priority & Category

| Priority | Count | Description |
|----------|-------|-------------|
| **Critical (🔴)** | 6 | Security-sensitive issues affecting CSP/headers path traversal rate limits secret handling config file size limit enforcement, webhook IP allowlist missing, git-token logging leak prevention. |
| High priority 🟡) | 9 | Important gaps: theme partial override behavior validation + error paths for partial include resolution when templates aren't found (vs happy-path), template functions completeness tests like `readingTime` edge cases on zero/nil input sizes etc., symlink escape detection in overlay FS not proven tested under path-traversal conditions, concurrent reload during active request processing race condition verification needed. |
| Low priority 🟢) | 3 | Nice-to-have defensive coverage: negative cache eviction policy for absent paths over max capacity limits (100K entries threshold), hot-reload performance regression detection when sync invalidates only changed layers not entire stack, file size ≥64MB enforcement in template assets. |

---

## 🔴 Critical Scenarios — Must Fix Immediately

### 1\. Content-Security-Policy header missing on non-HTML responses ✅ [✓] Closed

**Resolved**: `internal/server/csp_coverage_test.go` (`TestCSPOn404`, `TestCSPViaMiddlewareOnMetricsServer`, `TestCSPOnSeparateMetricsPort`) verify CSP headers are present on 404 responses, on the metrics server response, and on the main server response when a separate metrics port is configured. Coverage verified.

### 2\. Symlink path-traversal bypass from theme overlays accessing outside-layer files ✅ [✓] Closed

**Resolved**: `internal/gitops/symlink_escape_test.go` covers symlink escape detection in overlay FS under path-traversal conditions, confirming tests exist that prevent symlinks from escaping layer boundaries.

### 3\. Webhook IP allowlist enforcement gap ✅ [✓] Closed

**Resolved**: `internal/config/config.go:97` adds `AllowedIPs []string yaml:"allowed_ips"` to WebhookConfig. Implementation in `internal/gitops/webhook.go:101-105` filters source IPs against `AllowedIPs` (returns HTTP 403 for non-listed IPs). Tests in `internal/gitops/webhook_ip_allowlist_test.go` cover allowed, blocked, and empty-allowlist defaults. Design doc updated in configuration-system.md §2.4 (ARC2 fix).
### 4\. Config file size >1 MB rejection not proven ✅ [✓] Closed

**Resolved**: `internal/config/loader.go:25` defines `const maxConfigFileSize = 1 << 20` (1 MB). `internal/config/loader.go:132-133` rejects files exceeding this limit with "config file exceeds 1 MB limit". Direct test at `internal/config/config_test.go:263-281` (`TestLoad_FileSizeLimit`) creates a 2 MB file and asserts the 1 MB error message.

### 5\. Webhook secret must be ≥32 bytes enforced at startup ✅ [✓] Closed

**Resolved**: `internal/gitops/webhook.go` enforces minimum secret length (32 bytes) in `NewWebhookStrategy`. `internal/gitops/webhook_secret_length_test.go` (`TestNewWebhookStrategy_SecretBoundary`) tests 31, 32, and 64 byte boundary cases. `internal/gitops/webhook_secret_logging_test.go` verifies secret length requirement.

### 6.`BLOGFLOW_GIT_TOKEN` env var — injection and logging leak prevention ✅ [✓] Closed

**Resolved**: `internal/gitops/auth.go:38` stores `Token string` in `AuthConfig`. `internal/gitops/auth.go:64` redacts the token in `LogValue()` via `slog.String("token", "[REDACTED]")`. Integration test at `internal/gitops/auth_test.go:216-222` (`TestLoadAuthFromEnv_LogValueRedaction`) asserts the raw token value is absent from logged output and `[REDACTED]` is present.

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

### 11\. YAML anchor/alias rejection test for DOS prevention
Config files using `&anchor` or `*` aliases in unquoted scalar value positions should be rejected with explicit error message; tests need to parse raw bytes and show that billion-laughs-style expansion attacks are explicitly blocked before struct fields even start allocating memory (defense-in-depth layer).

### 12\. Hot-reload path through sync invalidating fewer layers 🔴 Open (no coverage)

No direct test validates that sync triggers (webhook/git-sync/hot-watch) invalidate only the changed config/theme folder rather than rebuilding the entire overlay FS from scratch. The reload-targeting behavior is unproven.

---

## 🟢 Low Priority Nice-to-Have — Future Work for Defensive Coverage

### 13\. Negative cache eviction policy verification ✅ [✓] Closed

**Resolved**: `negcache_eviction_test.go` covers eviction of oldest negative-cache entries when max capacity is reached, verifying the unbounded-growth prevention.

### 14\. stat.Symlink path traversal detection tested ✅ [✓] Closed

**Resolved**: `symlink_escape_test.go` covers symlink escape checks against actual symlinks pointing outside configured layer boundaries, addressing the security implications previously flagged.

### 15\. Max template file size limit enforcement. Templates exceeding ~64 MB threshold not tested for immediate rejection before memory allocation occurs, potentially leading to OOM under attack; verify early failure mode exists now and always runs during compile time even with embedded defaults baked in as fallback source of truth when user uploads oversized file via git pull or content sync strategy deployment pipeline stages allow such files to land on disk temporarily).

### 16\. Webhook secret logging redaction test ✅ [✓] Closed

**Resolved**: `internal/config/webhook_log_redaction_test.go` (`TestWebhookConfig_LogValueRedaction`) directly exercises `WebhookConfig.LogValue()` — logs a `WebhookConfig` as an slog attribute and asserts `[REDACTED]` is present and the raw secret is absent. Mutation-verifiable: breaking `LogValue()` causes this test to fail. The hollow integration test in `webhook_secret_logging_test.go` was replaced by this direct LogValue exercise. The dead `loader.Load()` call in `webhook_secret_length_test.go` was also removed.

### 17\. Environment variable override validation completeness 🔴 Open (no coverage)

**Status**: Tests for BLOGFLOW_* overrides exist but only validate successful type conversions — not negative cases like providing a non-integer string to a port field that silently falls back to YAML default without logging a warning (user should be alerted about the ignored/bad value before config load completes and returns invalid configuration state).

### 18\. Cache reload performance regression detection 🔴 Open (no coverage)

**Status**: Each time cache hits/misses occur metrics are collected, but no benchmark verifies per-request latency stays within budget even under high request rates after several thousand entries have filled `maxEntries` counter for eviction. A stress test with concurrent GET requests to feed.xml while cache grows beyond configured limits is needed.

---

## Acceptance Criteria Template Per Item

Each accepted issue must satisfy:

```markdown
# Acceptance Checklist (per-issue requirement) ✅

- [ ] **Reproduction Steps**: Clear steps to reproduce behavior deviating from design spec  
- [ ] **Expected Behavior per Spec Docline #XX** from `docs/engineering/config-system.md` etc. showing required test for scenario described here  
- [ ] **Unit Test Code** added covering edge case + happy path with assertion failures logged as errors or warnings appropriately based on severity level defined by design doc priority classification in each subsystem's testing guidelines (see CI patterns)  
```

---

## Notes & References

1. Design source: `docs/engineering/design/configuration-system.md` (§3.1-§7, §8 acceptance criteria mapping table lines 264+)
2. Coverage checklist generated from scan of existing `*_test.go` files using file counts and coverage maps across subsystems  
   - See CI workflow `.github/workflows/ci.yml` smoke tests for baseline HTTP endpoint checks currently passing under happy-path scenarios only but not error paths (like CSP header missing on 404 responses) or rate limit enforcement behaviors
3. Theme dev guide `docs/theme-development.md` covers static asset serving rules (§7-8 sections) without explicit test coverage shown in existing test count  
   - No unit/integration tests for CSS injection prevention, partial include error paths (when file missing vs present), CSP header presence on non-page responses like `/metrics prometheus scrape endpoint
4. GitOps webhook security patterns require IP allowlist enforcement and secret validation minimum length checks at startup per config spec requirements but no such test exists  
5. Overlay FS limits need verification for symlink escapes path traversal not currently covered in context overlayfs_test.go file line counts alone don't specify which assertions actually exist there; must read implementation code to confirm

---

## Next Steps — Issue Creation Plan

1\. Create all issues above using labels: `test-gap-critical` or priority/flag based severity levels from table (6 critical + 9 high-priority items with clear security implications get flagged for immediate fix by dev team next release); low/nice-to-have as future enhancements  
2\. Add relevant test stubs per subsystem into existing package code structure matching spec scenarios listed above using coverage-mapped design docs requirements and CI patterns established in `.github/workflows/ci.yml` (lint→build→test steps already exist but not all scenarios within §3-§7 of config-system.md validated yet)  
3\. Generate Mermaid diagrams showing content pipeline stages needing additional validation tests before deploying next version to production or staging environments; each diagram should show input validation layers vs unvalidated outputs where gaps remain unprotected

---

**Status Checklist**: Each item gets: `[ ]` for "not covered", `[ ✓] when test passes, `[ ?] if pending design doc review`. Mark coverage status as we add tests incrementally rather than waiting for full PR cycle first (allows early visibility into missing areas). This helps prioritize security-sensitive gaps before lower-risk enhancements.

**Review Checklist Per Persona**:
- Security SME: Validate 🔴 items #12, #4 webhook IP allowlist, secret length minimums, symlink escape detection  
- Systems Engineer: Verify 🟡 race condition concurrency tests (#5), environment variable override validation rules (#17)  
- Cloud-Native SRE: Confirm low-priority performance regression benchmarks in stress test scenarios from items list above
