# RFL Implementation Plan — BlogFlow

**Date:** 2026-06-21  
**Scope:** Bring BlogFlow's RFL system to feature parity with GameGrid's production reference  
**Reference:** `gamegrid/.github/skills/review-fix-loop/SKILL.md` (794 lines with §1–§6.6)

---

## 0. Current State Assessment

**What BlogFlow already has** (from the handoff phase):

| File | Lines | Status |
|------|-------|--------|
| `.github/skills/review-pr/SKILL.md` | ~350 | ✅ Created — 8-phase review pipeline |
| `.github/skills/review-pr/agent-map.md` | ~80 | ✅ Created — 10 BlogFlow agents × file patterns |
| `.github/skills/review-pr/checklist-map.md` | ~100 | ✅ Created — 14 checklists × patterns |
| `.github/skills/review-pr/rating-rubric.md` | ~150 | ✅ Created — 1–5 scale + formula + consensus |
| `.github/skills/review-fix-loop/SKILL.md` | ~320 | ⚠️ Created but **incomplete** (see §0 below) |
| `.github/skills/review-fix-loop/dismissal-rules.md` | ~120 | ✅ Created — mostly aligned with GameGrid |

**What BlogFlow is MISSING compared to GameGrid's 794-line SKILL.md:**

| GameGrid § | Feature | Status |
|------------|---------|--------|
| §1.5 | Run Identity (PR_NUMBER, PR_BRANCH, PR_URL, INITIAL_HEAD_SHA, LOOP_START_UTC) | ❌ Missing |
| §1.6 | Halt-Marker Check (pre-loop gate) | ❌ Missing |
| §3.1 | Council Composition Verification (model-param drift detection) | ❌ Missing |
| §3.3 | Sub-5/5 re-prompt procedure (mandatory escalation) | ⚠️ Partial — has "re-examine dismissals" but no re-prompt step |
| §3.4 | Deferral Filing Checkpoint (SQL table + GitHub issue filing + enforcement query + reconciliation) | ❌ Missing — dismissal-rules.md mentions deferrals generically |
| §5.5 | Pre-Merge Checklist Item #6 (CI green via branch-protection) | ❌ Missing |
| §6.1 | FINAL_HEAD_SHA capture | ❌ Missing |
| §6.2 | Council Composition Audit table in report | ❌ Missing |
| §6.4 | Post-verification via run-identity marker + retry logic | ❌ Missing |
| §6.5 | Composition Audit post-termination | ❌ Missing |
| §6.6 | CI Status Verification (branch-protection poll + path-filtered skip + rerun policy) | ❌ Missing |
| §Pre-Merge Checklist #1–#6 | Full 6-item pre-merge checklist | ⚠️ Partial — has 6 items but lacks specificity |

---

## 1. Target Architecture (What Are We Building?)

BlogFlow's RFL system is a **7-file skill framework** consisting of a review engine (`review-pr/`, 4 files) and a loop orchestrator (`review-fix-loop/`, 3 files) that together implement an iterative multi-model review-fix cycle. A PR is reviewed by **up to 4 parallel models** (Opus, Sonnet, GPT, Security-specialized Opus) using BlogFlow's **10 specialist agents** (Cloud-Native Systems Engineer, Security SME, SRE, Distributed Systems Architect, Front-End Engineer, Technical Writer, Product Manager, Program Manager, Solutions Engineer, Privacy & Compliance Lead) dispatched via agent-map.md file-pattern matching against 14 engineering checklists. Each round produces structured findings (id, severity, file:line, recommendation, consensus) that are classified as Actionable / Dismissed / Deferred per dismissal-rules.md. Actionable findings are fixed by matched agents in parallel (serialized on overlapping files). The loop repeats up to **5 rounds** and terminates **only when all 4 council slots return 5/5 with zero actionable findings** — a strict mandatory gate. Deferred findings require GitHub issues with run-identity markers, enforced via a session `deferrals` SQL table and cross-check against GitHub's durable record. A final **Council Composition Audit** and **CI Status Verification** (via `gh pr view statusCheckRollup` against branch-protection required checks) run before merge declaration. The progression report is posted as a PR comment, verified by run-identity marker query, and includes a round-by-round table, council composition audit, and findings summary.

---

