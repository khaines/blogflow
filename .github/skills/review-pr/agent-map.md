# PR Review — Agent Mapping Reference

This file maps file patterns and content signals to the specialist agent personas used during pull-request reviews. The `review-pr` skill consults this map to decide which agents should participate in a given review.

**How it works:**

1. For every changed file in a PR, match its path against the **File Pattern → Agent** table.
2. Apply the **Priority Rules** to decide which agents are included vs. advisory.
3. Run the **Content-Based Detection** heuristics for additional agent triggers that path patterns alone cannot capture.
4. Assemble the final reviewer set — every matched agent reviews the files that triggered it; the agent with the most file matches is designated the **primary** reviewer.

---

## 1. File Pattern → Agent Mapping

### cloud-native-systems-engineer

> Go services, HTTP handlers, content pipeline logic, configuration loading, backend diagnosability.

| Pattern | Priority | Notes |
|---|---|---|
| `*.go` | High | Go source files |
| `*_test.go` | High | Go test files |
| `go.mod`, `go.sum` | High | Go module dependency files |
| `cmd/**` | High | Go service entry-points |
| `internal/**` | High | Internal packages |

### cloud-native-front-end-engineer

> Semantic, accessible HTML templates, CSS theming, static assets, theme rendering.

| Pattern | Priority | Notes |
|---|---|---|
| `defaults/templates/**` | High | Default theme HTML templates |
| `defaults/static/**` | High | Default static assets (CSS, JS, images) |
| `*.html` | High | HTML template files |
| `*.css` | High | Stylesheets |
| `*.js` | High | JavaScript files |
| `**/theme*/**` | High | Theme-related directories |

### cloud-native-distributed-systems-architect

> Component boundaries, content pipeline topology, reliability / security / observability trade-offs.

| Pattern | Priority | Notes |
|---|---|---|
| `docs/engineering/adr/*` | Critical | Architecture Decision Records |
| `docs/engineering/design/*` | Critical | Design documents |
| `docs/engineering/research/*` | Critical | Architecture research documents |
| `*.md` with "architecture" in path | Critical | Architecture-related Markdown |
| _(new component)_ | Critical | Any file that introduces a new component — detected at review time |
| _(component-to-component changes)_ | Critical | Changes to inter-component communication patterns |

### cloud-native-security-sme

> Auth, credential handling, webhook signature verification, secrets management, secure delivery.

| Pattern | Priority | Notes |
|---|---|---|
| `**/auth/**` | Critical | Authentication code |
| `**/security/**` | Critical | Security modules |
| `**/crypto/**` | Critical | Cryptographic code |
| `**/*secret*` | Critical | Secrets handling |
| `**/*credential*` | Critical | Credential management |
| `**/*token*` | Critical | Token handling |
| `**/webhook/**` | Critical | Webhook signature verification |
| `.env*` | Critical | Environment variable files |
| `**/gitops/**` | Critical | Git operations with auth |

### cloud-native-site-reliability-engineer

> SLOs, observability, rollout safety, incident response, recovery readiness, toil reduction.

| Pattern | Priority | Notes |
|---|---|---|
| `deploy/**` | High | Deployment manifests |
| `Dockerfile*` | High | Container definitions |
| `docker-compose*` | High | Compose files |
| `.github/workflows/*` | High | CI/CD workflow definitions |
| `**/monitoring/**` | High | Monitoring configuration |
| `**/helm/**` | High | Helm charts |
| `**/terraform/**` | High | IaC — Terraform (also triggers architect) |
| `**/pulumi/**` | High | IaC — Pulumi (also triggers architect) |

### technical-writer

> Documentation architecture, docs-as-code, tutorials, troubleshooting guides.

| Pattern | Priority | Notes |
|---|---|---|
| `docs/**/*.md` | Standard | Documentation files (except ADRs/design → architect) |
| `**/*.md` | Standard | Any Markdown file in any directory |
| `*.md` (root) | Standard | Root-level docs: README, CONTRIBUTING, etc. |
| `CHANGELOG*` | Standard | Changelog |

### product-manager

> Product direction, user-value framing, roadmap trade-offs, outcome-based recommendations.

| Pattern | Priority | Notes |
|---|---|---|
| `docs/product/**` | Standard | Product strategy documents |

### program-manager

> Multi-workstream planning, dependency mapping, execution control, risk management.

| Pattern | Priority | Notes |
|---|---|---|
| `docs/product/*execution*` | Standard | Execution plan documents |
| `**/roadmap*` | Standard | Roadmap documents |

