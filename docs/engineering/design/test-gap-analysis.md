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

### 1\. Content-Security-Policy header missing on non-HTML responses
| Field | Value |
|-------|-------|
| **Title** | "CSP header missing on non-HTML endpoints (404s, /metrics)" |
| Category | `server/middleware` CSP enforcement completeness. 404/REST paths don't include same policy headers as HTML pages; could be exploited for XSS attacks if browser ignores meta-CSP fallback only when HTTP CSP is absent elsewhere in deployment configs where needed by some edge server setups omitting header injection (e.g., reverse proxy misconfigurations). |
| **Priority** | 🔴 critical-security-critical. 404 errors still serve raw HTML without same policy enforcement, allowing script injection into broken pages if user controls content paths via URL parameter manipulation in config directory structure. |

### 2\. Symlink path-traversal bypass from theme overlays accessing outside-layer files
| Field | Value |
|-------|-------|
| Overlay FS must validate all symlinks don't escape to read/write beyond configured layer boundaries; currently tests cover valid overlay resolution but not symlink escapes through `stat.Symlink` paths (especially when malicious user uploads a link in theme's `/static/images/` pointing outside root). |

### 3\. Webhook IP allowlist enforcement gap
### 3\. Webhook IP allowlist enforcement gap ✅ [✓] Closed
|~Field~|~Value~|
|~~When ip_allowlist flag is present the webhook handler should reject all non-listed IPs with HTTP status code; currently rate limiting tests exist but no test validates that invalid source IPs are rejected when filtering enabled, making possible brute-force signature attacks from anywhere. 4\. Webhook secret must be ≥32 bytes enforced at startup (HMAC-SHA256 key length requirement).~~|

**Resolved**: `internal/config/config.go:97` adds `AllowedIPs []string yaml:"allowed_ips"` to WebhookConfig. Implementation in `internal/gitops/webhook.go:101-105` filters source IPs against `AllowedIPs` (returns HTTP 403 for non-listed IPs). Tests in `internal/gitops/webhook_ip_allowlist_test.go` cover allowed, blocked, and empty-allowlist defaults. Design doc updated in configuration-system.md §2.4 (ARC2 fix).
### 4\. Config file size >1 MB rejection not proven ✅ [✓] Closed
|~Field~|~Value~|
|~~Test coverage exists for oversized config but no direct assertion rejects config files exceeding ~1MB with a clear resource-exhaustion prevention (YAML parsing loop can consume memory if payload unbounded).~~|

**Resolved**: `internal/config/loader.go:25` defines `const maxConfigFileSize = 1 << 20` (1 MB). `internal/config/loader.go:132-133` rejects files exceeding this limit with "config file exceeds 1 MB limit". Direct test at `internal/config/config_test.go:263-281` (`TestLoad_FileSizeLimit`) creates a 2 MB file and asserts the 1 MB error message.

### 6.`BLOGFLOW_GIT_TOKEN` env var — injection and logging leak prevention ✅ [✓] Closed

**Resolved**: `internal/gitops/auth.go:38` stores `Token string` in `AuthConfig`. `internal/gitops/auth.go:64` redacts the token in `LogValue()` via `slog.String("token", "[REDACTED]")`. Integration test at `internal/gitops/auth_test.go:216-222` (`TestLoadAuthFromEnv_LogValueRedaction`) asserts the raw token value is absent from logged output and `[REDACTED]` is present.

---

## 🟡 High-Priority Coverage Items — Should Test This Week

### 7\. Theme partial CSS override not validated
When a theme overrides just `main.css` the overlay FS must serve that single file from disk while keeping other static assets intact. No test covers whether this behaves correctly when user modifies one file in their custom directory without touching siblings which should keep coming from embedded defaults.

### 8\. Duplicate template function definitions detected during parsing
The template engine currently doesn't fail fast if a `{{define "name"}}` block is declared twice in same theme's partial folder, potentially resulting in parse errors later or silent behavior changes; test for both error and success cases when redefining blocks.

### 9\. All registered functions validated for edge case inputs
Template helpers like `readingTime`, `urlize`, `seq`, etc only tested with positive integers non-empty strings; must verify all of these handle nil pointers, zero-length strings, very long text (10k+ chars), overflow-sized durations beyond Go parse limits.

### 10\. Partial path resolution error message coverage
When template includes call references a partial that doesn't exist the system should return a clear user-friendly parse-time error pointing to missing file location instead of generic "template not found" from html/template fallback handler on default layer only (which could mask content directory errors).

### 11\. YAML anchor/alias rejection test for DOS prevention
Config files using `&anchor` or `*` aliases in unquoted scalar value positions should be rejected with explicit error message; tests need to parse raw bytes and show that billion-laughs-style expansion attacks are explicitly blocked before struct fields even start allocating memory (defense-in-depth layer).

### 12\. Hot-reload path through sync invalidating fewer layers
Webhook/git-sync/hot-watch triggers should invalidate only the changed config theme folder rather than rebuilding entire overlay FS from scratch each time; test shows current reload times match full rebuilds instead of targeted incremental updates.

---

## 🟢 Low Priority Nice-to-Have — Future Work for Defensive Coverage

### 13\. Negative cache eviction policy verification
OverlayFS maintains a list of ~100,000 absent paths in negative-cache mode with no test verifies oldest entries actually get removed when limit reached (prevents unbounded growth under heavy directory scanning loads).

### 14\.\`stat.Symlink path traversal detection not tested for security implications. No automated tests covering how the symlink escape check works against actual symlinks pointing outside configured layer boundaries or to external network shares; could allow access escalation if attacker places a bad link in content repo expecting config system to use it (though 2-layer overlay should prevent this anyway).

### 15\. Max template file size limit enforcement. Templates exceeding ~64 MB threshold not tested for immediate rejection before memory allocation occurs, potentially leading to OOM under attack; verify early failure mode exists now and always runs during compile time even with embedded defaults baked in as fallback source of truth when user uploads oversized file via git pull or content sync strategy deployment pipeline stages allow such files to land on disk temporarily).

### 16\. Webhook secret logging redaction test. The config loader already marks secret fields for log masking but no integration tests confirm that structured logs from webhook handlers also strip tokens before writing, especially when custom middleware outputs request/response details containing signature values as headers or body data streams (need end-to-end check across all possible code paths where secrets cross boundaries).

### 17\. Environment variable override validation completeness. Tests for BLOGFLOW_* overrides exist but only validate successful type conversions—not negative cases like providing non-integer string to port number field that silently falls back to YAML default without logging a warning (user should be alerted about ignored/bad value before config load completes and returns invalid configuration state).

### 18\. Cache reload performance regression detection. Each time cache hits/misses occur metrics are collected with no benchmark verifying the per-request latency stays within budget even under high request rates after several thousand entries have filled maxEntries counter for eviction; need stress test where concurrent GET requests hammer feed.xml endpoint while cache grows beyond configured limits to confirm response times don't degrade due to memory pressure or GC pauses.

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