## 2. File Inventory (Concrete List)

### Files to Modify

| File | Lines Changed | Purpose |
|------|---------------|---------|
| `.github/skills/review-fix-loop/SKILL.md` | +340 / -30 | Add §§1.5, 1.6, 3.1, 3.3 (sub-5/5 re-prompt), 3.4 (full deferral checkpoint), 6.1 (FINAL_HEAD_SHA), 6.2 (council audit table), 6.4 (post-verification), 6.5 (composition audit), 6.6 (CI verification), pre-merge checklist refinement |
| `.github/skills/review-pr/SKILL.md` | +10 / -10 | Update agent list reference from `.claude/agents/` to `.github/agents/` (BlogFlow's actual directory, not GameGrid's); update model versions from 4.6/5.4 to 4.7/5.5 |
| `.github/skills/review-pr/rating-rubric.md` | +5 | Add clarification that BlogFlow uses 4-slot council (no Gemini slot) |
| `.github/skills/review-fix-loop/dismissal-rules.md` | +20 / -5 | Add §2 protection of content-integrity domain (BlogFlow-specific), add explicit `blogflow-deferral` marker naming convention |
| `docs/RFL_CHECKLIST.md` | -62 (archive) | Replace with symlink or deprecation notice pointing to new skill docs; keep as historical archive |
| `.github/SKILL.md` | +50 (new) | Root skill index that references review-pr + review-fix-loop as sub-skills; entry point for RFL invocation |

### Files to Create

| File | Lines | Purpose |
|------|-------|---------|
| `.github/skills/review-fix-loop/deferral-gate.md` | ~120 | Self-contained deferral gate spec: SQL schema, GitHub issue format, enforcement query, reconciliation, consistency tripwire. Extracted from SKILL.md for composability |
| `.github/skills/review-fix-loop/ci-verification.md` | ~100 | Self-contained CI verification spec: branch-protection query, status rollup classification, path-filtered skip evidence, rerun policy, halt-marker format. Extracted from SKILL.md for composability |
| `.github/skills/review-fix-loop/halt-marker.md` | ~30 | Halt-marker and halt-closure comment formats, search query, lifecycle rules. Extracted from §§1.6, 6.6(8) |

---

## 3. Implementation Sequence (What to Build First)

### Phase 1 — Foundation (blocking)

These changes must land first. The loop has no value without a correct review engine and proper model-param discipline.

1. **Update `review-pr/SKILL.md` agent reference** — Change "10 available agents (defined in `.claude/agents/`)" → "10 available agents (defined in `.github/agents/`)" to match BlogFlow's actual directory structure. Update model versions from `claude-opus-4.6 / claude-sonnet-4.6 / gpt-5.4` → `claude-opus-4.7 / claude-sonnet-4.6 / gpt-5.5`.

2. **Add Council Composition Verification (§3.1 of review-pr/SKILL.md)** — Insert a new Phase 3.1 after the model dispatch table. This requires:
   - A protocol table listing exact `(agent_type, model)` tuples for all 4 slots
   - Read-back verification against the actual tool calls
   - Deviation handling (re-dispatch at original HEAD SHA, discard findings, annotate)
   - Composition record capture (slot, agent_type, model, dispatch HEAD, timestamp) per round
   - Gate: all aggregate-rating claims blocked until composition verified

3. **Fix reviewer slots for BlogFlow** — BlogFlow uses 4 slots in multi-model council (matching `review-pr/SKILL.md`). Ensure they map to BlogFlow's agent roster:
   - **Slot Architect**: `general-purpose` → `claude-opus-4.7` (deep reasoning — architecture implications)
   - **Slot Balanced**: `general-purpose` → `claude-sonnet-4.6` (code quality, patterns)
   - **Slot Quality**: `general-purpose` → `gpt-5.5` (alternative perspective)
   - **Slot Security**: `cloud-native-security-sme` → `claude-opus-4.7` (security focus)

### Phase 2 — Loop Critical Path (required for merge readiness)

These changes implement the loop mechanics that make RFL reliable. Without them, the loop produces unverified results.

