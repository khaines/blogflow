---
name: review-fix-loop
description: >-
  Orchestrates an iterative review-fix cycle on a pull request. Invokes the review-pr skill,
  evaluates findings, dispatches appropriate agent personas to fix actionable items, commits
  and pushes fixes, then re-reviews. Repeats until no new actionable findings are discovered
  or the safety-valve maximum round limit is reached. Produces a progression report showing
  round-by-round improvement.
---

# Review-Fix Loop Skill — Orchestration Instructions

This skill automates the iterative cycle of reviewing a pull request, fixing discovered issues, and re-reviewing until the PR reaches a clean state. It wraps the existing `review-pr` skill as its evaluation engine and dispatches specialist agent personas to perform fixes.

Read all supporting files before beginning:

- `.github/skills/review-pr/SKILL.md` — the review engine (Phases 1–8)
- `.github/skills/review-pr/agent-map.md` — file → agent mapping
- `.github/skills/review-pr/checklist-map.md` — file → checklist mapping
- `.github/skills/review-pr/rating-rubric.md` — severity and rating definitions
- `.github/skills/review-fix-loop/dismissal-rules.md` — when to dismiss vs. fix findings

---

## Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `max_rounds` | 5 | Maximum review-fix iterations before stopping |
| `target_rating` | 5 | Minimum acceptable rating. Loop will not terminate early below this unless max_rounds is hit or no progress is possible. See §3.3. |
| `auto_dismiss_low_consensus` | true | Auto-dismiss 1/4 consensus findings on pre-existing code |
| `auto_dismiss_out_of_diff` | true | Auto-dismiss findings referencing code not in the PR diff |

---

## Phase 1: Initialization

### 1.1 Determine PR Context

Identify the target pull request using the same detection logic as review-pr Phase 1.1:

- **PR number provided explicitly**: Fetch via GitHub MCP tools.
- **Current branch has an open PR**: Detect from branch name.
- **No PR exists**: Abort — this skill requires a PR to push fixes to.

### 1.2 Checkout the PR Branch

Ensure you are on the PR's source branch and it is up to date:

```
git checkout {branch_name}
git pull origin {branch_name}
```

### 1.3 Initialize Progression Tracker

Create a tracking structure for round-by-round results:

```
Round | Rating | Critical | High | Medium | Low | Info | Actionable Fixed | Dismissed | Status
```

This tracker is updated after each round and used to generate the final report.

### 1.4 Verify PR Scope Against Linked Issues

Before the first review round, verify that the PR's changes are scoped correctly to its linked issues. This prevents PRs from silently growing beyond their intended scope.

1. **Collect linked issues** — read the PR description for `Closes #N`, `Fixes #N`, `Resolves #N` references. Also check the PR's linked issues via GitHub MCP tools.
2. **Read each linked issue** — fetch the issue title, description, and acceptance criteria via `issue_read`.
3. **Build the expected scope** — from the linked issues, determine what files, components, and behaviors should be changed.
4. **Compare against actual changes** — review the PR's changed file list against the expected scope:

| Check | Finding |
|---|---|
| **Files changed outside expected scope** | Flag files that don't relate to any linked issue. Severity: **Medium** if tangential (e.g., unrelated refactor), **High** if in a completely different domain. |
| **Missing expected changes** | If a linked issue expects changes to a component but no files in that component are modified, flag as **Medium** — the PR may not fully resolve the issue. |
| **No linked issues** | Flag as **High** — every PR should reference at least one issue for traceability. |
| **Scope significantly exceeds issues** | If the PR touches 3× or more files than the issues suggest, flag as **Medium** with a recommendation to split the PR. |

5. **Record scope findings** — add any scope violations to the findings list with the tag `[Scope]` in the finding description. These findings are included in the first review round's results.

**Important**: Scope verification is about traceability and preventing scope creep. It is NOT about blocking legitimate changes that are tightly coupled to the issue (e.g., updating a test file alongside the implementation, or fixing a typo noticed while working). Use judgment — the goal is to catch "this PR also refactors the theme engine" situations, not "this PR also added a missing import."

