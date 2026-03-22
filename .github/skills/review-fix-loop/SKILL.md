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
  "models_flagging": ["opus", "sonnet", "gpt", "gemini"],
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

**Stop the loop** if ANY of these are true:

1. **Target rating achieved** — the current rating meets or exceeds `target_rating` AND there are no remaining Critical or High actionable findings. Proceed to Phase 6.
2. **Max rounds reached** — `current_round >= max_rounds`. Proceed to Phase 6 with a note that the limit was hit.
3. **No progress** — the count of actionable findings has not decreased from the previous round AND no findings were fixed. This prevents infinite loops on unfixable issues. Proceed to Phase 6.

**Below-target with no actionable findings — DO NOT terminate.**

If the rating is below `target_rating` but the round produced zero actionable findings (all findings were dismissed or deferred), apply the following escalation before terminating:

1. **Re-examine all dismissed High+ findings from this round and previous rounds.** For each one, ask: "Is the dismissal rationale genuinely solid, or was this dismissed to avoid work?" Challenge dismissals that cite convenience rather than impossibility.
2. **Re-examine all deferred findings.** Verify each deferral meets the strict Deferral Policy in `dismissal-rules.md`. If any deferral doesn't meet all three criteria, reclassify it as actionable and continue fixing.
3. **If re-examination yields new actionable findings**, continue the loop (do not terminate).
4. **If re-examination confirms all dismissals/deferrals are legitimate**, terminate with a detailed rationale explaining why 5/5 was not achievable in this PR. This rationale must be included in the progression report and PR review comment.

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

### 6.2 Generate Progression Report

Compile the full round-by-round progression:

```markdown
## Review-Fix Loop Report

**PR**: #{number} — {title}
**Branch**: {branch_name}
**Rounds completed**: {count}
**Final rating**: {rating}/5 ⭐ — {label}
**Termination reason**: {Target achieved | Max rounds | No progress | Below target — all dismissals/deferrals verified legitimate}

### Progression

| Round | Rating | 🔴 Critical | 🟠 High | 🟡 Medium | 🔵 Low | ℹ️ Info | Fixed | Dismissed |
|-------|--------|-------------|---------|-----------|--------|---------|-------|-----------|
| R1    | {n}/5  | {c}         | {h}     | {m}       | {l}    | {i}     | —     | {d}       |
| R2    | {n}/5  | {c}         | {h}     | {m}       | {l}    | {i}     | {f}   | {d}       |
| ...   | ...    | ...         | ...     | ...       | ...    | ...     | ...   | ...       |

### Findings Addressed
{For each fixed finding across all rounds:}
- **{finding.id}** ({severity}) — {brief description} → Fixed in R{round}

### Findings Dismissed
{For each dismissed finding:}
- **{finding.id}** ({severity}) — {brief description} → {dismissal_reason}

### Findings Deferred
{For findings noted for follow-up:}
- **{finding.id}** ({severity}) — {brief description} → {deferral_reason}

### Commits
{List of fix commits with SHAs and messages}
```

### 6.3 Post Final Summary

If the PR is on GitHub, post the progression report as a PR comment (not a review — a standalone comment) so it serves as a permanent record of the review-fix process.

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

1. **Full 4-model council RFL** completed (not a "quick verification" or "spot check" — the complete multi-model review pipeline)
2. **5/5⭐ rating** achieved, or below-target with all deferrals verified legitimate and tracked as GitHub issues
3. **§2.1 triage verification** completed for any round with 5+ dismissals or any dismissed findings in protected domains (security, content integrity, data integrity, cryptography)
4. **Dismissed findings audited** — real items identified and tracked as backlog issues with specific GitHub issue numbers
5. **Full progression report posted** as a PR comment containing: round-by-round progression table, all findings addressed, all findings dismissed with rationale, all findings deferred with issue numbers, all backlog issues created, commit SHAs
6. **CI green** on all checks (Build, Lint, Test, Docker) — no exceptions, no "it's just Docker" or "it's a pre-existing failure"

### What is NOT acceptable

- **"Quick verification"** after a rebase — a rebase can introduce merge conflicts that alter code semantics. A full council review is required after any non-trivial rebase.
- **"Already reviewed before rebase"** — prior review results are invalidated by code changes, including conflict resolution during rebase.
- **"CI will catch it"** — CI catches compilation and lint errors. It does not catch security gaps, content integrity failures, or design doc deviations. The RFL council catches those.
- **Merging by the agent** — AI agents do not merge PRs. Ever. The human reviewer merges after reviewing the RFL report.
- **Skipping the dismissed findings audit** — every RFL must include an assessment of whether dismissed findings should be backlog items.

### When to re-run the full RFL

A full 4-model council RFL must be re-run (not just a spot-check) when:

- The PR is rebased with conflict resolution (any file had merge markers)
- New commits are pushed after the RFL report was posted
- The PR base branch changed (retargeted from a scaffold branch to main)
- More than 24 hours have passed since the last full RFL (to catch drift from other merges)