4. **Add Run Identity capture (§1.5)** — After §1.4 "Verify PR Scope", insert a new §1.5:
   ```
   - PR_NUMBER = `gh pr view --json number --jq '.number'`
   - PR_BRANCH = `git rev-parse --abbrev-ref HEAD`
   - PR_URL = `gh pr view --json url --jq '.url'`
   - INITIAL_HEAD_SHA = `git rev-parse HEAD`
   - LOOP_START_UTC = `date -u +%FT%TZ`
   ```
   These are consumed by §§1.6, 3.4, 6.1, 6.4, 6.6.

5. **Add Halt-Marker Check (§1.6)** — New section probing the PR for `<!-- blogflow-rfl-halt pr=N head=... reason=... -->` markers from prior invocations. If any open markers lack a closure comment `<!-- blogflow-rfl-halt-resolved pr=N halt_head=... resolution_head=... evidence=... -->`, STOP and surface error.

6. **Add Deferral Filing Checkpoint (§3.4)** — The most complex new section. Requires:
   - SQL deferrals table creation (PR-scoped, finding_id PK)
   - GitHub issue creation via `gh issue create` with marker: `<!-- blogflow-deferral pr=N finding_id=X severity=Y -->`
   - Required labels: `service:*|tech-debt`, `type:tech-debt`, `priority:*`
   - Protected-domain re-assertion (`none` only)
   - Enforcement query (a): `SELECT pr, finding_id FROM deferrals WHERE pr=... AND issue_number IS NULL`
   - Enforcement query (b): `gh issue list --search "blogflow-deferral pr=N in:body"` → marker regex parse → reconciliation INSERT
   - Consistency tripwire (Q3), re-verify (Q4)
   - Extract to `deferral-gate.md` for composability; SKILL.md references it

7. **Replace §3.3 termination conditions** — Current BlogFlow termination ("target rating achieved with no Critical/High") is too weak. Replace with GameGrid's three conditions:
   1. **ALL** council slots return `5/5` AND zero actionable findings → terminate successfully
   2. `current_round >= max_rounds` → terminate as **NOT merge-ready** (unless all 5/5)
   3. No progress → terminate as **NOT merge-ready**
   Plus: **Sub-5/5 with no actionable findings** — DO NOT terminate. Re-prompt dissenting reviewer for concrete line-level change that raises rating to 5/5. If no concrete fix and no rating revision → protocol failure, surface impasse.

8. **Add CI Status Verification (§6.6)** — New section that:
   - Resolves post-fix HEAD and required-check set (branch-protection or fallback: `Lint`, `Build`, `Test`, `Docker`)
   - Queries `gh pr view N --json statusCheckRollup` at that HEAD
   - Classifies each check (SUCCESS / FAILURE / SKIPPED / in-flight / missing)
   - Path-filtered skip allowance with evidence
   - Polling loop for in-flight (45 min cap)
   - Rerun policy (transient-class explicit allowlist)
   - Extract to `ci-verification.md` for composability

9. **Add Council Composition Audit (§6.5)** — Post-termination audit that:
   - Enumerates every counted round's per-slot `(agent_type, model)` pairs
   - Verifies against the protocol table
   - Missing composition → corrective full 4-model council
   - Uncorrected deviation → corrective round (R{n}-corrective)
   - Updates composition record for re-prompt revisions

10. **Add Post-Verification (§6.4)** — After posting progression report comment, verify it exists by:
    - Run-identity marker query: `gh pr view N --json comments --jq '... contains("<!-- blogflow-rfl-report pr=N head=FINAL_SHA rounds=FINAL_ROUNDS -->") ...'`
    - Validate body contains report heading, progression table, final rating, council audit
    - On failure: re-post with retry cap of 2 (3 total)
    - On double failure: HARD ERROR, do not declare terminated

11. **Add FINAL_HEAD_SHA capture (§6.1)** — After final review pass completes, capture FINAL_HEAD_SHA and FINAL_ROUND_COUNT. These populate the run-identity marker and report.

### Phase 3 — Maturity (nice-to-have)

12. **Create `.github/SKILL.md`** — Root skill index that references both sub-skills, provides the top-level "invoke RFL" command, and serves as the discoverable entry point for agents and humans.

13. **Archive `docs/RFL_CHECKLIST.md`** — Move to `docs/archive/RFL_CHECKLIST.md` with a deprecation header pointing to the new skill framework.

---

## 4. Agent-to-Checklist Wiring