### 1.5 Capture Run Identity (MANDATORY)

Before entering the review loop, capture and persist a Run Identity tuple. These values are consumed by later phases — never re-derive from outside.

```bash
PR_NUMBER=$(gh pr view --json number --jq '.number')
PR_BRANCH=$(git rev-parse --abbrev-ref HEAD)
PR_URL=$(gh pr view --json url --jq '.url')
INITIAL_HEAD_SHA=$(git rev-parse HEAD)
LOOP_START_UTC=$(date -u +%FT%T%Z)
```

- `PR_NUMBER` — resolves against current branch (never accepts externally passed PR number)
- `INITIAL_HEAD_SHA` — HEAD at loop start; used for halt marker queries and deviation handling
- `LOOP_START_UTC` — loop-start timestamp in ISO-8601 UTC; uses in gate checks as epoch

### 1.6 Halt-Marker Check (MANDATORY)

Before entering the review loop, query the PR for any open halt markers from a prior RFL invocation:

```bash
gh pr view {PR_NUMBER} --json comments \
  --jq '.comments[] \
    | select(.body | contains("<!-- blogflow-rfl-halt pr={PR_NUMBER} ")) \
    | {url: .url, createdAt: .createdAt, body_preview: (.body[0:300])}'
```

For each halt marker returned, check whether a matching closure comment exists:

```bash
gh pr view {PR_NUMBER} --json comments \
  --jq '.comments[] \
    | select(.body | contains("<!-- blogflow-rfl-halt-resolved pr={PR_NUMBER} " \
        + "halt_head=<halt_head_from_marker> "))'
```

If **any** halt marker has no matching closure comment, **STOP** — do NOT proceed past Phase 1. Surface a hard error listing each unresolved halt marker (URL, halt_head, reason). The human must either (a) reply with explicit acknowledgment and direct the next RFL to post the closure comment, or (b) re-run the failing CI checks and confirm green. Only after every open halt marker has a closure comment may the orchestrator enter Phase 2.

This prevents re-entry into RFL when prior CI failures remain unresolved.

---

## Phase 2: Review (Invoke review-pr)

### 2.1 Execute the Review

Run the full `review-pr` skill pipeline (Phases 1–7) against the current state of the PR branch. Use the same complexity classification, agent selection, review mode (single or multi-model council), and checklist evaluation.

**Important**: Skip review-pr Phase 8 (GitHub Feedback) on ALL intermediate rounds. GitHub feedback is posted exclusively by this skill's Phase 6 (Final Report) after the loop terminates.

### 2.2 Collect Structured Findings

Capture every finding from the review in structured format:

```json
{
  "id": "R{round}-F{number}",
  "severity": "critical | high | medium | low | info",
  "file": "path/to/file.ext",
  "line": 42,
  "finding": "Description of the issue",
  "recommendation": "Suggested fix",
  "consensus": "4/4 | 3/4 | 2/4 | 1/4",
  "reviewers_flagging": ["Systems", "Security", "Architect", "SRE"],
  "in_pr_diff": true | false
}
```

### 2.3 Record Round Results

Update the progression tracker with the round's rating, finding counts by severity, and review mode used.

---

## Phase 3: Evaluate Findings

### 3.1 Load Dismissal Rules

Read `.github/skills/review-fix-loop/dismissal-rules.md` for the complete dismissal logic.

### 3.2 Categorize Each Finding

For every finding, apply the dismissal rules in priority order to classify it as one of:

