# Design Document — Checklist Cross-References

This file maps each section of the design document template (`docs/engineering/design/000-template.md`) to the engineering checklists that should be consulted during generation. The `design-doc` skill reads the relevant checklists to ensure design decisions align with BlogFlow's engineering standards.

---

## Section-to-Checklist Mapping

| Section | Title | Checklists | Purpose |
|---------|-------|------------|---------|
| §1 | Overview | — | No checklist — populated from issue and requirements |
| §2 | Logical Architecture | 01, 02 | Architecture patterns, microservice design |
| §2.4 | Data Model / Schema | 02 | Schema design conventions |
| §2.5 | API Surface | 02, 03a | API contract conventions, Go-specific standards |
| §3 | Functional Test Scenarios | 04, 04a, 04b | General testing, unit testing, integration testing |
| §4 | Performance | 08 | Performance checklist |
| §5 | Security | 05, 07 | Security checklist, privacy checklist |
| §6 | Threat Model | 05 | Security checklist (threat modelling section) |
| §7.1 | Logging Strategy | 09a | Logging checklist |
| §7.2 | Metrics & Dashboards | 09b | Metrics checklist |
| §7.3 | Distributed Tracing | 09c | Distributed tracing checklist |
| §7.4 | Alerting Rules | 09b | Metrics checklist (alerting section) |
| §8.1 | Rollout Strategy | 10 | Runtime environment checklist |
| §8.5 | Launch Checklist | 01, 02, 05, 10, 13 | Architecture, microservice, security, runtime, IaC |
| §9 | Open Questions | — | No checklist — populated during generation |
| §10 | References | — | No checklist — assembled from consulted docs |

---

## Checklist Reference Key

| Number | Filename | Topic |
|--------|----------|-------|
| 01 | `01-architecture-checklist.md` | Architecture patterns |
| 02 | `02-microservice-implementation-checklist.md` | Microservice design |
| 03a | `03a-go-coding-standards.md` | Go coding standards |
| 04 | `04-testing-checklist.md` | General testing |
| 04a | `04a-unit-testing-checklist.md` | Unit testing |
| 04b | `04b-integration-testing-checklist.md` | Integration testing |
| 05 | `05-security-checklist.md` | Security |
| 07 | `07-privacy-checklist.md` | Privacy / GDPR |
| 08 | `08-performance-checklist.md` | Performance |
| 09a | `09a-logging-checklist.md` | Logging |
| 09b | `09b-metrics-checklist.md` | Metrics |
| 09c | `09c-distributed-tracing-checklist.md` | Distributed tracing |
| 10 | `10-runtime-environment-checklist.md` | Runtime / containers / K8s |
| 13 | `13-infrastructure-as-code-checklist.md` | Infrastructure as Code |

> **Note**: Not all checklists listed above may exist yet. They will be created in a future phase. If a checklist file is not found, skip it and note the gap.

---

## Best-Practices Cross-References

In addition to checklists, the following best-practices docs should be loaded for context during section generation:

| Section | Best-Practices Doc | Purpose |
|---------|-------------------|---------|
| §2 | `01-architecture.md`, `02-microservices.md` | Architectural guardrails |
| §3 | `04-testing.md` | Testing philosophy and standards |
| §4 | `08-performance.md` | Performance standards and budgets |
| §5, §6 | `05-security.md`, `07-privacy.md` | Security and privacy standards |
| §7 | `09-telemetry-observability.md` | Observability standards |
| §8 | `02-microservices.md` | Deployment and runtime standards |

---

## Loading Instructions

When loading checklists, read the full file from `docs/engineering/checklists/{filename}`. Focus on checklist items relevant to the component being designed, not every item in the checklist. Similarly, load best-practices docs for reference context — the design doc should **reference** these documents, not duplicate their content.
