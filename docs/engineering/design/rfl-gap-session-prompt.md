# RFL Gap — New Session Prompt

## Context

BlogFlow currently has **no automated RFL (Review-Fix-Loop) skill system**. We only have a single static file: `docs/RFL_CHECKLIST.md` — a 62-line one-pass checkbox document with no iteration, no numeric ratings, no structured findings, and no multi-model council.

GameGrid has a production-grade RFL system with 7 supporting skill files and a fully automated iterative review loop. We have a **massive gap** between our approach and the reference implementation.

The detailed gap analysis has been generated at:
`docs/engineering/design/rfl-handoff-gap-analysis.md`

We've also created the reference files for BlogFlow's agent personas and checklists (based on GameGrid's patterns, wired to BlogFlow's actual agents and directories):

- `.github/skills/review-pr/agent-map.md` — 11 BlogFlow agents mapped to file patterns
- `.github/skills/review-pr/checklist-map.md` — 14 engineering checklists mapped to file patterns
- `.github/skills/review-pr/rating-rubric.md` — 1–5 numeric scale + mathematical formula

## Your Task

Build a **concrete implementation plan** for bringing BlogFlow's RFL system up to GameGrid's parity. This plan will guide implementation work, so make it specific and actionable.

### Output Format

Return **one markdown document** organized as follows. Each section must be filled out — no skips.

```markdown
# RFL Implementation Plan — BlogFlow

## 1. Target Architecture (What Are We Building?)
One paragraph describing the final system. Include:
- Number of skill files total
- The orchestrator loop mechanics
- Rating scale and termination criteria
- How agents are dispatched
- How deferrals are tracked

## 2. File Inventory (Concrete List)

### Files to Create
| File | Lines | Purpose |
|------|-------|---------|
| ... | ~N | ... |

### Files to Modify
| File | Lines Changed | Purpose |
|------|---------------|---------|
| ... | +N/-N | ... |

## 3. Implementation Sequence (What to Build First)

### Phase 1 — Foundation (blocking)
1. ...
2. ...

### Phase 2 — Loop (required for merge readiness)
...

### Phase 3 — Maturity (nice-to-have)
...

## 4. Agent-to-Checklist Wiring
For each of BlogFlow's 11 agents, list which checklists fire when their file patterns match:

| Agent | When Files Matched | Checklists Fired |
|-------|-------------------|------------------|
| Cloud-Native Systems Engineer | *.go, cmd/**, internal/** | 03, 03a, 05, 04a |
| ... | ... | ... |

## 5. Deferral Tracking Mechanism
- SQL table schema for `deferrals` table
- GitHub issue creation format (marker, labels, protected-domain)
- Session restart reconciliation from GitHub
- What types of findings CANNOT be deferred

## 6. Estimated Total Lines of New Code
| Phase | Files | Total Lines |
|-------|-------|-------------|
| ... | ... | ~N |
| **Total** | | **~N** |

## 7. Risks and Mitigations
| Risk | Mitigation |
|------|------------|
| ... | ... |
```

### Constraints
- Read `docs/engineering/design/rfl-handoff-gap-analysis.md` first — it has the gap details
- Read `.github/skills/review-pr/*` — we've already created agent-map.md, checklist-map.md, and rating-rubric.md to help you
- All 10 sections above must be present — no skips
- Every item must be concrete — no vague "improve review process" items
- Use BlogFlow's **actual** 11 agents from `docs/persona/agents/`
- Use BlogFlow's **actual** directory structure (internal/config, internal/overlayfs, etc.)
- Include line count estimates for each planned file
- The plan must be implementable by a dev or skill agent

### Do Not
- Do not write code — produce a plan only
- Do not be vague — every item must have a concrete file path and purpose
- Do not skip any of the 10 sections
- Do not produce a 1-page summary — this must be comprehensive

---

*This prompt was generated for a new session that will build BlogFlow's RFL skill framework, filling the gap identified between our current manual checklist system and gamegrid's production review-fix-loop system.*
