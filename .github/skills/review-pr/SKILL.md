---
name: review-pr
description: >-
  Orchestrates world-class pull request reviews using specialist agent personas and engineering checklists.
  Use this when asked to review a PR, review code changes, or assess PR quality.
  Supports single-model review for simple changes and council mode (4 specialized agents, same model)
  for complex changes. Posts findings as GitHub PR comments for remote PRs.
---

# PR Review Skill — Orchestration Instructions

Execute the following 8-phase pipeline to produce a thorough, actionable pull request review. Read all supporting files from this skill directory before beginning.

---

## Phase 1: PR Analysis & Triage

### 1.1 Determine Review Context

Detect whether a GitHub PR is available:

- **PR number provided explicitly**: Use GitHub MCP tools (`pull_request_read` with method `get`) to fetch the PR title, description, diff, files changed, and linked issues or work items.
- **Current branch has an open PR**: Run `git branch --show-current` to get the branch name, then use `list_pull_requests` filtered by `head` to find an open PR for that branch. If found, fetch its details as above.
- **No PR exists (local-only changes)**: Use `git diff main...HEAD` (or the appropriate base branch) to collect the changes. Note that **Phase 8 (GitHub Feedback) will be skipped** for local-only reviews.

### 1.2 Collect File List and Diff

- For GitHub PRs: use `pull_request_read` with method `get_files` to get the full file list, and method `get_diff` to get the complete diff.
- For local changes: run `git diff --name-only main...HEAD` for the file list and `git diff main...HEAD` for the full diff.

### 1.3 Classify Complexity

Evaluate the change against the following criteria. If **ANY single criterion** falls into the "Complex" column, classify the entire review as **Complex** and use multi-model council mode.

| Criteria | Simple | Complex |
|---|---|---|
| Files changed | 1–5 | 6+ |
| Domains touched | 1 | 2+ |
| Cross-cutting concerns | None | Auth, content integrity, infra |
| ADR / architecture changes | No | Yes |
| New component | No | Yes |
| Schema / API changes | No | Yes |

To determine "domains touched," group the changed files by their top-level directory or functional area (e.g., `internal/server/`, `internal/theme/`, `docs/`, `deploy/`). Each distinct area counts as one domain.

### 1.4 Identify Governing Design Document(s)

Before proceeding to agent selection or review dispatch, determine whether a design document governs this change. This must happen early because the design document content is included in the review input package for all models.

1. Read the PR description and linked issues — look for references to design documents (e.g., "per design doc 004", "implements #92", `docs/engineering/design/` paths)
2. Search `docs/engineering/design/` for documents matching the component or package being changed
3. If the PR itself **is** a design document (changed files are in `docs/engineering/design/`), note this — Phase 5 will be skipped, but the standard checklist evaluation in Phase 4 still applies (including any documentation or style checklists)

Record the result for use in Phase 3 (council input) and Phase 5 (conformance check):
- **Design document(s) found**: List paths and document numbers
- **No design document**: Note the reason (design doc PR, pure refactor, no doc exists)

### 1.5 Output Triage Summary

Before proceeding, output a triage block:

```
### Triage Summary
- **Files changed**: {count}
- **Domains touched**: {list of domains}
- **Complexity classification**: Simple | Complex
- **Triggering criteria**: {which criteria triggered Complex, if applicable}
- **Review mode**: Single Model | Multi-Model Council
- **Design document(s)**: {list of governing design docs, or "None applicable — {reason}"}
```

---

## Phase 2: Agent Persona Selection

### 2.1 Load Agent Mapping

Read the agent mapping file at `.github/skills/review-pr/agent-map.md`. This file maps file patterns and paths to the appropriate specialist agent personas.

### 2.2 Match Files to Agents

For each changed file, determine which agent persona(s) are relevant based on the file patterns and paths defined in the agent map. The 10 available agents (defined in `.github/agents/`) are:

- `cloud-native-distributed-systems-architect`
- `cloud-native-front-end-engineer`
- `cloud-native-security-sme`
- `cloud-native-site-reliability-engineer`
- `cloud-native-systems-engineer`
- `privacy-compliance-grc-lead`
- `product-manager`
- `program-manager`
- `solutions-engineer-developer-success-architect`
- `technical-writer`