For each of BlogFlow's 10 agents (agent-map.md already defines these), here is which checklists fire when their file patterns match:

| Agent | When Files Matched | Checklists Fired |
|-------|-------------------|------------------|
| **Cloud-Native Systems Engineer** | `*.go`, `cmd/**`, `internal/**` (config, content, gitops, overlayfs, server, envfile, otel), `go.mod`, `go.sum` | 03, 03a (Go Coding Standards), 05 (Security — on all Go files), 08 (Performance — hot paths), 09 (Telemetry), 09a (Logging — when `slog.` calls), 09b (Metrics — when `prometheus.` calls) |
| **Cloud-Native Security SME** | `**/auth/**`, `**/security/**`, `**/*secret*`, `**/*token*`, `**/*credential*`, `**/middleware/**`, `internal/gitops/webhook.go`, `internal/config/**` (YAML secrets), `internal/overlayfs/**` (path traversal), `defaults/static/*.css` (CSP) | **05 (Security — CRITICAL)**, 07 (Privacy), 09a (Logging redaction) |
| **Cloud-Native Site Reliability Engineer** | `k8s/**`, `helm/**`, `Dockerfile*`, `.github/workflows/**`, `internal/server/*server.go*`, `internal/otel/**`, `internal/content/*renderer.go*`, `internal/content/*cache.go*` | 10 (Runtime/Containers), 09 (Telemetry), 09b (Metrics — `/metrics` endpoint), 09c (Tracing) |
| **Cloud-Native Distributed Systems Architect** | `docs/engineering/design/**`, `docs/engineering/adr/**`, `internal/overlayfs/**`, `internal/content/**`, `internal/config/**`, `internal/gitops/**`, new service/component directories | 01 (Architecture), 03 (Coding conventions), cross-domain impact |
| **Cloud-Native Front-End Engineer** | `defaults/templates/**`, `defaults/static/**` (all CSS/JS/images), `*.html`, `*.css`, `internal/theme/**`, theme `*.yaml` | 03, 03b (TS/React Standards), 05 (CSP/XSS on CSS/HTML), 04a (UI test cases) |
| **Technical Writer** | `**/*.md`, `docs/**`, `CHANGELOG.md`, `README.md`, `CONTRIBUTING.md` | 11 (Documentation), on PR metadata (title, description) |
| **Product Manager** | `docs/engineering/adr/**` (product impact), `CHANGELOG.md`, `docs/persona/agents/**`, `defaults/templates/**` (user-visible) | 01 (Architecture — product impact), 11 (UX documentation) |
| **Program Manager** | `.github/workflows/**`, `docs/engineering/design/**` (multi-phase implications), `README.md`, all PRs touching 3+ domains | Cross-domain coordination, phase tracking |
| **Solutions Engineer** | `cmd/**` (CLI behavior), `README.md` (getting-started), `defaults/` (zero-config experience), `.github/skills/**` | DX assessment, onboarding path |
| **Privacy & Compliance Lead** | `**/auth/**`, `**/credential*`, `internal/config/**` (user/policy data), `**/*.json`, `**/*.yaml` with "pii"/"cookie"/"gdpr" patterns | **07 (Privacy — CRITICAL when user data present)**, 05 (Security — data handling) |

### Agent Dispatch Priority Order

1. **Cloud-Native Security SME** — CRITICAL (never deprioritized)
2. **Cloud-Native Systems Engineer** — HIGH (Go/core always reviewed)
3. **Cloud-Native Distributed Systems Architect** — HIGH (architecture always reviewed)
4. **Cloud-Native Site Reliability Engineer** — HIGH (operability always reviewed)
5. **Cloud-Native Front-End Engineer** — STANDARD (when theme/templates present)
6. **Privacy & Compliance Lead** — CRITICAL (when user data or secrets present)
7. **Product Manager** — SUPPLEMENTARY (when ADRs or UX-decisions involved)
8. **Program Manager** — SUPPLEMENTARY (when cross-repo or multi-phase)
9. **Technical Writer** — SUPPLEMENTARY (when docs present)
10. **Solutions Engineer** — SUPPLEMENTARY (when DX or onboarding affected)

---

## 5. Deferral Tracking Mechanism

### SQL Table Schema

