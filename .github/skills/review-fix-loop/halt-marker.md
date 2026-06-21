# Halt Marker — BlogFlow RFL

Defines halt marker and halt-closure comment formats. Referenced by sections 1.6, 6.6(8) of review-fix-loop/SKILL.md.

## Halt Marker (posted by the system)

```html
<!-- blogflow-rfl-halt pr=42 head=abc123 reason=single-line-slug -->
```

| Field | Description |
|-------|------------|
| pr | PR number (integer) |
| head | HEAD SHA that triggered the halt |
| reason | Short slug: ci-red, ci-stuck, sha-drift, composition-invalid |

Used to signal that the prior RFL session could not declare termination (CI red, stuck checks, post-fix SHA drift, invalid council composition).

## Halt-Closure Comment (posted by follow-up RFL or human)

```html
<!-- blogflow-rfl-halt-resolved pr=42 halt_head=original_hash resolution_head=new_hash evidence=ci-passing -->
```

| Field | Description |
|-------|------------|
| pr | PR number (integer) |
| halt_head | Original HEAD from the halt marker |
| resolution_head | HEAD where the CI now passes |
| evidence | Single word: ci-passing, human-ack, ci-fix |

Posted when the issue is resolved.

## Halt Marker Query

At loop start (section 1.6), always query for ALL open halts on the PR:

```bash
gh pr view $PR_NUM --json comments \
  --jq '.comments[] | select(.body | contains("<!-- blogflow-rfl-halt"))'
```

Open halt without matching closure → hard stop.

---

*Source: review-fix-loop/SKILL.md sections 1.6, 6.6(8). Extracted for composability.*
