---
name: design-doc
description: >-
  Generates comprehensive component design documents from GitHub issues. Orchestrates specialist
  agent personas — Architect for structure and logic, Security SME for threat modelling, SRE for
  observability and rollout — to produce a complete design document following the standard template.
  Creates a worktree branch, commits the design doc, opens a PR, and invokes the review-fix-loop
  skill for automated quality improvement.
---

# Design Document Skill — Orchestration Instructions

Generate a comprehensive design document for a BlogFlow component or feature. Read all supporting files before beginning:

- `docs/engineering/design/000-template.md` — the design document template (all sections must be populated)
- `docs/engineering/design/README.md` — design doc conventions and file placement rules
- `.github/skills/design-doc/section-map.md` — which agent persona owns which template section
- `.github/skills/design-doc/checklist-refs.md` — which engineering checklists to cross-reference per section
- `.github/skills/review-fix-loop/SKILL.md` — the review-fix loop used in Phase 8

---

## Phase 1: Issue Analysis & Context Gathering

### 1.1 Identify the Source Issue

Determine the GitHub issue that defines the work being designed:

- **Issue number provided explicitly**: Use `issue_read` with method `get` to fetch the issue title, body, labels, and linked issues. Also fetch comments with method `get_comments` for additional context.
- **Issue URL provided**: Extract the owner, repo, and issue number from the URL, then fetch as above.
- **No issue provided**: Ask the user to provide an issue number or URL. This skill requires a source issue to proceed.

### 1.2 Extract Design Context

From the issue, extract:

1. **Component name**: Derive from the issue title or body. This becomes the document title and filename slug.
2. **Feature category**: Determine from issue labels, referenced requirements, or the component's domain. Use the following categories as a guide:
   - `01-core-server` — HTTP server, configuration, CLI, caching
   - `02-content-pipeline` — content parsing, markdown rendering, front matter, feed generation, sitemaps
   - `03-theme-engine` — theme loading, template rendering, default theme, static assets
   - `04-storage-sync` — overlay filesystem, git sync, git auth, file watching
   - `05-integrations` — webhooks, external service integration
   - `06-infrastructure` — deployment, monitoring, CI/CD, containerization
   - If the component spans categories or doesn't fit, place the doc flat in `docs/engineering/design/`.
3. **Referenced requirements**: Scan the issue body for requirement identifiers. If found, read the corresponding requirements doc to gather acceptance criteria, priority, and dependencies.
4. **Related ADRs**: Scan the issue body and referenced requirements for ADR references. Read any linked ADRs from `docs/engineering/adr/`.
5. **Existing design docs**: Check `docs/engineering/design/` for any existing design doc for this component. If one exists, this is an **update** operation — see the Update Mode Reconciliation note in Important Notes.

### 1.3 Determine File Placement

Based on the extracted context:

- If the component clearly belongs to a single feature category, place the doc at `docs/engineering/design/<category>/<component-slug>.md`. Create the subdirectory if it doesn't exist.
- If the component spans categories or is a platform-level concern, place it at `docs/engineering/design/<component-slug>.md`.
- Use lowercase kebab-case for the filename: `content-pipeline.md`, `overlay-filesystem.md`.

### 1.4 Output Context Summary

Before proceeding, output a context block:

```
📋 Design Document Context
━━━━━━━━━━━━━━━━━━━━━━━━━
Issue:       #NNN — [Title]
Component:   [Component Name]
Category:    [Feature Category or "Platform-level"]
File:        docs/engineering/design/[path]
Mode:        New | Update
Requirements: [list] or "None referenced"
ADRs:        [ADR-NNN, ...] or "None referenced"
```

---

## Phase 2: Architect — Structure & Logic

### 2.1 Load Architect Context

Read the section map (`.github/skills/design-doc/section-map.md`) to confirm which sections the Architect agent owns. Load the following reference docs:

- `docs/engineering/best-practices/01-architecture.md`
- `docs/engineering/best-practices/02-microservices.md`
- `docs/engineering/checklists/01-architecture-checklist.md`
- `docs/engineering/checklists/02-microservice-implementation-checklist.md`
- Any ADRs referenced by the issue

### 2.2 Generate Architecture Sections

Using the `cloud-native-distributed-systems-architect` agent persona, generate:

- **§1 · Overview** — What the component is, its functionality, importance, and requirements traceability. Ground this in the issue description and referenced requirements.
- **§2 · Logical Architecture** — High-level architecture diagram (Mermaid `graph`), component boundaries table, data flow (Mermaid `sequenceDiagram`), data model, API surface, dependencies table, and related considerations.
- **§9 · Open Questions & Decisions** — Initial questions surfaced during architecture analysis.

### 2.3 Architecture Quality Gates

Validate the generated architecture sections against the architecture checklist:

- [ ] Component boundaries are explicit and non-overlapping
- [ ] Data flow diagrams show both happy-path and error paths
- [ ] Dependencies include failure behaviour for each
- [ ] API surface specifies error codes and rate limits

---