```sql
CREATE TABLE IF NOT EXISTS deferrals (
  pr              INTEGER NOT NULL,              -- PR_NUMBER from Run Identity
  round           INTEGER NOT NULL,              -- RFL round number (1-based)
  finding_id      TEXT    NOT NULL,              -- "R{n}-F{m}"
  severity        TEXT    NOT NULL,              -- critical|high|medium|low|info (lowercase)
  summary         TEXT    NOT NULL,              -- Short description of the finding
  deferral_reason TEXT    NOT NULL,              -- Why it cannot be fixed in this PR
  source          TEXT    NOT NULL DEFAULT 'session-filed',  -- 'session-filed' | 'reconciled-from-github'
  issue_number    INTEGER,                        -- Assigned after gh issue create succeeds
  filed_at        TIMESTAMP                        -- ISO-8601 UTC
  -- Composite PK enforced by ON CONFLICT
  PRIMARY KEY (pr, finding_id)
);
```

- `pr` bound to `PR_NUMBER` from Run Identity (§1.5). Never accept externally passed PR number.
- `source` provenance: `'session-filed'` for in-loop classification; `'reconciled-from-github'` for rows rebuilt from GitHub's durable record on fresh sessions.
- `issue_number`, `filed_at`, and `source` MUST be append-only: once set, no subsequent pass may overwrite them.

### GitHub Issue Creation Format

For every row where `issue_number IS NULL`:

```bash
gh issue create \
  --title "RFL Deferral: {short_summary}" \
  --label "type:tech-debt" \
  --label "service:blogflow" \
  --label "priority:medium" \
  --label "rfl-deferred"
```

**Issue body MUST contain** (in order):

1. **Run-identity marker** (first line, HTML comment):
   ```html
   <!-- blogflow-deferral pr=42 finding_id=R3-F7 severity=medium -->
   ```
   (severity LOWERCASE in marker; title-case in quoted finding below)

2. **Backlink to source PR**:
   ```
   Deferred from PR #42 — finding R3-F7
   ```

3. **Quoted finding** (per rating-rubric Finding Body Format Contract):
   ```
   **Finding (Medium, Security R3):** Webhook handler does not enforce ip_allowlist from config.
   ```

4. **Deferral rationale**:
   ```
   Cannot fix because: requires changes to a different repository or infrastructure that doesn't exist yet.
   Risk assessment: Low — all webhook payloads are still validated by HMAC; IP filtering is defense-in-depth.
   ```

5. **Protected-domain assessment** (on its own line):
   ```
   Protected-domain assessment: none
   ```
   (`security`, `tenant-isolation`, `data-integrity`, `cryptography`, `blogflow-content-integrity` are NOT allowed — hard error)

6. **Concrete acceptance criteria** so the issue is actionable later.

### Enforced Labels

After creation, verify via:
```bash
gh issue view "$N" --json labels --jq '[.labels[].name]'
```
Assert: at least one of `service:*` OR `tech-debt`, one `type:tech-debt`, one `priority:*`. If missing, apply labels and re-verify.

### Session Restart Reconciliation

When a fresh session starts on a previously-reviewed PR:

1. At §3.4.3(b), query GitHub:
   ```bash
   gh issue list --state all \
     --search "blogflow-deferral pr=${PR_NUMBER} in:body" \
     --json number,title,body,state \
     --jq '.[] | {number, state, title, body}'
   ```

2. Parse marker regex (multiline-mode anchored):
   ```
   (?m)^\s*<!--\s*blogflow-deferral\s+pr=(\d+)\s+finding_id=(\S+)\s+severity=(critical|high|medium|low|info)\s*-->\s*$
   ```

3. For each OPEN issue whose `finding_id` is NOT in local `deferrals` table:
   - Validate: protected-domain = `none`, labels present, severity cross-check
   - INSERT with `source = 'reconciled-from-github'` and `round = 0` sentinel

4. For CLOSED issues whose `finding_id` ALSO appears as Deferred in current session → hard error (resurfaced-after-close pattern), unless an OPEN issue supersedes it.

5. Run consistency tripwire: compare local `deferrals` issue_numbers against (b) result set. Missing rows = search-coverage failure = hard error.

6. On fresh-session, additionally seed reconciliation expectations from prior RFL report comment (parsed via `<!-- blogflow-rfl-report pr=N -->` marker query).