### 2.3 Determine Primary and Secondary Agents

- **Primary agent**: The agent whose domain covers the most changed files or the most critical domain in the change.
- **Secondary agents**: All other agents whose domains are touched by the change.

### 2.4 Apply Selection Rules

- If only **1 domain** is touched, use only the primary agent. Do not invoke secondary agents.
- If **multiple domains** are touched, each domain gets its own agent review. Dispatch each agent's review as a separate task so domain-specific expertise is applied independently.
- **Fallback**: If no agent matches any changed file, use `technical-writer` as the default reviewer for Markdown/documentation files, or apply a general code-quality review using the base coding conventions checklist (03) with no specific persona. Never proceed with zero agents — always assign at least one reviewer.

---

## Phase 3: Review Mode Decision

Based on the complexity classification from Phase 1, execute one of the following modes.

### Simple Change — Single-Model Review

Use the default model. Run the review through the primary agent persona's lens:

1. Apply the primary agent's system prompt and domain expertise to the diff.
2. Load and apply the relevant checklists (see Phase 4).
3. Produce findings in the structured format: `{severity, file, line, finding, recommendation}`.

### Complex Change — Multi-Model Council

Dispatch **4 sequential reviews**, all running on the **same model** as the session. Each review uses a different **specialized agent persona** (from agent-map.md). The specialization — not model diversity — drives blind-spot coverage.

All 4 calls **must** be made sequentially (one at a time), not in parallel. Sequential execution produces deterministic, order-independent results that the orchestrator can merge reliably. Parallel dispatch introduces timing sensitivity and divergent outputs that are harder to reconcile.

| Slot | Agent Persona | Review Focus |
|---|---|---|
| **Systems** | `cloud-native-systems-engineer` | Go code quality, HTTP handlers, config loading, content pipeline |
| **Security** | `cloud-native-security-sme` | HMAC validation, path traversal, secrets handling, content integrity |
| **Architect** | `cloud-native-distributed-systems-architect` | System architecture, overlay FS design, content pipeline topology |
| **SRE** | `cloud-native-site-reliability-engineer` | Container health, webhook reliability, cache SLOs, CI/CD |