| Category | Action | Criteria |
|----------|--------|----------|
| **Actionable** | Fix in this round | In-diff, 2+ model consensus (or Critical), clear recommendation |
| **Dismissed — Out of scope** | Skip | Finding references code not changed in this PR |
| **Dismissed — Pre-existing** | Skip | Issue exists in base branch, not introduced by this PR |
| **Dismissed — Low consensus** | Skip | 1/4 consensus on non-Critical finding |
| **Dismissed — Design choice** | Skip | Finding challenges an intentional design decision documented in ADR or PR description |
| **Deferred** | Track for follow-up; create tracking issue | Meets ALL Deferral Policy criteria in `dismissal-rules.md`: confirmed valid, impossible to fix in this PR, not security/content-integrity/data-integrity. Requires tracking issue and risk assessment. |

### 3.3 Check Termination Conditions

**The ONLY acceptable successful termination is a full 5/5 rating from every council slot with zero actionable findings.** There is no "good enough" carve-out. Sub-5 ratings from any slot block merge regardless of finding count or deferral legitimacy.

**Stop the loop** if ANY of these are true:

1. **Target rating achieved** — **every** council slot returns a `5/5` rating AND there are zero actionable findings at any severity (Critical, High, Medium, Low). INFO-level findings do not block termination. Proceed to §3.4.
2. **Max rounds reached** — `current_round >= max_rounds`. Proceed to §3.4. A max-rounds termination with any actionable findings remaining is a **failed RFL** — the PR is NOT merge-ready.
3. **No progress** — the count of actionable findings has not decreased from the previous round AND no findings were fixed. Proceed to §3.4. Treated as a failed RFL.

**Sub-5/5 with no actionable findings — DO NOT terminate.**

If any reviewer returned a rating below 5 — even with zero actionable findings — the loop MUST continue. Treat the gap as actionable:

1. **Re-prompt the dissenting reviewer** with a single question: "What specific, concrete change to this PR would raise your rating from {N}/5 to 5/5? Reply with actionable line-level findings, or explicitly state 'no change needed; rating revised to 5/5'."
2. **If the reviewer produces actionable findings**, add them to the current round's finding set (round counter does NOT advance), then proceed to §3.4.
3. **If the reviewer revises to 5/5**, capture that as the official R{n}-revised rating and re-check termination conditions.
4. **If the reviewer cannot articulate a concrete fix and refuses to revise to 5/5**, that is a protocol failure — do NOT terminate. Surface the impasse to the human reviewer. Proceed to §3.4 before surfacing.**

If none of the termination conditions are met, proceed to §3.4.

### 3.4 Deferral Filing Checkpoint (MANDATORY)

Every finding classified as **Deferred** in §3.2 MUST have a GitHub issue filed and recorded in the `deferrals` table **before** proceeding to Phase 4, advancing to Phase 6, or posting any convergence report. A deferral without an issue number is not valid — it remains Actionable.

#### 3.4.1 Deferrals Table (session SQL)

On first use, create the table:

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

`source` provenance: `'session-filed'` for in-loop classification; `'reconciled-from-github'` for rows rebuilt from GitHub on fresh sessions. `issue_number`, `filed_at`, and `source` MUST be append-only.

When a finding is classified as Deferred:

```sql
INSERT INTO deferrals (pr, round, finding_id, severity, summary, deferral_reason, source)
VALUES (?, ?, ?, ?, ?, ?, 'session-filed')
ON CONFLICT (pr, finding_id) DO UPDATE SET
  round = excluded.round, severity = excluded.severity,
  summary = excluded.summary, deferral_reason = excluded.deferral_reason;
```

#### 3.4.2 Filing the Issue

For every row where `issue_number IS NULL`, create a GitHub issue via `gh issue create` with:

1. **Run-identity marker** (first line):
   ```html
   <!-- blogflow-deferral pr={PR_NUMBER} finding_id={finding_id} severity={severity} -->
   ```
   (severity LOWERCASE in marker)

2. **Backlink to PR**: "Deferred from PR #{PR_NUMBER} — finding {finding.id}"

3. **Quoted finding** (per rating-rubric Finding Body Format Contract):
   ```
   **Finding ({Severity}, {ReviewerSlot} {Round}):** {finding text}
   ```

4. **Deferral rationale**: why it cannot be fixed in this PR, and risk assessment.