## Phase 3: Functional Design & Performance

### 3.1 Load Functional Context

Read the checklist references (`.github/skills/design-doc/checklist-refs.md`) for sections §3 and §4. Load:

- `docs/engineering/best-practices/04-testing.md`
- `docs/engineering/best-practices/08-performance.md`
- `docs/engineering/checklists/04-testing-checklist.md`
- `docs/engineering/checklists/04a-unit-testing-checklist.md`
- `docs/engineering/checklists/04b-integration-testing-checklist.md`
- `docs/engineering/checklists/08-performance-checklist.md`

### 3.2 Generate Functional Test Scenarios

Using the `cloud-native-distributed-systems-architect` agent persona (with systems engineering depth), generate:

- **§3 · Functional Test Scenarios** — Happy-path scenarios, edge cases and error scenarios, integration test boundaries, and acceptance criteria mapping from the source issue.

Every acceptance criterion from the issue **must** map to at least one test scenario. If an acceptance criterion cannot be mapped, flag it in §9 (Open Questions).

### 3.3 Generate Performance Section

Generate:

- **§4 · Performance** — Load profile, latency targets (p50/p95/p99), throughput targets, scaling strategy, resource budgets, and performance test plan.

Reference the performance best-practices and checklist.

---

## Phase 4: Security SME — Security & Threat Model

### 4.1 Load Security Context

Read the section map to confirm Security SME ownership of §5 and §6. Load:

- `docs/engineering/best-practices/05-security.md`
- `docs/engineering/best-practices/07-privacy.md`
- `docs/engineering/checklists/05-security-checklist.md`
- `docs/engineering/checklists/07-privacy-checklist.md`

### 4.2 Generate Security Sections

Using the `cloud-native-security-sme` agent persona, generate:

- **§5 · Security** — Authentication & authorization model, data classification table, input validation strategy, and content integrity approach.
- **§6 · Threat Model** — Trust boundary diagram (Mermaid), threat actors and attack surfaces, STRIDE analysis table, and mitigations with residual risks.

### 4.3 Security Quality Gates

Validate against the security checklist:

- [ ] Every data element is classified (public, internal, confidential, restricted)
- [ ] Encryption at rest and in transit is specified for confidential/restricted data
- [ ] STRIDE table has an entry for every interface exposed by the component
- [ ] Trust boundary diagram includes all network and authentication boundaries

---

## Phase 5: SRE — Observability & Rollout

### 5.1 Load SRE Context

Read the section map to confirm SRE ownership of §7 and §8. Load:

- `docs/engineering/best-practices/09-telemetry-observability.md`
- `docs/engineering/best-practices/02-microservices.md`
- `docs/engineering/checklists/09a-logging-checklist.md`
- `docs/engineering/checklists/09b-metrics-checklist.md`
- `docs/engineering/checklists/09c-distributed-tracing-checklist.md`
- `docs/engineering/checklists/10-runtime-environment-checklist.md`
- `docs/engineering/checklists/01-architecture-checklist.md`
- `docs/engineering/checklists/02-microservice-implementation-checklist.md`

### 5.2 Generate Observability Sections

Using the `cloud-native-site-reliability-engineer` agent persona, generate:

- **§7 · Observability** — Logging strategy with structured log levels, RED/USE metrics with Prometheus-style metric names, distributed tracing strategy, and alerting rules with severity and escalation.

### 5.3 Generate Rollout Sections

Generate:

- **§8 · Rollout & Risk** — Rollout strategy (canary/blue-green/feature flags), rollback plan, risk register, dependency sequencing, and launch checklist.

### 5.4 SRE Quality Gates

Validate against the observability and runtime checklists:

- [ ] Logging includes request_id and trace_id in structured fields
- [ ] Metrics follow RED pattern for request-driven components
- [ ] At least one critical alert and one high alert are defined
- [ ] Rollback plan includes trigger criteria and expected rollback time
- [ ] Launch checklist references all relevant engineering checklists

---

## Phase 6: Assembly & Validation

### 6.1 Merge Agent Outputs

Combine the outputs from Phases 2–5 into a single design document following the `000-template.md` structure. Ensure:

1. **Metadata blockquote** is populated: Status = "Draft", Issue = source issue link, Author = "design-doc skill", Reviewers = agent personas used, Last Updated = today's date.
2. **All 10 sections** are present in order (§1–§10).
3. **§10 · References** links to the source issue, all referenced requirements, ADRs, and engineering docs consulted during generation.

### 6.2 Completeness Check

Verify every template section is populated. For each section:

- **Populated**: Content is present and substantive.
- **Partially populated**: Content exists but is incomplete. Add a `<!-- TODO: [what's missing] -->` comment.
- **Not applicable**: Section is explicitly marked "Not applicable — [reason]."
- **Missing**: Section has no content. Add `<!-- TBD — needs input: [what information is needed] -->`.

Output a completeness summary:

