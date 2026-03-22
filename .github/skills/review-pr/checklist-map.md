# Checklist Map — PR Review Skill

This file maps file patterns to engineering checklists so the `review-pr` skill knows which checklists to apply when reviewing a pull request. The skill reads this map, matches changed files against the patterns below, and loads the corresponding checklists from `docs/engineering/checklists/`.

---

## Primary Mapping

| File Pattern | Checklists | Notes |
|---|---|---|
| **Go source code** | | |
| `*.go` | 03, 03a | All Go files get coding conventions + Go standards |
| `*.go` (in `cmd/`, `internal/`) | 03, 03a, 02, 04 | Component code also gets microservice + testing checklists |
| `*_test.go` | 03, 03a, 04, 04a | Unit test files get unit-testing checklist |
| `*_integration_test.go`, `test/integration/**` | 03, 03a, 04, 04b | Integration tests get integration-testing checklist |
| `*.go` with logging calls | 03, 03a, 09a | Add logging checklist when log statements are present |
| `*.go` with metrics instrumentation | 03, 03a, 09b | Add metrics checklist when metrics code is present |
| `*.go` with tracing spans | 03, 03a, 09c | Add tracing checklist when tracing code is present |
| `*.go` hot-path / algorithmic code | 03, 03a, 08 | Add performance checklist for perf-sensitive code |
| **Templates & Themes** | | |
| `*.html` | 03 | HTML templates reviewed with coding conventions |
| `*.css` | 03 | Stylesheets reviewed with coding conventions |
| `defaults/templates/**` | 03 | Default theme templates |
| `defaults/static/**` | 03 | Default theme static assets |
| **Infrastructure** | | |
| `Dockerfile*`, `docker-compose*` | 10, 05 | Container definitions: runtime env + container security |
| `deploy/**` | 10 | Deployment manifests |
| `**/helm/**` | 10 | Helm charts |
| `**/*.tf`, `**/*.tfvars` | 10, 13 | Terraform files: runtime env + IaC |
| `**/pulumi/**` | 10, 13 | Pulumi files: runtime env + IaC |
| **Auth / Security** | | |
| `**/auth/**` | 05, 07 | **CRITICAL** — always apply security + privacy |
| `**/security/**` | 05 | **CRITICAL** — always apply security |
| `**/webhook/**` | 05 | **CRITICAL** — always apply security (signature verification) |
| `**/gitops/**` | 05 | **CRITICAL** — always apply security (git credential handling) |
| **User data handling** | | |
| `**/user/**`, `**/profile/**`, `**/account/**` | 07, 05 | Privacy + security for user-facing data |
| **Observability** | | |
| `**/monitoring/**`, `**/telemetry/**`, `**/observability/**` | 09 | Telemetry overview for observability code |
| Above paths with logging changes | 09, 09a | Add logging checklist |
| Above paths with metrics changes | 09, 09b | Add metrics checklist |
| Above paths with tracing changes | 09, 09c | Add tracing checklist |
| **Documentation** | | |
| `**/*.md` | 11, markdown-style-guide | Any Markdown file in any directory gets documentation + style checklists |
| **Architecture** | | |
| `docs/engineering/adr/*` | 01 | Architecture decision records |
| New component directories | 01, 02 | Architecture + microservice for new components |
| **IaC** | | |
| `**/terraform/**` | 13, 10 | IaC + runtime environment |
| `**/pulumi/**` | 13, 10 | IaC + runtime environment |
| **CI/CD** | | |
| `.github/workflows/*` | 10, 05 | Runtime environment + security (secrets, permissions) |

### Checklist Number Key

| Number | Filename | Topic |
|---|---|---|
| 01 | `01-architecture-checklist.md` | Architecture patterns |
| 02 | `02-microservice-implementation-checklist.md` | Microservice design |
| 03 | `03-coding-conventions-checklist.md` | General coding conventions |
| 03a | `03a-go-coding-standards.md` | Go coding standards |
| 04 | `04-testing-checklist.md` | General testing |
| 04a | `04a-unit-testing-checklist.md` | Unit testing |
| 04b | `04b-integration-testing-checklist.md` | Integration testing |
| 05 | `05-security-checklist.md` | Security |
| 07 | `07-privacy-checklist.md` | Privacy / GDPR |
| 08 | `08-performance-checklist.md` | Performance |
| 09 | `09-telemetry-observability-checklist.md` | Observability overview |
| 09a | `09a-logging-checklist.md` | Logging |
| 09b | `09b-metrics-checklist.md` | Metrics |
| 09c | `09c-distributed-tracing-checklist.md` | Distributed tracing |
| 10 | `10-runtime-environment-checklist.md` | Runtime / containers / K8s |
| 11 | `11-documentation-support-checklist.md` | Documentation |
| 13 | `13-infrastructure-as-code-checklist.md` | Infrastructure as Code |
| — | `markdown-style-guide-checklist.md` | Markdown formatting |

> **Note**: Not all checklists listed above may exist yet. They will be created in a future phase. If a checklist file is not found, skip it and note the gap in the review output.

---

## Universal Checklists

These checklists apply to **every PR** regardless of which files changed:

- **03 — Coding Conventions** applies to all code files. Even if the Go-specific checklist (03a) also applies, the general conventions checklist is always included.
- **05 — Security** receives a lightweight scan on all changes. The full security checklist is applied only when security-sensitive patterns match (auth, webhook, secrets, CI/CD), but a baseline security review is part of every PR.

---

## Checklist Priority

Priority determines whether a checklist can be deprioritized when a PR touches many areas. Higher priority checklists are never skipped.

| Priority | Checklists | Rule |
|---|---|---|
| **CRITICAL** | 05 (Security) | Never skipped when file patterns match. Blocking issues from these checklists must be resolved before merge. |
| **HIGH** | 03a (Go), 04 (Testing), 04a (Unit Testing), 04b (Integration Testing) | Applied to all matching code changes. Findings are expected to be addressed. |
| **STANDARD** | 01 (Architecture), 02 (Microservice), 07 (Privacy), 08 (Performance), 09/09a/09b/09c (Observability) | Applied when file patterns match. Findings are recommendations; author discretion on edge cases. |
| **SUPPLEMENTARY** | 11 (Documentation), markdown-style-guide | Applied to documentation files. Non-blocking unless docs are factually incorrect. |

---

## Loading Instructions

When loading checklists, read the full file from `docs/engineering/checklists/{filename}`. Each checklist is a Markdown file with structured sections. Focus on checklist items relevant to the specific changes in the PR, not every item in the checklist.