5. **Protected-domain assessment** (on its own line):
   ```
   Protected-domain assessment: none
   ```
   (`security`, `content-integrity`, `data-integrity`, `cryptography` are NOT allowed — hard error)

6. **Concrete acceptance criteria** so the issue is actionable later.

Required labels: `type:tech-debt`, at least one of `service:*` OR `tech-debt`, and `priority:*`. After creation, verify labels via `gh issue view $N --json labels`. Missing labels → apply and re-verify.

```sql
UPDATE deferrals SET issue_number = ?, filed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE pr = ? AND finding_id = ? AND issue_number IS NULL;
```

#### 3.4.3 Enforcement Query (the gate)

Before advancing to Phase 4, re-dispatching council, or posting a convergence report, run BOTH:

**(a) Session-SQL check**:
```sql
SELECT pr, round, finding_id, severity, summary
FROM deferrals WHERE pr = ? AND issue_number IS NULL;
```
If any row returned, file missing issues and re-run. Only empty result set is accepted.

**(b) GitHub-issue cross-check**:
```bash
gh issue list --state all --search "blogflow-deferral pr=${PR_NUMBER} in:body" \
  --json number,title,body,state --jq '.[] | {number, state, title, body}'
```
For each issue: parse marker regex, reconcile OPEN/CLOSED state, verify protected-domain = `none`, verify labels present, verify severity cross-check. INSERT reconciliation rows with `source = 'reconciled-from-github'` for any OPEN issues not in local `deferrals` table.

**Only when both (a) returns empty AND (b) is reconciled does the gate permit progression.**

**§3.4 runs on EVERY branch out of §3.3 — non-negotiable. There is no path from §3.3 to either Phase 4 or Phase 6 that bypasses §3.4.**

If none of the termination conditions are met, proceed to Phase 4.

---

## Phase 4: Fix Actionable Findings

### 4.1 Group Findings by Agent

Using the agent-map from `review-pr`, group actionable findings by the agent persona best suited to fix them:

1. Match each finding's `file` path against the agent-map patterns.
2. Group findings by their matched agent.
3. If a finding matches multiple agents, assign it to the **primary agent** for that file (highest priority match).

### 4.2 Dispatch Agent Fixes

For each agent group, dispatch the appropriate specialist agent to fix the findings. Use the `task` tool with the agent's corresponding `agent_type` (from `.claude/agents/` or custom agents).

**Dispatch rules:**

- **Independent agent groups run in parallel.** If findings span multiple agents with no file overlap, dispatch all agents simultaneously.
- **Overlapping files serialize.** If two agents need to edit the same file, dispatch them sequentially to avoid conflicts.
- **Provide complete context** to each agent:
  - The specific findings assigned to them (ID, severity, file, line, finding, recommendation)
  - The relevant file content (not just the diff — agents need surrounding context to make correct fixes)
  - The applicable checklist items that were violated
  - Instructions to make minimal, surgical fixes that address the findings without introducing new issues

**Agent dispatch prompt template:**

```
You are fixing PR review findings for BlogFlow. You are acting as the {agent_name} specialist.

## Findings to Address

{for each finding}
### {finding.id} — {finding.severity}
- **File**: {finding.file}:{finding.line}
- **Finding**: {finding.finding}
- **Recommendation**: {finding.recommendation}
- **Consensus**: {finding.consensus}
{end for}

## Instructions

1. Read each file that needs changes.
2. Make precise, surgical fixes that address each finding.
3. Do NOT modify code unrelated to the findings.
4. Do NOT introduce new patterns, refactors, or improvements beyond what the findings require.
5. Verify your changes don't break surrounding code.
6. After fixing, briefly confirm which findings were addressed and how.
```

### 4.3 Validate Fixes

After all agents complete:

1. **Check for conflicts** — ensure no file was modified by two agents in incompatible ways.
2. **Verify files are syntactically valid** — run any available linters or formatters for the affected file types.
3. **Spot-check critical fixes** — for Critical and High severity fixes, read the changed code to verify correctness.