```
✅ §1 Overview — Complete
✅ §2 Logical Architecture — Complete
⚠️ §3 Functional Test Scenarios — Partial (edge cases need domain input)
✅ §4 Performance — Complete
✅ §5 Security — Complete
✅ §6 Threat Model — Complete
✅ §7 Observability — Complete
✅ §8 Rollout & Risk — Complete
⚠️ §9 Open Questions — 3 questions pending
✅ §10 References — Complete
```

### 6.3 Mermaid Validation

Verify all Mermaid diagrams are syntactically valid:

- Each diagram block starts with ` ```mermaid ` and ends with ` ``` `
- Diagram type is declared (`graph`, `sequenceDiagram`, `flowchart`, etc.)
- No orphaned nodes or broken references

### 6.4 Cross-Reference Validation

Verify that:

- All requirement IDs mentioned in the doc correspond to real requirements
- All ADR references link to existing ADRs
- All checklist references use correct checklist numbers

---

## Phase 7: Git Workflow & PR

### 7.1 Create Worktree Branch

Create a git worktree for the design doc work:

```bash
BRANCH_NAME="design/$(echo '<component-slug>' | tr '[:upper:]' '[:lower:]')"
git worktree add /tmp/blogflow-design-${BRANCH_NAME##*/} -b "$BRANCH_NAME" main
```

### 7.2 Write the Design Document

Write the assembled design document to the determined file path within the worktree. If a categorized subdirectory is needed, create it first.

### 7.3 Commit and Push

```bash
cd /tmp/blogflow-design-<slug>
git add docs/engineering/design/
git commit -m "Design: <Component Name> design document

Generates design document for <Component Name> from issue #NNN.
Covers architecture, security, threat model, observability, and rollout.

Refs #NNN

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin "$BRANCH_NAME"
```

### 7.4 Open Pull Request

Use GitHub CLI or MCP tools to create a PR:

- **Title**: `Design: <Component Name> design document`
- **Body**: Include a summary of the design doc with links to each section, the completeness summary from Phase 6, and a link to the source issue.
- **Labels**: `design-doc`, `documentation`
- **Linked issue**: Reference the source issue

---

## Phase 8: Review-Fix Loop

### 8.1 Invoke Review-Fix Loop

After the PR is open, invoke the `review-fix-loop` skill to automatically review and improve the design document:

1. The review-fix loop will invoke the `review-pr` skill to review the design doc PR.
2. By default, `docs/engineering/design/` files match the `technical-writer` agent via agent-map.md. However, design documents require deeper specialist review. When invoking the review, explicitly instruct it to include all agents that participated in generation: `cloud-native-distributed-systems-architect`, `cloud-native-security-sme`, and `cloud-native-site-reliability-engineer`, in addition to the pattern-matched agents.
3. Findings will be evaluated and actionable items will be fixed automatically.
4. The loop repeats until no new actionable findings are discovered or the max round limit is reached.

### 8.2 Final Report

After the review-fix loop completes, output a summary:

```
📄 Design Document — Complete
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Component:   <Component Name>
File:        docs/engineering/design/<path>
PR:          #NNN
Issue:       #NNN
Status:      Ready for human review

Review Rounds: N
Final Rating:  ⭐⭐⭐⭐⭐

Sections:
  §1 Overview                    ✅
  §2 Logical Architecture        ✅
  §3 Functional Test Scenarios   ✅
  §4 Performance                 ✅
  §5 Security                    ✅
  §6 Threat Model                ✅
  §7 Observability               ✅
  §8 Rollout & Risk              ✅
  §9 Open Questions              ⚠️ 2 questions pending
  §10 References                 ✅
```

---

## Important Notes

- **Issue is the source of truth.** The GitHub issue defines what is being designed. Requirements and feature research docs are supplementary context referenced by the issue.
- **Update mode reconciliation.** If a design doc already exists for the component, do not regenerate from scratch. Instead: (1) read the existing doc section by section, (2) compare each section against the newly generated content, (3) preserve human-authored content that is more detailed or has been marked as reviewed, (4) merge new material into sections that are incomplete or marked TBD, (5) flag any conflicts between existing and generated content in §9 (Open Questions) for human resolution. Output a section-by-section comparison showing what changed.
- **Agent persona depth.** Each agent persona should draw on its full knowledge: the Architect references architecture ADRs and best-practices, the Security SME references security checklists and threat modelling patterns, the SRE references observability standards and deployment pipelines.
- **Mermaid diagrams are required.** Sections §2.1 (architecture), §2.3 (data flow), and §6.1 (trust boundaries) must include Mermaid diagrams. Other sections may include diagrams where they aid understanding.
- **TBD is acceptable.** Not every section can be fully populated from an issue alone. Mark gaps with `<!-- TBD -->` comments so human reviewers know what needs input.
- **Design docs are living documents.** They should be updated as implementation reveals new information. The initial generation is a starting point, not a final artifact.
- **Worktree cleanup.** After Phase 8 completes (or if the skill exits early due to an error), remove the worktree: `git worktree remove /tmp/blogflow-design-<slug>`. This prevents stale worktrees from accumulating across invocations.
