# PR Review — Agent Mapping for BlogFlow

Maps file patterns to specialist agent personas. The `review-pr` skill matches changed files against these patterns and activates all matching agents.

---

## Agent → File Patterns

### cloud-native-distributed-systems-architect
**Domain:** System architecture, overlay FS design, content pipeline topology, multi-repo coordination, reliability/security/observability trade-offs.

| Pattern | Trigger |
|---------|---------|
| `docs/engineering/design/**` | Design docs and ADRs |
| `docs/engineering/adr/**` | Architecture Decision Records |
| `docs/engineering/research/**` | Architecture research |
| `*.md` with "architecture" or "topology" in filename | Architecture content |
| `internal/overlayfs/**` | Overlay filesystem changes |
| `internal/content/**` | Content pipeline topology changes |
| New service/component directories | Introduces new component |

### cloud-native-systems-engineer
**Domain:** Go services, go-git, goldmark, HTTP server, overlay FS internals, configuration system.

| Pattern | Trigger |
|---------|---------|
| `*.go` | All Go source code |
| `internal/config/**` | Configuration system changes |
| `internal/gitops/**` | Git sync, webhook, polling |
| `internal/overlayfs/**` | Overlay filesystem core |
| `internal/server/**` | HTTP server and routing |
| `go.mod`, `go.sum` | Go module dependency changes |
| `cmd/**` | CLI and entry-point changes |

### cloud-native-front-end-engineer
**Domain:** Default theme, templates, CSS, responsive design, accessibility, static assets.

| Pattern | Trigger |
|---------|---------|
| `defaults/templates/**` | Default template changes |
| `defaults/static/**` | Default CSS/JS/image changes |
| `*.html` | HTML template changes |
| `*.css` | Stylesheet changes |
| `*.yaml` in `defaults/` or `theme/` | Theme metadata changes |
| `internal/theme/**` | Theme engine changes |

### cloud-native-site-reliability-engineer
**Domain:** Container health, webhook reliability, cache SLOs, K8s manifests, CI/CD, rollout safety.

| Pattern | Trigger |
|---------|---------|
| `k8s/**`, `helm/**`, `deploy/**` | K8s and deploy manifests |
| `Dockerfile*` | Container definition changes |
| `.github/workflows/**` | CI/CD pipeline changes |
| `internal/otel/**` | Tracing/metrics instrumentation |
| `internal/server/server.go` | Server health, readiness, metrics |
| `internal/content/renderer.go` | Content pipeline performance |

### cloud-native-security-sme
**Domain:** Distroless hardening, HMAC-SHA256 webhook signing, deploy keys (SSH/PAT/GitHub App), secrets handling, CSP headers, path traversal prevention.

| Pattern | Trigger |
|---------|---------|
| `**/auth/**` | Authentication code |
| `**/security/**` | Security modules |
| `**/*secret*`, `**/*token*`, `**/*credential*` | Secrets and tokens |
| `**/middleware/**` | HTTP middleware |
| `internal/gitops/webhook.go` | Webhook security |
| `.env*` | Environment variable files |
| **/auth/**, **/credential* patterns | Authentication, authorization |
| `internal/overlayfs/**` | Path validation and traversal prevention |

### technical-writer
**Domain:** Content authoring docs, theme guide, API reference, gitflow workflow, changelog.

| Pattern | Trigger |
|---------|---------|
| `**/*.md` | All Markdown documentation |
| `docs/**` | Documentation directory |
| `CHANGELOG.md` | Changelog updates |
| `README.md` | README updates |
| `docs/engineering/design/**` | Design document changes |

### product-manager
**Domain:** Feature roadmap, UX trade-offs, blog content workflow, user expectations, prioritization.

| Pattern | Trigger |
|---------|---------|
| `docs/engineering/adr/**` | Architecture decisions (product impact) |
| `CHANGELOG.md` | Changelog content strategy |
| `docs/persona/agents/**` | Agent persona changes |
| `docs/**` | Documentation with user-facing implications |

### program-manager
**Domain:** Phase tracking, cross-repo coordination, execution governance, milestone delivery.

| Pattern | Trigger |
|---------|---------|
| `.github/workflows/**` | CI/CD pipeline coordination |
| `docs/engineering/design/**` | Design docs with multi-phase implications |
| `README.md` | Project overview updates |
| All PRs with files across 3+ domains | Cross-repo coordination needed |

### solutions-engineer
**Domain:** Quick-start, deployment guides, integration patterns, developer experience.

| Pattern | Trigger |
|---------|---------|
| `cmd/**` | CLI behavior changes |
| `docs/deployment-guide.md` | Deployment guide updates |
| `.github/skills/**` | Developer experience changes |
| `README.md` | Getting-started changes |
| `defaults/` | Zero-config experience changes |

### privacy-compliance-grc-lead
**Domain:** GDPR for blog content, data retention policies, cookie compliance, PII handling, secrets management.

| Pattern | Trigger |
|---------|---------|
| `**/user/**` | User data handling |
| `**/auth/**` | Authentication/authorization |
| `**/credential*` | Credential/secret handling |
| `internal/config/**` | Config containing user/policy data |
| `**/*.json`, `**/*.yaml` with "pii", "cookie", "gdpr" patterns | PII or compliance markers |

---

## Agent Priority / Dispatch Rules

| Rule | Action |
|------|--------|
| Security SME matches any `auth/`, `security/`, `*secret*`, `*token*`, `*credential*` pattern | **CRITICAL** — always active when patterns match |
| Multiple agents match same file | All activate; each reviews their domain |
| File matches no agent | Assign to `general-purpose` with `[no-agent-match]` annotation |
| Security SME + SRE both match a file | Dispatch both, but **merge** when both return 5/5 |

---

## Agent Dispatch Priority

1. **cloud-native-security-sme** — CRITICAL (never deprioritized)
2. **cloud-native-systems-engineer** — HIGH (Go/core always reviewed)
3. **cloud-native-site-reliability-engineer** — HIGH (operability always reviewed)
4. **cloud-native-distributed-systems-architect** — HIGH (architecture always reviewed)
5. **cloud-native-front-end-engineer** — STANDARD (theme/frontend when present)
6. **technical-writer** — SUPPLEMENTARY (docs when present)
7. **product-manager** — SUPPLEMENTARY (UX decisions when present)
8. **program-manager** — SUPPLEMENTARY (coordination when cross-repo)
9. **solutions-engineer** — SUPPLEMENTARY (DX when present)
10. **privacy-compliance-grc-lead** — CRITICAL (when user data or secrets present)

---

## Agent Prompt Template

Standard dispatch prompt for each agent:

You are acting as the **{agent_name}** specialist for BlogFlow.

## Findings to Address
**{finding.id}** — {severity}
- File: {agent} → {finding.file}:{finding.line}
- Finding: {finding.finding}
- Recommendation: {finding.recommendation}
- Consensus: {finding.consensus}

## Instructions
1. Read each file that needs changes.
2. Make precise, surgical fixes for this finding only.
3. Do NOT introduce new patterns or refactor beyond what's required.
4. Verify your change doesn't break surrounding code.
5. Confirm which findings were addressed and how.


