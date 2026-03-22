# Implement Work Item — Component-to-Agent Mapping

This file maps `component:*` issue labels to the specialist agent personas responsible for implementation. The `implement-work-item` skill consults this map to determine which agent writes the code, what language to use, and which checklists to load.

---

## How It Works

1. Read the issue's `component:*` label.
2. Match against the **Component → Agent** table below.
3. Load the agent's coding standards, checklists, and reference docs.
4. If no `component:*` label matches, use the **Fallback Rules** at the bottom.

---

## Component → Agent Mapping

### Core Server

| Component Label | Primary Agent | Secondary Agent | Language | Checklists |
|-----------------|---------------|-----------------|----------|------------|
| `component:server` | `cloud-native-systems-engineer` | — | Go | 03a, 04a, 04b, 05, 09a, 09b, 09c |
| `component:config` | `cloud-native-systems-engineer` | — | Go | 03a, 04a, 04b, 05 |
| `component:cli` | `cloud-native-systems-engineer` | — | Go | 03a, 04a, 04b |
| `component:cache` | `cloud-native-systems-engineer` | — | Go | 03a, 04a, 04b, 05, 08 |

### Content Pipeline & Rendering

| Component Label | Primary Agent | Secondary Agent | Language | Checklists |
|-----------------|---------------|-----------------|----------|------------|
| `component:content-pipeline` | `cloud-native-systems-engineer` | — | Go | 03a, 04a, 04b, 05 |
| `component:theme-engine` | `cloud-native-systems-engineer` | `cloud-native-front-end-engineer` | Go | 03a, 04a, 04b, 05 |
| `component:default-theme` | `cloud-native-front-end-engineer` | `cloud-native-systems-engineer` | Go + HTML/CSS | 03a, 04a, 04b |
| `component:feed` | `cloud-native-systems-engineer` | — | Go | 03a, 04a, 04b, 05 |
| `component:sitemap` | `cloud-native-systems-engineer` | — | Go | 03a, 04a, 04b |

### Storage & Sync

| Component Label | Primary Agent | Secondary Agent | Language | Checklists |
|-----------------|---------------|-----------------|----------|------------|
| `component:overlay-fs` | `cloud-native-systems-engineer` | `cloud-native-distributed-systems-architect` | Go | 03a, 04a, 04b, 05, 08 |
| `component:git-sync` | `cloud-native-systems-engineer` | `cloud-native-security-sme` | Go | 03a, 04a, 04b, 05, 09a, 09b |
| `component:git-auth` | `cloud-native-systems-engineer` | `cloud-native-security-sme` | Go | 03a, 04a, 04b, 05, 09a, 09b |
| `component:file-watcher` | `cloud-native-systems-engineer` | — | Go | 03a, 04a, 04b, 05 |

### Integrations & Webhooks

| Component Label | Primary Agent | Secondary Agent | Language | Checklists |
|-----------------|---------------|-----------------|----------|------------|
| `component:webhook` | `cloud-native-systems-engineer` | `cloud-native-security-sme` | Go | 03a, 04a, 04b, 05, 09a, 09b |

### Infrastructure & Cross-Cutting

| Label | Primary Agent | Secondary Agent | Language | Checklists |
|-------|---------------|-----------------|----------|------------|
| `type:infra` | `cloud-native-site-reliability-engineer` | `cloud-native-distributed-systems-architect` | Go + YAML | 03a, 10, 13 |
| `type:docs` | `technical-writer` | — | Markdown | 11 |

---

## Checklist Reference Key

| Number | Filename | Topic |
|--------|----------|-------|
| 03a | `03a-go-coding-standards.md` | Go coding standards |
| 04a | `04a-unit-testing-checklist.md` | Unit testing |
| 04b | `04b-integration-testing-checklist.md` | Integration testing |
| 05 | `05-security-checklist.md` | Security |
| 08 | `08-performance-checklist.md` | Performance |
| 09a | `09a-logging-checklist.md` | Logging |
| 09b | `09b-metrics-checklist.md` | Metrics |
| 09c | `09c-distributed-tracing-checklist.md` | Distributed tracing |
| 10 | `10-runtime-environment-checklist.md` | Runtime / containers / K8s |
| 11 | `11-documentation-support-checklist.md` | Documentation |
| 13 | `13-infrastructure-as-code-checklist.md` | Infrastructure as Code |

---

## Fallback Rules

When the issue's `component:*` label does not match any entry above:

1. **Infer from design doc file patterns**: Match the design doc's API surface section within §2 and the referenced file paths against the review-pr `agent-map.md` patterns.
2. **Infer from issue type**:
   - `type:bug` on a Go component → `cloud-native-systems-engineer`
   - `type:bug` on a theme/template component → `cloud-native-front-end-engineer`
   - `type:spike` → `cloud-native-distributed-systems-architect` (architecture exploration)
   - `type:adr` → `cloud-native-distributed-systems-architect`
3. **Last resort**: Ask the user which agent persona to use.

---

## Secondary Agent Rules

Secondary agents are dispatched when the implementation touches their domain:

- **Security SME** (`cloud-native-security-sme`): Always involved when the component handles auth, secrets, git credentials, or webhook signatures. Reviews security-sensitive code after the primary agent implements it.
- **Distributed Systems Architect** (`cloud-native-distributed-systems-architect`): Involved when changes affect the overlay filesystem, content integrity boundaries, or introduce new architectural patterns.
- **Front-end Engineer** (`cloud-native-front-end-engineer`): Involved when changes affect HTML templates, CSS, or theme rendering. The primary agent handles Go logic; the front-end agent handles template and styling concerns.

Secondary agents participate in implementation only when the issue explicitly requires their domain. Otherwise, they participate during the Phase 8 review-fix-test loop via the review-pr agent-map.