### Finding Types That CANNOT Be Deferred (from dismissal-rules.md §2.1)

| Domain | Examples |
|--------|----------|
| **Security** | Auth bypass, credential exposure, injection, hardcoded secrets |
| **Content Integrity** | Path traversal, symlink escape in overlayfs, cross-site content leakage |
| **Data Integrity** | Schema correctness, referential integrity, data loss risk |
| **Cryptography** | HMAC validation bypass, weak key length enforcement |

---

## 6. Halt Marker Format

**Halt marker** (posted by the system on CI failure, stuck checks, or post-fix SHA drift):
```html
<!-- blogflow-rfl-halt pr=42 head=abc123 reason=ci-red -->
```

**Halt-closure comment** (posted when the halt is resolved):
```html
<!-- blogflow-rfl-halt-resolved pr=42 halt_head=abc123 resolution_head=def456 evidence=ci-passing -->
```

All prior open halts MUST be checked at loop start (§1.6). No open halt without closure = hard stop.

---

## 7. Estimated Total Lines of New Code

| Phase | Action | Lines |
|-------|--------|-------|
| **P1** | §3.1 Council Composition Verification (new section in review-pr/SKILL.md) | ~90 |
| **P1** | Update agent reference + model versions (review-pr/SKILL.md) | ~2 / -2 |
| **P2** | §1.5 Run Identity + §1.6 Halt-Marker Check (in SKILL.md) | ~50 |
| **P2** | §3.4 Deferral Filing Checkpoint (SKILL.md + new deferral-gate.md) | ~160 + 120 = 280 |
| **P2** | §3.3 Replace termination conditions (SKILL.md) | ~40 / -15 |
| **P2** | CI Status Verification (SKILL.md + new ci-verification.md) | ~80 + 100 = 180 |
| **P2** | §6.4 Post-Verification (SKILL.md) | ~40 |
| **P2** | §6.5 Council Composition Audit (SKILL.md) | ~50 |
| **P2** | §6.1 FINAL_HEAD_SHA + agent-map fix in dismissal-rules.md | ~10 / -5 |
| **P2** | Agent-map protection of content-integrity domain | ~20 / -5 |
| **P3** | `.github/SKILL.md` root skill index | ~50 |
| **P3** | Archive `docs/RFL_CHECKLIST.md` | ~5 moved |
| **TOTAL** | | **~905 new / ~27 removed** |

---

## 8. Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| **Sub-5/5 re-prompt impasse** — a council slot holds sub-5 with zero actionables and refuses to state a concrete fix. The loop has no way to escape. | Infinite loop or premature termination without quality. | §3.3 escalation: surface full transcript to human reviewer. No agent auto-terminates in this state. This is a protocol failure, not a quality decision. |
| **Session SQL lost between rounds** — if the orchestrator's session resets, the deferrals table is empty. | Deferral gate passes vacuously; deferred items never get GitHub issues. | §3.4.5 persistence model: GitHub is the source of truth. Fresh session triggers §3.4.3(b) reconciliation from GitHub's durable record before any gate evaluation. |
| **GitHub search-index lag** — newly created `blogflow-deferral` issues don't appear in `gh issue list --search` within seconds. | §3.4.3(b) returns fewer issues than expected; consistency tripwire fires false-positive. | Exclude rows with `filed_at >= LOOP_START_UTC` from the tripwire. On fresh sessions, seed reconciliation from the prior RFL report comment. Retry `gh issue list` once after 15s if result count is lower than expected. |
| **Model-param drift** — a model call silently uses `gpt-5.4` instead of `gpt-5.5`, producing falsely-confident unanimity. | False "unanimous 5/5" results; unsafe merge. | §3.1 Composition Verification: mandatory read-back of every `(agent_type, model)` pair against the protocol table. Deviant rounds are voided and re-dispatched. |
| **CI verification false pass** — local build passes but CI is red (Buf lint, Docker build, etc.). | PR merged with failing CI. | §6.6 mandates `gh pr view statusCheckRollup` against POST_FIX_HEAD. Requires ALL required checks SUCCESS (fetched from branch-protection). |
| **CI check set drift** — required checks change without RFL skill update. | Stale required-check list; missing CI gates. | §6.6 fetches required checks from branch-protection API dynamically. Falls back to fallback allowlist (Lint, Build, Test, Docker) when protection not configured. |
| **Report post silently fails** — `gh pr comment` errors but orchestrator claims success. | No audit trail; human reviewer has no RFL report to inspect. | §6.4 post-verification via run-identity marker query. Retry cap of 2 (3 total). On double failure: HARD ERROR, do not declare terminated. |
| **Scope creep in fixes** — agents fix beyond their assigned findings, introducing new issues. | New findings in each round → infinite loop. | Agent dispatch includes explicit "Do NOT modify code unrelated to the findings" instruction. §4.3 spot-check required for all Critical/High fixes. |
| **CI rollback on rebase** — post-merge CI failures on the merged PR. | Broken main branch. | "When to re-run the full RFL" section includes: "New commits are pushed after the RFL report was posted" and "PR base branch changed." Re-run is mandatory. |
| **Agent-map pattern conflicts** — a file matches multiple agents with overlapping priority. | Ambiguous ownership; findings may not get the right specialized review. | agent-map.md already defines dispatch rules: "Security SME matches any `auth/`, `security/`, `*secret*` pattern → CRITICAL — always active." Agent dispatch priority list resolves conflicts. |

