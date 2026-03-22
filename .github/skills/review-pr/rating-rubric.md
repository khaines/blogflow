# PR Review Rating Rubric

This document defines the scoring system, severity definitions, and consensus algorithm used by the `review-pr` skill to produce objective, repeatable PR ratings. AI review agents parse this rubric to assign findings, calculate scores, and format GitHub PR comments.

---

## Overall Rating Scale

| Rating | Label | Criteria | Action |
|--------|-------|----------|--------|
| ⭐⭐⭐⭐⭐ 5 | **Exceptional** | Zero Critical/High findings. Follows all applicable checklist items. Clean metadata. Well-structured, idiomatic code. Adds value beyond the minimum (good tests, clear docs, thoughtful error handling). | `APPROVE` |
| ⭐⭐⭐⭐ 4 | **Good** | Zero Critical findings. At most 1–2 High findings (minor). Follows most checklist items. Metadata is accurate. Solid code that meets quality standards. | `APPROVE` |
| ⭐⭐⭐ 3 | **Acceptable** | Zero Critical findings. 3–5 High findings. Some checklist gaps. Metadata may need minor updates. Code works but has room for improvement. | `COMMENT` |
| ⭐⭐ 2 | **Needs Work** | Zero Critical findings, but 5+ High findings OR multiple significant checklist violations. Metadata issues. Code has patterns that should be corrected before merge. | `REQUEST_CHANGES` |
| ⭐ 1 | **Significant Issues** | 1+ Critical findings (security vulnerabilities, data loss risk, broken functionality, content integrity breach or unauthorized content access). Major checklist violations. PR should not merge in current state. | `REQUEST_CHANGES` |

---

## Finding Severity Levels

### 🔴 Critical — Must fix before merge. Blocks approval.

