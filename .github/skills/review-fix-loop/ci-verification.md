# CI Verification — BlogFlow RFL

Defines the CI status verification mechanism for the review-fix-loop. Referenced by section 6.6 of review-fix-loop/SKILL.md.

## Required Checks (fallback)

When branch-protection is not configured, these checks are required:

| Check | CI Workflow |
|-------|-----------|
| Lint | .github/workflows/ci.yml — golangci-lint |
| Build | .github/workflows/ci.yml — go build ./... |
| Test | .github/workflows/ci.yml — go test -race |
| Docker | .github/workflows/deploy.yml — container build |

Branch-protection API overrides this fallback list when configured.

## Verification Procedure

### Step 1 — Resolve HEAD and Required Checks

```bash
POST_FIX_HEAD=$(git rev-parse HEAD)
# Fetch from branch-protection (primary source)
REQUIRED_FILE=$(mktemp)
if required=$(gh api "repos/{owner}/{repo}/branches/{base_branch}/protection/required_status_checks" \
  --jq '(.contexts // [])[], (.checks // [])[].context' 2>/dev/null); then
  printf '%s\n' "$required" | sort -u > "$REQUIRED_FILE"
  SOURCE=branch-protection
else
  # Fallback
  printf '%s\n' Lint Build Test Docker | sort -u > "$REQUIRED_FILE"
  SOURCE=fallback
fi
```

### Step 2 — Query Status Rollup

```bash
gh pr view {PR_NUMBER} --json statusCheckRollup,headRefOid \
  --jq '{head: .headRefOid, checks: [.statusCheckRollup[] | {
           kind: (.__typename // "unknown"),
           name: (.name // .context),
           conclusion: (.conclusion // .state)}]}'
```

Confirm .head equals POST_FIX_HEAD. Mismatch → wait 10s, re-fetch (at most 5 retries).

### Step 3 — Classify Every Check

- SUCCESS → Passing
- FAILURE, TIMED_OUT, CANCELLED, ACTION_REQUIRED → Failing
- SKIPPED → Invalid for required checks (unless path-filtered)
- Missing from rollup but in required set → Hard failure
- null and IN_PROGRESS/PENDING/QUEUED → In-flight → poll (step 4)

### Step 3a — Path-Filtered Skip

A SKIPPED check may be exempt only if ALL recorded:
- The workflow path + on.pull_request.paths clause
- PR changed-file list (from gh pr view)
- Explicit empty intersection evidence

### Step 4 — In-Flight Polling

Poll every 30s (45 min cap). On timeout → STOP and surface to human.

### Step 5 — Failing Checks

Treat as new actionable findings (severity High). Re-enter section 3.3.

---

*Source: review-fix-loop/SKILL.md section 6.6. This file exists for composability.*