---

## Phase 5: Commit, Push & Loop

### 5.1 Stage and Commit

Stage all fixed files and commit with a descriptive message:

```
git add {fixed_files}
git commit -m "Review fixes (Round {N}): Address {count} findings

Fixed:
- {finding.id}: {brief description} ({file})
- ...

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

### 5.2 Push

Push the fixes to the PR branch:

```
git push origin {branch_name}
```

### 5.3 Resolve PR Comments

If the review-pr skill posted inline comments on the PR for findings that have now been fixed:

- Use GitHub MCP tools to check for existing review threads on the PR.
- For each fixed finding, if a matching inline comment exists, post a reply indicating what was fixed and resolve the thread.

### 5.4 Loop Back

Return to **Phase 2** for the next review round. Increment the round counter.

---

## Phase 6: Final Report

### 6.1 Execute Final Review

Regardless of termination reason, run one final review-pr pass (including Phase 8 — GitHub Feedback) to post the conclusive review to the PR. If the loop exited due to `max_rounds` or `no_progress`, include a note in the review body explaining why the loop stopped and listing any actionable findings that remain unresolved.

Immediately after the final review pass completes, capture:

```bash
FINAL_HEAD_SHA=$(git rev-parse HEAD)    # after all fix commits pushed
FINAL_ROUND_COUNT={total_counted_rounds}
```

These populate the run-identity marker and are consumed by §6.4 and §6.6.

### 6.2 Generate Progression Report

Compile the full round-by-round progression:

```markdown
## RFL Report for PR #{PR_NUMBER}

<!-- blogflow-rfl-report pr={PR_NUMBER} head={FINAL_HEAD_SHA} rounds={FINAL_ROUND_COUNT} -->

**PR**: #{PR_NUMBER} — {title}
**Branch**: {PR_BRANCH}
**HEAD SHA**: `{FINAL_HEAD_SHA}`
**Rounds completed**: {FINAL_ROUND_COUNT}
**Final rating**: {rating}/5 ⭐ — {label}
**Termination reason**: {Target achieved (5/5 from all slots) | Max rounds (FAILED RFL) | No progress (FAILED RFL)}

### Progression

| Round | Rating | 🔴 Critical | 🟠 High | 🟡 Medium | 🔵 Low | ℹ️ Info | Fixed | Dismissed |
|-------|--------|-------------|---------|-----------|--------|---------|-------|-----------|
| R1    | {n}/5  | {c}         | {h}     | {m}       | {l}    | {i}     | —     | {d}       |
| R2    | {n}/5  | {c}         | {h}     | {m}       | {l}    | {i}     | {f}   | {d}       |
| ...   | ...    | ...         | ...     | ...       | ...    | ...     | ...   | ...       |

### Council Composition Audit

Per round, verified `(persona, model)` pairs for each slot:

| Round | Slot | Agent Persona | Model (session) | Dispatch HEAD |
|-------|------|---------------|-----------------|---------------|
| R1 | Systems | `cloud-native-systems-engineer` | `{session_model}` | `{sha}` |
| R1 | Security | `cloud-native-security-sme` | `{session_model}` | `{sha}` |
| R1 | Architect | `cloud-native-distributed-systems-architect` | `{session_model}` | `{sha}` |
| R1 | SRE | `cloud-native-site-reliability-engineer` | `{session_model}` | `{sha}` |
### Findings Addressed
{For each fixed finding across all rounds:}
- **{finding.id}** ({severity}) — {brief description} → Fixed in R{round}

### Findings Dismissed
{For each dismissed finding:}
- **{finding.id}** ({severity}) — {brief description} → {dismissal_reason}

### Findings Deferred (from deferrals table joined to issue_number)
- **{finding.id}** ({severity}) — {brief description} → #{issue_number} [{source}] ({deferral_reason})
- Every row has non-NULL `issue_number` (§3.4.3 gate). `source`: `session-filed` or `reconciled-from-github`.

