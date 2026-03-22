# BlogFlow Agent Roster

BlogFlow uses a research-backed agent system for AI-assisted development. Each agent has a canonical specification (this directory) and platform-specific wrappers:

- `.github/agents/*.agent.md` — GitHub Copilot custom agent wrappers
- `.claude/agents/*.md` — Claude Code agent wrappers

Wrappers are lightweight and point back to their canonical spec here. **If a wrapper and its canonical spec drift, the canonical spec wins.**

## Agents

| Agent | File | Domain |
|---|---|---|
| Cloud-Native Distributed Systems Architect | [cloud-native-distributed-systems-architect-agent.md](cloud-native-distributed-systems-architect-agent.md) | System architecture, overlay FS, content pipeline topology |
| Cloud-Native Systems Engineer | [cloud-native-systems-engineer-agent.md](cloud-native-systems-engineer-agent.md) | Go services, go-git, goldmark, HTTP server |
| Cloud-Native Site Reliability Engineer | [cloud-native-site-reliability-engineer-agent.md](cloud-native-site-reliability-engineer-agent.md) | Container health, webhook reliability, cache SLOs |
| Cloud-Native Security SME | [cloud-native-security-sme-agent.md](cloud-native-security-sme-agent.md) | Distroless hardening, HMAC webhooks, deploy keys |
| Cloud-Native Front-End Engineer | [cloud-native-front-end-engineer-agent.md](cloud-native-front-end-engineer-agent.md) | Default theme, templates, CSS, accessibility |
| Technical Writer | [technical-writer-agent.md](technical-writer-agent.md) | Documentation, tutorials, quick-start guides |
| Product Manager | [product-manager-agent.md](product-manager-agent.md) | Feature roadmap, content workflow UX |
| Program Manager | [program-manager-agent.md](program-manager-agent.md) | Phase tracking, cross-repo coordination |
| Solutions Engineer | [solutions-engineer-developer-success-architect-agent.md](solutions-engineer-developer-success-architect-agent.md) | Quick-start experience, deployment guides |
| Privacy & Compliance Lead | [privacy-compliance-grc-lead-agent.md](privacy-compliance-grc-lead-agent.md) | GDPR, data retention, cookie compliance |

## Design Principles

1. **Source-of-truth model**: Canonical specs define the agent's full mission, decision heuristics, and expected outputs. Wrappers are thin references.
2. **BlogFlow-grounded**: Every agent understands BlogFlow's architecture — overlay FS, content pipeline, go-git, distroless, gitflow content promotion.
3. **Delegation rules**: Each agent knows when to hand off to sibling agents.
4. **No domain bleed**: Agents are focused on blog engine development, not other domains.
