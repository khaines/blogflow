# Design Document — Section-to-Agent Mapping

This file maps each section of the design document template (`docs/engineering/design/000-template.md`) to the specialist agent persona responsible for generating its content. The `design-doc` skill consults this map to dispatch the right agent for each section.

---

## Section Ownership

| Section | Title | Primary Agent | Advisory Agents | Phase |
|---------|-------|---------------|-----------------|-------|
| §1 | Overview | `cloud-native-distributed-systems-architect` | — | 2 |
| §2 | Logical Architecture | `cloud-native-distributed-systems-architect` | `cloud-native-systems-engineer` | 2 |
| §3 | Functional Test Scenarios | `cloud-native-distributed-systems-architect` | `cloud-native-systems-engineer` | 3 |
| §4 | Performance | `cloud-native-distributed-systems-architect` | `cloud-native-site-reliability-engineer` | 3 |
| §5 | Security | `cloud-native-security-sme` | `privacy-compliance-grc-lead` | 4 |
| §6 | Threat Model | `cloud-native-security-sme` | `cloud-native-distributed-systems-architect` | 4 |
| §7 | Observability | `cloud-native-site-reliability-engineer` | — | 5 |
| §8 | Rollout & Risk | `cloud-native-site-reliability-engineer` | `cloud-native-distributed-systems-architect` | 5 |
| §9 | Open Questions & Decisions | `cloud-native-distributed-systems-architect` | All participating agents | 2–5 |
| §10 | References | `design-doc` skill (automated) | — | 6 |

---

## Ownership Rules

1. **Primary agent** generates the section content. The section should reflect that agent's expertise, vocabulary, and decision heuristics.
2. **Advisory agents** are listed for reference context. During initial generation (Phases 2–5), the primary agent handles all content. Advisory agents participate during the Phase 8 review-fix loop, where the `review-pr` skill may route findings to them for specialist review. Advisory agents may also be explicitly consulted when the primary agent encounters a question in the advisory agent's domain — for example, the Architect may consult the Systems Engineer on API contract details for §2.5.
3. **§9 (Open Questions)** is populated incrementally by every agent. Each agent adds questions surfaced during their section generation.
4. **§10 (References)** is assembled automatically by the skill during Phase 6 (Assembly & Validation) from all docs consulted during generation.

---

## Agent Persona References

| Agent | Canonical Spec | Research |
|-------|---------------|----------|
| `cloud-native-distributed-systems-architect` | `docs/persona/agents/cloud-native-distributed-systems-architect-agent.md` | `docs/persona/research/cloud-native-distributed-systems-architect.md` |
| `cloud-native-systems-engineer` | `docs/persona/agents/cloud-native-systems-engineer-agent.md` | `docs/persona/research/cloud-native-systems-engineer.md` |
| `cloud-native-security-sme` | `docs/persona/agents/cloud-native-security-sme-agent.md` | `docs/persona/research/cloud-native-security-sme.md` |
| `cloud-native-site-reliability-engineer` | `docs/persona/agents/cloud-native-site-reliability-engineer-agent.md` | `docs/persona/research/cloud-native-site-reliability-engineer.md` |
| `cloud-native-front-end-engineer` | `docs/persona/agents/cloud-native-front-end-engineer-agent.md` | `docs/persona/research/cloud-native-front-end-engineer.md` |
| `privacy-compliance-grc-lead` | `docs/persona/agents/privacy-compliance-grc-lead-agent.md` | `docs/persona/research/privacy-compliance-grc-lead.md` |
| `technical-writer` | `docs/persona/agents/technical-writer-agent.md` | `docs/persona/research/technical-writer.md` |
| `product-manager` | `docs/persona/agents/product-manager-agent.md` | `docs/persona/research/product-manager.md` |
| `program-manager` | `docs/persona/agents/program-manager-agent.md` | `docs/persona/research/program-manager.md` |
| `solutions-engineer-developer-success-architect` | `docs/persona/agents/solutions-engineer-developer-success-architect-agent.md` | `docs/persona/research/solutions-engineer-developer-success-architect.md` |