### solutions-engineer-developer-success-architect

> Onboarding, quickstart guides, getting-started flows, developer success.

| Pattern | Priority | Notes |
|---|---|---|
| `**/quickstart*/**` | Standard | Quick-start guides |
| `**/getting-started*/**` | Standard | Getting-started guides |
| `**/onboarding/**` | Standard | Onboarding flows |

### privacy-compliance-grc-lead

> GDPR, privacy controls, compliance operations, audit evidence.

| Pattern | Priority | Notes |
|---|---|---|
| `**/compliance/**` | High | Compliance code and config |
| `**/gdpr/**` | High | GDPR-specific code |
| `**/privacy/**` | High | Privacy controls (also triggers security SME) |
| `**/consent/**` | High | Consent management |

---

## 2. Priority Rules

### Priority Levels

| Level | Behaviour | Agents |
|---|---|---|
| **Critical** | **Always** included when patterns match, regardless of how many other agents are active. | `cloud-native-security-sme`, `cloud-native-distributed-systems-architect` |
| **High** | Included when their patterns match. These are the domain-specialist agents. | `cloud-native-systems-engineer`, `cloud-native-front-end-engineer`, `cloud-native-site-reliability-engineer`, `privacy-compliance-grc-lead` |
| **Standard** | Included only when the file falls within their **primary** domain — i.e., no higher-priority agent also claims the file, or the file belongs squarely to the Standard agent's area. | `technical-writer`, `product-manager`, `program-manager`, `solutions-engineer-developer-success-architect` |

### Multi-Agent Resolution

1. **All matching agents review.** If a file matches patterns for more than one agent, every matching agent is added to the reviewer set for that file.
2. **Primary agent designation.** The agent with the most file matches across the entire PR is designated the primary reviewer and provides the top-level summary.
3. **Tie-breaking.** When two agents match the same number of files, prefer the agent with the higher priority level (Critical > High > Standard).
4. **Cross-triggers.** Some patterns explicitly trigger a second agent (noted in the tables above). For example:
   - `**/terraform/**` → `cloud-native-site-reliability-engineer` **+** `cloud-native-distributed-systems-architect`
   - `**/privacy/**` → `privacy-compliance-grc-lead` **+** `cloud-native-security-sme`

---

## 3. Content-Based Detection

File-path patterns catch most cases, but some agent assignments require inspecting the **content** of changed files. Apply these heuristics after pattern matching.

### Import & Dependency Signals

| Signal | Example | Triggers Agent |
|---|---|---|
| Security / crypto imports | `import "crypto/..."`, `import "golang.org/x/crypto"` | `cloud-native-security-sme` |
| JWT / auth libraries | `jwt.Parse`, `bcrypt.CompareHashAndPassword` | `cloud-native-security-sme` |
| Kubernetes client usage | `import "k8s.io/client-go/..."` | `cloud-native-site-reliability-engineer` |
| Git library imports | `import "github.com/go-git/go-git/v5"` | `cloud-native-systems-engineer` |
| Compliance / GDPR annotations | GDPR consent flags, privacy-related constants | `privacy-compliance-grc-lead` |
| Template engine imports | `import "html/template"`, `import "text/template"` | `cloud-native-front-end-engineer` |

### Operation Signals

| Signal | Example | Triggers Agent |
|---|---|---|
| Database queries / migrations | `CREATE TABLE`, `ALTER TABLE`, `sql.Open`, ORM model changes | `cloud-native-systems-engineer` + `cloud-native-distributed-systems-architect` |
| Secret or credential construction | Hardcoded tokens, API-key literals, `os.Getenv("*SECRET*")` | `cloud-native-security-sme` |
| New component registration | Adding a new entry in a component registry, new `main.go` under `cmd/` | `cloud-native-distributed-systems-architect` |
| Helm value / chart changes | `values.yaml` modifications, new chart templates | `cloud-native-site-reliability-engineer` |
| Webhook signature verification | HMAC computation, signature comparison | `cloud-native-security-sme` |
| File system operations | `os.OpenFile`, `os.MkdirAll`, overlay-fs layer manipulation | `cloud-native-systems-engineer` |

### Notes

- Content-based detection is **additive** — it adds agents to the reviewer set; it never removes an agent that was already matched by path.
- When content detection triggers an agent that was not matched by path, annotate the review with the specific signal that caused the addition so reviewers understand why the agent was included.
- For large PRs (50+ changed files), prioritise content scanning on files that did **not** already match a path pattern to keep review latency manageable.