- Security vulnerabilities (injection, auth bypass, credential exposure)
- Data loss or corruption risk
- Content integrity breach or unauthorized content access (cross-site content leakage, path traversal exposing other users' content)
- Breaking changes to public APIs without migration path
- Production outage risk (missing error handling on critical paths, resource leaks)

> **Example:** "SQL injection via unsanitized user input in query builder"
>
> **Example:** "Content path not validated — path traversal could expose content from other users' sites"

### 🟠 High — Should fix before merge. Strongly recommended.

- Logic errors that affect correctness but aren't critical
- Missing or inadequate test coverage for new functionality
- Performance issues that would affect user experience (N+1 queries, unbounded loops)
- Missing error handling on important code paths
- Checklist violations for security, testing, or observability

> **Example:** "New endpoint has no unit tests"
>
> **Example:** "Unbounded pagination query could return millions of rows"

### 🟡 Medium — Should address. May be acceptable to defer with justification.

- Code style or pattern inconsistencies with project conventions
- Missing documentation for public APIs or complex logic
- Non-critical checklist gaps
- Suboptimal patterns that work but could be improved

> **Example:** "Function exceeds 100 lines — consider extracting sub-functions"
>
> **Example:** "Missing structured logging context in error path"

### 🔵 Low — Nice to have. Improvement suggestions.

- Minor code style preferences
- Alternative approaches that might be slightly better
- Documentation wording improvements
- Test coverage for edge cases (not primary paths)

> **Example:** "Consider using a table-driven test here for clarity"
>
> **Example:** "This comment could be more specific about the 'why'"

### ℹ️ Info — Observations. No action required.

- Positive callouts (well-designed patterns, good test coverage)
- Context for reviewers (explains why something looks unusual but is correct)
- FYI notes about related areas that might need attention in future PRs

> **Example:** "Good use of context propagation for deadline handling"
>
> **Example:** "This area will need updating when multi-site support lands"

---

## Multi-Model Consensus Scoring

When the `review-pr` skill runs in **multi-model council mode** (4 models reviewing the same PR independently), each finding is tagged with a consensus label derived from how many models flagged the same issue.

### Consensus Thresholds

| Models Agreeing | Consensus Label | Confidence | Weight |
|-----------------|-----------------|------------|--------|
| 4/4 | ✅ **Unanimous** | Very High | 1.0× severity |
| 3/4 | ✅ **High consensus** | High | 1.0× severity |
| 2/4 | ⚠️ **Split** | Moderate | 0.75× severity (may be style preference) |
| 1/4 | ⚠️ **Low consensus** | Low | 0.5× severity (may be false positive) |

### Consensus Rules

1. **Critical safety net** — A Critical finding from ANY single model is always included, regardless of consensus. Safety findings are never suppressed.
2. **High finding threshold** — A High finding needs 2+ models to be reported at High severity. If only 1 model flags it, the finding is downgraded to Medium.
3. **Medium/Low inclusion** — Medium and Low findings need 2+ models to be included in the report. Findings flagged by only 1 model are mentioned under Info.
4. **Unanimous highlighting** — Unanimous findings are highlighted prominently in the report with a ✅ badge.
5. **Severity disagreement** — If models disagree on severity (e.g., one says Critical, another says High), use the **HIGHER** severity but note the disagreement in the finding description.

---

## Rating Calculation Algorithm

### Base Algorithm

```
1. Start at rating 5
2. For each Critical finding:  rating = 1  (immediate — short-circuit)
3. For each High finding:      rating = min(rating, rating - 0.5)  [floor at 2]
4. For each Medium finding:    rating = min(rating, rating - 0.2)  [floor at 3]
5. Low and Info findings do not affect the rating
6. Round to nearest integer
7. Apply consensus weighting in multi-model mode
```

### Special Rules

| Condition | Effect |
|-----------|--------|
| Any Critical finding | Automatic rating **1** |
| 5+ High findings | Rating capped at **2** |
| 3–5 High findings | Rating capped at **3** |
| 1–2 High findings | Rating capped at **4** |
| Perfect score (5) | Requires: zero High+, all relevant checklists pass, metadata clean |

### Consensus Weighting (Multi-Model Mode)

In multi-model mode, apply consensus weights **before** the base algorithm:

1. Multiply each finding's severity impact by its consensus weight (see table above).
2. A Critical finding at 0.5× weight still triggers rating = 1 (Critical safety net applies unconditionally).
3. After weighting, fractional severity impacts below 0.1 are discarded.

---

## GitHub PR Comment Templates

> **Note**: These templates are for the **review body** only. All findings with a file/line reference (Critical through Low) are posted as **inline review comments** on the specific files and lines in the PR. The review body contains the summary, rating, metadata assessment, info-level observations, and recommendation. See Phase 7.2 in SKILL.md.

### APPROVE Template

Used for ratings 4–5. Action: `APPROVE`.

```markdown
## ✅ PR Review — Rating: {rating}/5 ⭐

{summary}

### Findings Summary
{count} findings posted as inline comments ({breakdown by severity}).

### Info
{info_observations — positive callouts and context notes}

### Checklists Passed
{checklist_summary}

**Recommendation**: Approved — {rationale}
```

### REQUEST_CHANGES Template

Used for ratings 1–2. Action: `REQUEST_CHANGES`.

```markdown
## 🔴 PR Review — Rating: {rating}/5

{summary}

### Findings Summary
{count} findings posted as inline comments ({breakdown by severity}).
See inline comments for details on each finding.

### Info
{info_observations}

### Checklists
{checklist_violations}

**Recommendation**: Changes requested — {rationale}
```

### COMMENT Template

Used for rating 3. Action: `COMMENT`.

```markdown
## 🟡 PR Review — Rating: {rating}/5

{summary}

### Findings Summary
{count} findings posted as inline comments ({breakdown by severity}).
See inline comments for details on each finding.

### Info
{info_observations}

### Checklists
{checklist_summary}

**Recommendation**: Looks good with suggested improvements — {rationale}
```

---

## Tie-Breaking Rules

When the calculated rating falls between two integers, apply these rules in priority order:

1. **Round DOWN** if any findings are in the **security**, **content integrity**, or **data integrity** domains. Err on the side of caution for safety-sensitive areas.
2. **Round UP** if all Critical/High findings have **mitigations already in progress** (e.g., linked follow-up issues, partial fixes in the PR, documented workarounds).
3. **Round to nearest** otherwise (standard rounding: 0.5 rounds up).
