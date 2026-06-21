# PR Review Rating Rubric

Defines the scoring system, severity definitions, and consensus algorithm used by BlogFlow's `review-pr` skill.

---

## Overall Rating Scale

| Rating | Label | Criteria | Action |
|--------|-------|----------|--------|
| ⭐⭐⭐⭐⭐ 5/5 | **Exceptional** | Zero Critical/High findings. Follows all applicable checklists. Clean metadata. Well-structured code. | `APPROVE` |
| ⭐⭐⭐⭐ 4/5 | **Good** | Zero Critical findings. 0–2 High findings. Follows most checklists. Solid code. | `APPROVE` |
| ⭐⭐⭐ 3/5 | **Acceptable** | Zero Critical findings. 3–5 High findings. Some checklist gaps. Code works but needs improvement. | `COMMENT` |
| ⭐⭐ 2/5 | **Needs Work** | Zero Critical findings, but 5+ High findings OR multiple significant checklist violations. | `REQUEST_CHANGES` |
| ⭐ 1/5 | **Significant Issues** | 1+ Critical findings (security vulnerabilities, data loss risk, broken functionality). | `REQUEST_CHANGES` |

---

## Finding Severity

### 🔴 Critical — Must fix before merge. Blocks approval.
- Security vulnerabilities (injection, auth bypass, credential exposure)
- Data loss or corruption risk
- Tenant isolation breach
- Breaking changes to public APIs without migration path
- Production outage risk (missing error handling on critical paths, resource leaks)
- *Example: "Path traversal not blocked — symlink in theme layer escapes overlay root"*
- *Example: "CSP header missing on /metrics endpoint — script injection possible"*

### 🟠 High — Should fix before merge. Strongly recommended.
- Logic errors affecting correctness but not Critical
- Missing or inadequate test coverage for new functionality
- Performance issues that would affect user experience
- Missing error handling on important code paths
- Checklist violations for security, testing, or observability
- *Example: "Webhook handler missing IP allowlist enforcement"*
- *Example: "No negative case tests for new validation in config loader"*

### 🟡 Medium — Should address. May be acceptable to defer with justification.
- Code style or pattern inconsistencies with project conventions
- Missing documentation for public APIs or complex logic
- Non-critical checklist gaps
- Suboptimal patterns that work but could be improved
- *Example: "Function exceeds 80 lines — consider extracting sub-functions"*
- *Example: "Missing structured logging in error path"*

### 🔵 Low — Nice to have. Improvement suggestions.
- Minor code style preferences
- Alternative approaches that might be slightly better
- Documentation wording improvements
- Test coverage for edge cases (not primary paths)
- *Example: "Consider table-driven test here for clarity"*
- *Example: "This comment could be more specific about the 'why'"*

### ℹ️ Info — Observations. No action required.
- Positive callouts (well-designed patterns, good test coverage)
- Context for reviewers (explains why something looks unusual but is correct)
- FYI notes about related areas | *Example: "Good use of context propagation for deadline handling"*

---

## Finding Body Format Contract

When a reviewer emits a finding into a PR comment or council report, the quoted-finding line MUST use:

```
**Finding ({Severity}, {ReviewerSlot} {Round}):** <finding text>
```

- `{Severity}` — `Critical | High | Medium | Low | Info` (Title-Case)
- `{ReviewerSlot}` — agent persona name (e.g. `Security`, `Systems Engineer`, `SRE`)
- `{Round}` — `R{N}` where N is the 1-based RFL round index

Example:
`**Finding (High, Security R3):** Webhook handler does not enforce ip_allowlist from config. All IPs accepted when filter is enabled.`

---

## Multi-Model Consensus Scoring

When the skill runs in multi-model council mode (4+ models reviewing independently), each finding is tagged with a consensus label.

### Consensus Thresholds

| Models Agreeing | Consensus | Weight |
|-----------------|-----------|--------|
| 4/4 | ✅ **Unanimous** | 1.0× severity |
| 3/4 | ✅ **High consensus** | 1.0× severity |
| 2/4 | ⚠️ **Split** | 0.75× severity |
| 1/4 | ⚠️ **Low consensus** | 0.5× severity |

### Consensus Rules
1. **Critical safety net** — A Critical finding from ANY single model is always included
2. **High finding threshold** — A High finding needs 2+ models to be reported at High severity
3. **Medium/Low inclusion** — Medium and Low findings need 2+ models for inclusion in report
4. **Unanimous highlighting** — Unanimous findings are highlighted prominently

---

## Rating Calculation Algorithm

```
1. Start at rating 5
2. For each Critical finding:   rating = 1 (immediate — short-circuit)
3. For each High finding:       rating = min(rating, rating - 0.5)
4. For each Medium finding:     rating = min(rating, rating - 0.2)
5. Low and Info findings do not affect rating
6. Round to nearest integer
7. Apply consensus weighting before steps 2–5 in multi-model mode
```