### Commits
{List of fix commits with SHAs and messages}
```

### 6.3 Post Final Summary

If the PR is on GitHub, post the progression report as a PR comment (not a review — a standalone comment) so it serves as a permanent record of the review-fix process.

### 6.4 Verify the Final Report Was Posted (MANDATORY)

**The orchestrator MUST not claim the loop terminated successfully until it has verified, on GitHub, that the progression report comment exists on the PR.**

Verification procedure:
1. **Use Run Identity from §1.5.** Query:
   ```bash
   gh pr view {PR_NUMBER} --json comments \
     --jq '.comments[] \
       | select(.body | contains("\u003c!-- blogflow-rfl-report pr={PR_NUMBER} head={FINAL_HEAD_SHA} rounds={FINAL_ROUND_COUNT} \")) \
       | {url: .url, createdAt: .createdAt, body_preview: (.body[0:300])}'
   ```
2. **Validate the matched comment body.** It MUST contain:
   - `## RFL Report` as a heading
   - The progression table header `| Round | Rating |`
   - The substring `**Final rating**:` 
   - `createdAt` newer than `LOOP_START_UTC` from §1.5
   - At least 800 characters in length
   If any check fails, treat as missing. Post with retry cap of 2 re-post attempts (3 total). On double failure: HARD ERROR — do NOT declare terminated.
3. **Capture the verified comment URL** and report as: `RFL Report: <url>`
4. **Only after the comment URL is verified** may the orchestrator proceed to §6.5.

### 6.5 Council Composition Audit (MANDATORY)

Before declaring the loop terminated, audit the council composition record:

1. **Enumerate every counted round** in the progression report.
2. **For each round, locate the per-slot composition row** in the `### Council Composition Audit` table.
3. **Verify each row**:
   - The persona must match the protocol table from review-pr Phase 3 (§3.1).
   - The model must equal the session model in every slot.
   Any deviation on either field makes the round INVALID.
4. **Missing composition data** → treat the round as INVALID. Dispatch a full 4-persön council at current HEAD as a new counted corrective round (label `R{n}-corrective`).
5. **Uncorrected deviation** → dispatch a corrective round at current HEAD. Annotate as `R{n}-superseded`. The corrective replaces the deviant in convergence math.
6. **Only after every counted round passes** may the loop proceed to §6.6.

### 6.6 CI Status Verification (MANDATORY)

Local build/test/lint output is NOT sufficient evidence of CI green. Before declaring the loop terminated, the orchestrator MUST poll the GitHub status check rollup.

Verification procedure:
1. **Resolve post-fix HEAD and required-check set.**
   ```bash
   POST_FIX_HEAD=$(git rev-parse HEAD)
   REQUIRED_CHECKS=$(gh api "repos/{owner}/{repo}/branches/{base_branch}/protection/required_status_checks" \
     --jq '(.contexts // [])[], (.checks // [])[].context' 2>/dev/null || printf 'Lint\nBuild\nTest\nDocker')
   ```
2. **Query the status check rollup** at post-fix HEAD:
   ```bash
   gh pr view {PR_NUMBER} --json statusCheckRollup,headRefOid \
     --jq '{head: .headRefOid, checks: [.statusCheckRollup[] | {
              kind: (.__typename // "unknown"),
              name: (.name // .context),
              conclusion: (.conclusion // .state)}]}'
   ```
   Confirm `.head` equals `POST_FIX_HEAD`. If mismatched, wait 10s and re-fetch (<= 5 retries).
3. **Classify every check:**
   - `SUCCESS` → ✅ passing
   - `FAILURE | TIMED_OUT | CANCELLED | ACTION_REQUIRED` → ❌ failing
   - `SKIPPED` → ⚠️ invalid for required checks (unless path-filtered, see step 3a)
   - Missing but required → ❌ **hard failure**
