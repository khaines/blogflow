# Deferral Gate — BlogFlow RFL

Defines the deferral tracking mechanism for the review-fix-loop. Referenced by section 3.4 of review-fix-loop/SKILL.md.

## SQL Table Schema

```sql
CREATE TABLE IF NOT EXISTS deferrals (
  pr              INTEGER NOT NULL,
  round           INTEGER NOT NULL,
  finding_id      TEXT    NOT NULL,
  severity        TEXT    NOT NULL,
  summary         TEXT    NOT NULL,
  deferral_reason TEXT    NOT NULL,
  source          TEXT    NOT NULL DEFAULT 'session-filed',
  issue_number    INTEGER,
  filed_at        TIMESTAMP,
  PRIMARY KEY (pr, finding_id)
);
```

### Column Notes

- **pr**: Bound to PR_NUMBER from Run Identity (section 1.5 of SKILL.md). Never accept externally passed PR number.
- **source**: 'session-filed' for in-loop classification; 'reconciled-from-github' for rows rebuilt from GitHub on fresh sessions.
- **issue_number**, **filed_at**, **source**: Append-only once set. Subsequent passes may NOT overwrite them.

### Session-Filing SQL

When a finding is classified as Deferred:

```sql
INSERT INTO deferrals (pr, round, finding_id, severity, summary, deferral_reason, source)
VALUES (?, ?, ?, ?, ?, ?, 'session-filed')
ON CONFLICT (pr, finding_id) DO UPDATE SET
  round = excluded.round, severity = excluded.severity,
  summary = excluded.summary, deferral_reason = excluded.deferral_reason;
```

## GitHub Issue Requirements

For every row where issue_number IS NULL — create a GitHub issue with the following:

1. **Run-identity marker** (first line): `<!-- blogflow-deferral pr=N finding_id=X severity=Y -->` (severity LOWERCASE in marker)
2. **Backlink to PR**: "Deferred from PR #N — finding X"
3. **Quoted finding** (per rating-rubric): `**Finding (Severity, Slot Rn):** text`
4. **Deferral rationale**: why it cannot be fixed in this PR + risk assessment
5. **Protected-domain assessment** (on its own line): `Protected-domain assessment: none`
   Allowed values: none only. security, content-integrity, data-integrity, cryptography are NOT allowed — hard error.
6. **Concrete acceptance criteria** so the issue is actionable later.

Required labels: type:tech-debt AND at least one of service:* OR tech-debt AND priority:*.

After creation, verify labels: `gh issue view $N --json labels`

## Enforcement Query (the Gate)

### (a) Session-SQL Check

```sql
SELECT pr, round, finding_id, severity, summary
FROM deferrals WHERE pr = ? AND issue_number IS NULL;
```
Empty result = gate passes. Any row = file missing issues, re-verify.

### (b) GitHub Cross-Check

```bash
gh issue list --state all --search "blogflow-deferral pr=N in:body" \
  --json number,title,body,state --jq '.[] | {number, state, title, body}'
```

Parse marker regex, reconcile OPEN/CLOSED state, verify labels, severity cross-check.

Marker regex (multiline-mode anchored):
patterns: blogflow-deferral pr=(N+) finding_id=(+) severity=(critical|high|medium|low|info)

### Only when BOTH (a) empty AND (b) reconciled does the gate pass.

## Protected Domains — Cannot Be Deferred

| Domain | What it covers |
|--------|---------------|
| Security | Auth bypass, credential exposure, injection, hardcoded secrets |
| Content Integrity | Path traversal, symlink escape in overlayfs, cross-site content leakage |
| Data Integrity | Schema correctness, referential integrity, data loss risk |
| Cryptography | HMAC validation, key management, weak keys |

---

*Source: review-fix-loop/SKILL.md section 3.4. This file exists for composability.*
