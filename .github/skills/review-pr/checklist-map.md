# Checklist Map — BlogFlow PR Review

Maps file patterns to engineering checklists. Each checklist is a Markdown file in `docs/engineering/checklists/`. Read the full checklist file when its patterns match.

---

## File Pattern → Checklist Mappings

### Go Source Code (`*.go`)
| Patterns | Checklists |
|----------|-----------|
| `*.go` | 03 (Coding Conventions), 03a (Go Standards) |
| `internal/*` (non-test) | 03, 03a, 05 (Security), 09 (Obs) |
| `*_test.go` | 03, 03a, 04 (Testing), 04a (Unit Testing) |
| `*_integration_test.go` | 03, 03a, 04, 04b (Integration Testing) |
| Go files with `slog.` calls | 03, 03a, 09a (Logging) |
| Go files with `metrics.` calls | 03, 03a, 09b (Metrics) |
| Go files with tracing calls | 03, 03a, 09c (Distributed Tracing) |
| Hot-path code (`Server`, `Handler`, `Render`, `resolve`) | 03, 03a, 08 (Performance) |

### TypeScript / React / HTML Templates (`*.ts`, `*.tsx`, `*.html`)
| Patterns | Checklists |
|----------|-----------|
| `defaults/templates/*.html` | 03, 03b (TS/React Standards), 04a |
| `*.css` | 03, 03b, 05 (Security - CSP/XSS) |
| `*.html` template files | 03, 03b, 05 (XSS prevention) |
| Theme `*.yaml` metadata | 03 |

### Infrastructure & Deployment (`Dockerfile*`, `k8s/**`, `helm/**`)
| Patterns | Checklists |
|----------|-----------|
| `Dockerfile*` | 10 (Runtime/Containers), 05 (Security) |
| `k8s/**`, `helm/**`, `deploy/**` | 10 (Runtime/Containers) |
| `docker-compose*` | 10 (Runtime/Containers) |
| `.github/workflows/**` | 10 (Runtime/Containers), 05 (Security) |
| `.golangci.yml` or `Makefile` | 10 (Runtime/Containers) |

### IaC (`*.tf`, `*.pulumi.*`, etc.)
| Patterns | Checklists |
|----------|-----------|
| `**/*.tf`, `**/*.tfvars` | 10 (Runtime/Containers), 13 (IaC) |
| `**/Pulumi.*`, `**/pulumi/**` | 10 (Runtime/Containers), 13 (IaC) |
| `**/opentofu/**` | 10 (Runtime/Containers), 13 (IaC) |

### Security-Sensitive Code
| Patterns | Checklists |
|----------|-----------|
| `**/auth/**`, `**/security/**`, `**/tenant/**` | **05 (Security) — CRITICAL** |
| `**/auth/**` | 07 (Privacy) |
| `**/secrets/**`, `**/*secret*`, `**/*token*` | 05 (Security), 09a (Logging - redaction!) |
| `internal/overlayfs/**` path validation | 05 (Security), 04a (Testing) |
| `internal/gitops/webhook.go` | 05 (Security), 04a (Testing) |

### Content Pipeline
| Patterns | Checklists |
|----------|-----------|
| `internal/content/**` | 03, 03a, 08 (Performance), 09 (Obs) |
| Frontmatter parsing | 03, 03a, 05 (Security) |
| Markdown rendering | 03, 03a, 08 (Performance) |

### Config / YAML Processing
| Patterns | Checklists |
|----------|-----------|
| `internal/config/**` | 03, 03a, 05 (Security - no secrets in YAML!), 09 (Obs) |
| Config file loading | 04a (Testing), 04b (Integration Testing) |
| YAML parsing with secrets | 05 (Security) |

### Documentation (`**/*.md`, `docs/**`)
| Patterns | Checklists |
|----------|-----------|
| `**/*.md` | 11 (Documentation Support) |
| `docs/engineering/adr/**` | 01 (Architecture) |
| `CHANGELOG.md` | 11 (Documentation) |
| `docs/engineering/design/**` | 01 (Architecture) |

### Observability / Telemetry (`**/otel/**`, `**/monitoring/**`)
| Patterns | Checklists |
|----------|-----------|
| `**/monitoring/**`, `**/telemetry/**` | 09 (Telemetry Overview) |
| Go files with `slog.` calls | 09a (Logging Checklist) |
| Go files with `prometheus.` calls | 09b (Metrics Checklist) |
| Go files with tracing spans | 09c (Distributed Tracing Checklist) |

---

## Checklist Number Key

| Number | Filename | Topic | Priority |
|--------|----------|-------|----------|
| 01 | `01-architecture-checklist.md` | Architecture patterns, ADRs | CRITICAL |
| 02 | `02-microservice-implementation.md` | Service boundaries, API contracts | HIGH |
| 03 | `03-coding-conventions.md` | Universal coding conventions | HIGH |
| 03a | `03a-go-coding-standards.md` | Go-specific conventions | HIGH |
| 03b | `03b-typescript-react-standards.md` | TS/React conventions | HIGH |
| 04 | `04-testing-checklist.md` | General testing standards | HIGH |
| 04a | `04a-unit-testing-checklist.md` | Unit testing standards | HIGH |
| 04b | `04b-integration-testing-checklist.md` | Integration testing standards | HIGH |
| 05 | `05-security-checklist.md` | Security standards | **CRITICAL** |
| 07 | `07-privacy-checklist.md` | GDPR / PII / data retention | HIGH |
| 08 | `08-performance-checklist.md` | Performance budgets, profiling | STANDARD |
| 09 | `09-telemetry-observability-overview.md` | Telemetry overview | STANDARD |
| 09a | `09a-logging-checklist.md` | Logging standards | STANDARD |
| 09b | `09b-metrics-checklist.md` | Metrics standards | STANDARD |
| 09c | `09c-distributed-tracing.md` | Distributed tracing | STANDARD |
| 10 | `10-runtime-environment-checklist.md` | Containers, K8s, CI/CD | HIGH |
| 11 | `11-documentation-support.md` | Documentation standards | SUPPLEMENTARY |
| 13 | `13-infrastructure-as-code.md` | Terraform / Pulumi standards | HIGH |
| 14 | `14-tenant-isolation-checklist.md` | Multi-tenant isolation | **CRITICAL** |

---

## Universal Checklists (Apply to Every PR)

1. **03 — Coding Conventions** — always, on all code files
2. **05 — Security** — lightweight scan on all changes; full check when security patterns match

---

## Checklist Priority

| Priority | Checklists | Rule |
|----------|-----------|------|
| **CRITICAL** | 05 (Security), 14 (Tenant) | Never deprioritized; blocking issues must be resolved before merge |
| **HIGH** | 01, 02, 03, 03a, 03b, 04, 04a, 04b, 10, 13 | Applied to all matching changes |
| **STANDARD** | 07, 08, 09/09a/09b/09c | Applied when patterns match; recommendations only |
| **SUPPLEMENTARY** | 11 (Documentation) | Non-blocking unless documentation is factually incorrect |

---

## Loading Instructions

When loading checklists, read the full file from `docs/engineering/checklists/{filename}`. Focus on checklist items relevant to the specific changes in the PR, not every item in the checklist.