4. **In-flight checks:** poll every 30s (45 min cap). On timeout: STOP and surface to human with `gh run list --branch {PR_BRANCH} --limit 10`.
5. **Failing checks:** treat as new actionable findings (severity High). Re-enter §3.3 → §3.4 → Phase 4.
6. **Only after every required check is SUCCESS** may the loop be declared terminated.

Post a `### CI Status Verification` section in the progression report listing verified HEAD, every check conclusion, and any rerun or path-filter-skip rationale.

---

## Important Notes

- **The review-pr skill is the evaluation engine.** This skill orchestrates the loop; review-pr does the actual reviewing. Do not duplicate review logic — always invoke review-pr.
- **Fixes must be minimal and surgical.** Agent fix dispatches should address specific findings, not refactor or improve beyond what was flagged. Scope creep in fixes triggers new findings in the next round, creating infinite loops.
- **Dismissal discipline prevents churn.** The dismissal rules exist to prevent the loop from chasing pre-existing issues, subjective preferences, or out-of-scope problems. Apply them consistently.
- **The safety valve is non-negotiable.** Never exceed `max_rounds`. If the loop hasn't converged after 5 rounds, something is wrong — either findings are being introduced by fixes, or dismissal rules need adjustment.
- **Intermediate rounds skip GitHub feedback.** Posting review comments on every round would create noise. Only the final round posts to GitHub — all intermediate rounds skip Phase 8.
- **Track everything.** The progression report is the primary output of this skill. It should tell the complete story of what was reviewed, what was fixed, what was dismissed, and why.

---

## Merge Policy — Mandatory Quality Gate

**AI agents MUST NOT merge pull requests.** Merging is a human-only action. The agent's responsibility ends at delivering a PR that meets all quality criteria. The human reviewer merges after inspecting the RFL report.

### Pre-Merge Checklist (ALL required before a PR is merge-ready)

Every PR must have ALL of the following before it can be considered merge-ready:

1. **Full council RFL** — all 4 specialized reviewers completed (Systems → Security → Architect → SRE), each using the session model with its own persona
2. **5/5⭐ rating from ALL council slots** with zero actionable findings (strict — no sub-5 acceptable, no carve-outs)
3. **Council composition verified** — every round's `(agent_type, model)` pairs match the review-pr protocol table (§3.1)
4. **§2.1 triage verification** completed for any round with 5+ dismissals or any dismissed findings in protected domains (security, content integrity, data integrity, cryptography)
5. **All deferred findings** tracked as GitHub issues with `blogflow-deferral` markers, required labels, and `protected-domain assessment: none`
6. **Full progression report posted** as PR comment with: council composition audit table, all findings addressed/dismissed/deferred with issue numbers, commit SHAs
7. **Report post verified** — run-identity marker query (§6.4) confirms comment exists; verified URL reported
8. **CI green** on all required checks (fetched from branch-protection via §6.6) — no exceptions, no "it's just Docker" or "it's a pre-existing failure"
9. **No open halt markers** on the PR at termination time

### What is NOT acceptable

- **"Quick verification"** after a rebase — a rebase can introduce merge conflicts that alter code semantics. A full council review is required after any non-trivial rebase.
- **"Already reviewed before rebase"** — prior review results are invalidated by code changes, including conflict resolution during rebase.
- **"CI will catch it"** — CI catches compilation and lint errors. It does not catch security gaps, content integrity failures, or design doc deviations. The RFL council catches those.
- **Merging by the agent** — AI agents do not merge PRs. Ever. The human reviewer merges after reviewing the RFL report.
- **Skipping the dismissed findings audit** — every RFL must include an assessment of whether dismissed findings should be backlog items.

### When to re-run the full RFL

A full council RFL (all 4 specialized reviewers) must be re-run (not just a spot-check) when:

- The PR is rebased with conflict resolution (any file had merge markers)
- New commits are pushed after the RFL report was posted
- The PR base branch changed (retargeted from a scaffold branch to main)
- More than 24 hours have passed since the last full RFL (to catch drift from other merges)