---

## 9. Merge Policy Summary (What's Required Before Merge)

Every PR must pass ALL of these before a human reviewer merges:

1. **Full 4-model council RFL** completed (not a spot-check)
2. **5/5⭐ from ALL 4 council slots** with **zero actionable findings** (strict gate)
3. **Council composition verified** — every round's `(agent_type, model)` pairs match the protocol table
4. **Sub-5/5 gap resolved** — no reviewer holds sub-5 unconditionally (they stated a concrete fix that was applied, or they revised to 5/5)
5. **Deferral gate cleared** — all deferred findings have GitHub issues with blogflow-deferral markers, required labels, and `protected-domain assessment: none`
6. **Dismissal triage verified** — for any round with 5+ dismissals or protected-domain dismissals
7. **Final report posted** as PR comment with Council Composition Audit table
8. **Report post verified** via run-identity marker query
9. **CI green** — all required checks (from branch-protection) SUCCESS at POST_FIX_HEAD
10. **No open halt markers** on the PR at loop termination time

**Agents MUST NOT merge PRs.** The human reviewer merges after inspecting the RFL report.

---

## 10. File Paths Reference (BlogFlow-Specific)

| Category | Path |
|----------|------|
| Review skill | `.github/skills/review-pr/SKILL.md` |
| Agent map | `.github/skills/review-pr/agent-map.md` |
| Checklist map | `.github/skills/review-pr/checklist-map.md` |
| Rating rubric | `.github/skills/review-pr/rating-rubric.md` |
| Loop orchestrator | `.github/skills/review-fix-loop/SKILL.md` |
| Dismissal rules | `.github/skills/review-fix-loop/dismissal-rules.md` |
| Deferral gate (NEW) | `.github/skills/review-fix-loop/deferral-gate.md` |
| CI verification (NEW) | `.github/skills/review-fix-loop/ci-verification.md` |
| Halt marker spec (NEW) | `.github/skills/review-fix-loop/halt-marker.md` |
| Root skill index (NEW) | `.github/SKILL.md` |
| Agent personas (source of truth) | `.github/agents/` (10 agent files) |
| Agent specs (archival) | `docs/persona/agents/` |
| Engineering checklists | `docs/engineering/checklists/` |
| Design documents | `docs/engineering/design/` |
| CI workflows | `.github/workflows/` (ci.yml, deploy.yml, publish.yml, release.yml) |
| Go internal packages | `internal/config/`, `internal/content/`, `internal/envfile/`, `internal/gitops/`, `internal/otel/`, `internal/overlayfs/`, `internal/server/`, `internal/theme/` |
| Embedded defaults | `defaults/templates/`, `defaults/static/`, `defaults/config/` |

---

*This plan is a concrete implementation guide. Every item has a target file path and purpose. An agent can execute Phase 1 (Foundation) as a single PR, Phase 2 (Loop Critical Path) as a second PR, and Phase 3 (Maturity) as a third PR. Target total new code: ~905 lines, with ~27 lines removed (obsolete patterns).*
