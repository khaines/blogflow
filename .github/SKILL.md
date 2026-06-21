# BlogFlow RFL Skill Index

This directory contains the BlogFlow Pull Request Review skill framework.

## Skills

| Skill | Purpose |
|-------|---------|
| [`.github/skills/review-pr/SKILL.md`](./skills/review-pr/SKILL.md) | Single-pass review engine with multi-model council |
| [`.github/skills/review-pr/agent-map.md`](./skills/review-pr/agent-map.md) | 10 BlogFlow agents → file pattern mapping |
| [`.github/skills/review-pr/checklist-map.md`](./skills/review-pr/checklist-map.md) | 14 engineering checklists → file pattern mapping |
| [`.github/skills/review-pr/rating-rubric.md`](./skills/review-pr/rating-rubric.md) | 1-5 rating scale, severity, consensus scoring |
| [`.github/skills/review-fix-loop/SKILL.md`](./skills/review-fix-loop/SKILL.md) | Iterative review-fix-loop orchestrator |
| [`.github/skills/review-fix-loop/dismissal-rules.md`](./skills/review-fix-loop/dismissal-rules.md) | Finding dismissal logic (fix / dismiss / defer) |
| [`.github/skills/review-fix-loop/deferral-gate.md`](./skills/review-fix-loop/deferral-gate.md) | Deferral tracking: SQL schema + GitHub issue filing |
| [`.github/skills/review-fix-loop/ci-verification.md`](./skills/review-fix-loop/ci-verification.md) | CI status verification via branch-protection API |
| [`.github/skills/review-fix-loop/halt-marker.md`](./skills/review-fix-loop/halt-marker.md) | Halt marker and closure comment formats |

## Invoking RFL

To run the full Review-Fix Loop on a PR:

The orchestrator automatically:
1. Identifies the PR (by number or current branch)
2. Runs multi-model council review (review-pr SKILL.md)
3. Classifies, fixes, and re-reviews up to 5 rounds
4. Validates CI, composition, and deferral gate
5. Posts final report + verdict

## Architecture

```
review-pr/            Single-pass review engine (evaluation)
├── SKILL.md          8-phase review pipeline
├── agent-map.md      10 agents to file patterns
├── checklist-map.md  14 checklists to file patterns
└── rating-rubric.md  1-5 scale + consensus scoring

review-fix-loop/      Iterative orchestrator (control)
├── SKILL.md          Loop mechanic (sections 1 to 6.6)
├── dismissal-rules.md Fix/dismiss/defer logic
├── deferral-gate.md  Deferral tracking (companion)
├── ci-verification.md CI status verification (companion)
└── halt-marker.md    Halt marker formats (companion)
```

---

*BlogFlow RFL skill framework. Built to match GameGrid production-reference RFL system.*