All slots run on the **same model** (the session's model). The **personas drive blind-spot coverage** — different agent personas see different code paths, apply different checklists, and evaluate against different criteria.

### 3.1 Council Composition Verification (MANDATORY)

After dispatching the 4 reviewers and **before** reporting any "unanimous N/N" result, the orchestrator MUST verify that each slot used the correct **agent persona** and **the same model** as the session. Using the wrong persona defeats blind-spot coverage; using a different model wastes a valuable slot.

#### Protocol Table

Each counted round must have exactly these four `(slot, persona, model)` tuples:

| Slot | Agent Persona | Model (must match session) |
|---|---|---|
| Systems | `cloud-native-systems-engineer` | **= session_model** |
| Security | `cloud-native-security-sme` | **= session_model** |
| Architect | `cloud-native-distributed-systems-architect` | **= session_model** |
| SRE | `cloud-native-site-reliability-engineer` | **= session_model** |

**No slots may deviate.** All four personas MUST be present exactly once. The model MUST match the session's model exactly in every slot. If even one uses the wrong persona OR the wrong model, the entire round is INVALID.

#### Why This Matters

The specialization — not model diversity — drives blind-spot coverage. Different agent personas see different code paths, apply different checklists, and evaluate against different criteria. Running all slots on the same model means findings diverge ONLY because of agent expertise, not because of model biases. This is the right signal for consensus scoring.

If a slot was dispatched with the wrong persona or wrong model, the round is INVALID. The slot must be re-dispatched with the correct persona AND the session model. At most ONE corrective re-dispatch is allowed. If it fails again, STOP and surface a hard error.

#### Dispatch Order

Reviewers fire **sequentially** — Systems → Security → Architect → SRE. Each sees the same PR diff, agent persona instructions, applicable checklists, and design document (if any).

If a reviewer fails, times out, or is skipped, the remaining slots may still produce valid results; note the degradation in the composition record.

#### Capture Verified Composition

Record per slot: slot name, persona name verbatim, model verbatim (must equal session model), dispatch HEAD SHA (`git rev-parse HEAD`), and dispatch timestamp. This is consumed by the Council Composition Audit table in the §6.2 progression report.

---

Each reviewer receives the same input package:

- The full PR diff
- The agent persona instructions (from the matched agent's system prompt in `.github/agents/`)
- The applicable checklists (loaded in Phase 4)
- The governing design document(s), if any (identified in Phase 1.4) — each reviewer must cross-check the implementation against the spec
- Instructions to return findings in this structured format:

```json
{
  "severity": "critical | high | medium | low | info",
  "file": "path/to/file.ext",
  "line": 42,
  "finding": "Description of the issue found",
  "recommendation": "Suggested fix or improvement"
}
```

If a model **fails or times out**, proceed with the remaining models and note the gap in the final report. Do not retry or block on a single model failure.

---

## Phase 4: Checklist-Based Quality Review

### 4.1 Load Checklist Mapping

Read the checklist mapping file at `.github/skills/review-pr/checklist-map.md`. This file maps file patterns, change types, and domains to specific engineering checklists.

### 4.2 Match Files to Checklists

For each changed file, determine which checklists apply based on the checklist map. A single file may trigger multiple checklists (e.g., a Go handler file might trigger both the "Go code quality" checklist and the "API design" checklist if it defines endpoints).

### 4.3 Load Checklists

Load each applicable checklist from `docs/engineering/checklists/`. Read the full checklist content so every item can be evaluated.

### 4.4 Evaluate Changes Against Checklists

Walk through each checklist item and evaluate whether the PR's changes comply:

- **Pass**: The change satisfies the checklist item or the item is not applicable to this change.
- **Fail**: The change violates the checklist item. Create a finding with the appropriate severity.
- **Indeterminate**: Cannot tell from the diff alone. Flag as an `info`-level finding suggesting manual verification.

Record every checklist violation as a finding in the standard format: `{severity, file, line, finding, recommendation}`.

---

## Phase 5: Design Document Conformance

Implementation PRs **must** be cross-checked against their governing design document. A design document is the source of truth for what the implementation should do — checklist compliance alone is insufficient.

### 5.1 Determine if a Design Document Applies

A design document applies when **any** of the following are true:

- The PR description or linked issues reference a design document (e.g., "per design doc 004", "implements #92")
- The changed files implement a component that has a design document in `docs/engineering/design/`
- The PR title uses a `feat:` or `fix:` prefix targeting a component covered by a design document

**Skip this phase** if:
- The PR **is** a design document (the changed files are in `docs/engineering/design/`)
- No design document exists for the component being changed (flag as an `info` finding: "No design document found for this component")
- The PR is a pure refactor, dependency update, or CI/tooling change with no functional behavior changes

### 5.2 Locate the Design Document

1. Check linked issues — read the issue body for design document references
2. Search `docs/engineering/design/` for documents matching the component name or functional area
3. Check the PR description for explicit design document references

If multiple design documents are relevant (e.g., a component implementation references both its own design doc and the shared libraries design doc), load all of them.

### 5.3 Read and Cross-Check

Load the full design document(s). For each of the following areas, compare the implementation against the spec:

| Area | What to Check |
|---|---|
| **API surface** | Do the function signatures, types, and interfaces match the design doc's specification? Are all specified public APIs implemented? |
| **Behavior** | Does the implementation follow the behavioral contracts described in the design doc? (e.g., loading hierarchy, error handling, retry logic) |
| **Configuration** | Are all configuration options from the design doc supported? Is the configuration loading mechanism correct? |
| **Dependencies** | Does the implementation use the libraries and frameworks specified in the design doc? |
| **Data flow** | Does data flow through the system as the design doc describes? Are all pipeline stages present? |
| **Security controls** | Are the security measures from the design doc's security section implemented? |
| **Observability** | Are the metrics, traces, and logs from the design doc's observability section emitted? |
| **Test scenarios** | Do the tests cover the functional test scenarios listed in the design doc? |

### 5.4 Generate Conformance Findings

For each deviation from the design document:

- **Missing feature/behavior** specified in the design doc → **High** severity finding
- **Partial implementation** that covers some but not all of a design doc requirement → **Medium** severity finding
- **Behavioral divergence** where the implementation works differently than specified → **High** severity finding
- **API signature mismatch** between design doc and implementation → **Medium** severity finding

Format each finding with explicit reference to the design document section:

```json
{
  "severity": "high",
  "file": "internal/server/config/config.go",
  "line": 1,
  "finding": "Design doc 004 §2.3 specifies config loading from YAML files and secret files with a 4-layer hierarchy (defaults → YAML → secrets → env vars). Implementation only supports env vars.",
  "recommendation": "Add YAML config file loading and Kubernetes secret file loading per the design doc specification."
}
```

### 5.5 Include in Report

Add a **Design Doc Conformance** section to the final report (Phase 7):

```markdown
### Design Doc Conformance
- Design document(s): {list of design docs checked, or "None applicable — {reason}"}
- Conformance: ✅ Full | ⚠️ Partial ({N} deviations) | ❌ Significant gaps ({N} deviations)
```

---

## Phase 6: PR Metadata Verification

If this is a **local-only review** (no GitHub PR), skip title and description checks. Instead, verify commit messages are meaningful and properly formatted, then proceed to Phase 7.

For GitHub PRs, check the following:

### 6.1 PR Title

- Does the title accurately describe the scope of the change?
- Does it follow conventional commit format or the project's title conventions?
- Is it concise but informative? Flag titles that are too vague (e.g., "Fix stuff") or misleading.

### 6.2 PR Description

- Does the description explain **what** changed and **why**?
- Is the level of detail proportional to the scope? A large refactor with a one-line description is a problem. A typo fix with a 500-word essay is also a problem.
- Are there screenshots, examples, or test instructions where appropriate?

### 6.3 Work Item Reference

- Is there a linked issue, ticket, or work item (e.g., `Closes #123`, `Fixes JIRA-456`)?
- Does the referenced item match the actual scope of the change?
- Flag if the reference is **missing** (no linked work item at all) or **mismatched** (the linked item describes something different from what the PR actually does).

---

## Phase 7: Rating & Consensus Report

### 7.1 Load Rating Rubric

Read the rating rubric from `.github/skills/review-pr/rating-rubric.md`. This defines the 1–5 star scale and the criteria for each rating level.

### 7.2 Calculate Overall Rating

Apply the rubric criteria to the collected findings to determine the overall rating (1–5 stars).

### 7.3 Compile and Order Findings

Sort all findings by severity in this order: **Critical → High → Medium → Low → Info**.

### 7.4 Apply Consensus Scoring (Multi-Model Council Only)

If the review used multi-model council mode:

- For each unique finding, count how many of the 4 reviewers flagged it.
- Display consensus as a fraction (e.g., "4/4", "3/4", "2/4", "1/4").
- Findings flagged by **3 or more models** → tag with **"✅ High consensus"**.
- Findings flagged by **only 1 model** → tag with **"⚠️ Low consensus"**.
- Findings flagged by **2 models** → no special tag (moderate consensus).
- Higher consensus means higher confidence. Weight high-consensus findings more heavily when determining the overall rating.
- Deduplicate findings that are semantically identical across models. Merge them into a single finding with the consensus count.

### 7.5 Generate Final Report

Produce the report in this exact structure:

```markdown
## PR Review Report

**PR**: #{number} — {title}
**Rating**: {1-5} ⭐ — {rating_label}
**Review Mode**: {Single Model | Multi-Model Council}
**Models Used**: {list of models used}
**Agents Applied**: {list of agent personas used}
**Checklists Applied**: {list of checklists evaluated}

### Summary
{2-3 sentence summary of the overall quality and key concerns. Be specific — reference the most important findings.}

### Findings

#### 🔴 Critical ({count})
{Each finding with: file, line, description, recommendation. Include consensus indicator if multi-model.}

#### 🟠 High ({count})
{findings}

#### 🟡 Medium ({count})
{findings}

#### 🔵 Low ({count})
{findings}

#### ℹ️ Info ({count})
{findings}

### PR Metadata
- Title: ✅/❌ {assessment}
- Description: ✅/❌ {assessment}
- Work Item: ✅/❌ {assessment}

### Design Doc Conformance
- Design document(s): {list of design docs checked, or "None applicable — {reason}"}
- Conformance: ✅ Full | ⚠️ Partial ({N} deviations) | ❌ Significant gaps ({N} deviations)

### Recommendation
{APPROVE | REQUEST_CHANGES | COMMENT} — {rationale for the recommendation}
```

For local-only reviews, omit the `**PR**:` line and the `### PR Metadata` section. Replace with `**Review Target**: Local changes ({branch_name} vs {base_branch})`.

---

## Phase 8: GitHub PR Feedback

### Skip Condition

**Skip this phase entirely** if reviewing local-only changes with no GitHub PR. Output the report from Phase 7 directly to the user and stop.

### 8.1 Determine Review Action

Based on the rating and findings:

- **Rating 4–5** with **no Critical or High findings** → submit as **APPROVE**.
- **Any Critical findings** → submit as **REQUEST_CHANGES**.
- **All other cases** → submit as **COMMENT** with the findings.

### 8.2 Post the Review

Use GitHub MCP server tools to submit the review:

1. Use `create_pull_request_review` to submit the overall review with the recommendation (APPROVE, REQUEST_CHANGES, or COMMENT). The review body should contain the summary, rating, PR metadata assessment, and recommendation from Phase 7.
2. Post **every finding that has a file and line reference** as an **inline review comment** on the specific file and line. This includes Critical, High, Medium, and Low severity findings — all of them belong in context where the author can see them next to the code. Use the GitHub MCP `create_pull_request_review` comments array or `add_pull_request_review_comment` to attach each finding to its file and line in the diff.
3. Format each inline comment with the severity badge, finding description, and recommendation:
   ```
   **🟡 Medium** — {finding description}

   **Recommendation**: {recommendation}
   ```
4. **Info** findings (observations, positive callouts) remain in the review body only — they are not actionable and do not need inline placement.
5. If a finding references a file but not a specific line (e.g., a structural concern about an entire file), post it as a file-level review comment rather than omitting it.

### 8.3 Avoid Duplicates

Before posting, check for existing review comments from previous runs of this skill. Do not post duplicate comments on the same finding at the same file and line. If a previous review already flagged the same issue, skip that inline comment.

---

## Important Notes

- **Always read supporting files first.** Before starting the pipeline, load `agent-map.md`, `checklist-map.md`, and `rating-rubric.md` from `.github/skills/review-pr/`. These files are required for Phases 2, 4, and 7 respectively. Do not proceed without them.
- **Sequential execution in council mode.** In council mode, the 4 reviewers **must** fire sequentially (one at a time). This is critical for determinism — each reviewer sees the same diff, same inputs, and the orchestrator merges findings reliably. Do NOT dispatch in parallel.
- **Handle reviewer failures gracefully.** If a reviewer fails or times out, proceed with the remaining reviewers and note which slot(s) were unavailable. A 3-reviewer consensus is valid but noted as degraded.
- **Repository-agnostic design.** This skill should work for any repository. BlogFlow-specific agents and checklists are the defaults, but the pipeline logic is general-purpose.
- **Professional and constructive tone.** When posting findings to GitHub, write them to help the author improve. Be specific, cite the relevant code, and suggest concrete fixes. Never be dismissive, sarcastic, or discouraging.
- **Design document conformance is mandatory for implementation PRs.** Phase 5 cross-checks the implementation against its governing design document. The design document is the source of truth — if the code diverges from the spec, that is a finding regardless of whether the code itself is well-written. This prevents "works but doesn't match spec" gaps that cascade into integration failures.
- **No duplicate comments.** Never post duplicate comments on the same finding. Check for existing review comments before posting inline feedback.
