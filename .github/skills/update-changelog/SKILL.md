---
name: update-changelog
description: >-
  Updates CHANGELOG.md with entries for the current PR using the Prometheus/Cortex ecosystem convention:
  prefixed tags ([FEATURE], [ENHANCEMENT], [BUGFIX], [CHANGE]), component prefixes,
  and inline PR references. Designed to be invoked as a final step in the implement-work-item
  or review-fix-loop skills before opening/updating a PR.
---

# Update Changelog Skill — Orchestration Instructions

Append changelog entries to `CHANGELOG.md` for the current PR. Read the existing changelog
before making changes to match the established style.

---

## Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `changelog_path` | `CHANGELOG.md` | Path to the changelog file |
| `section` | `main / unreleased` | Section to append entries under |
| `auto_detect_tag` | true | Infer the tag from issue labels and change type |

---

## Format Convention (Prometheus/Cortex Ecosystem Style)

Every entry is a single bullet using this format:

```
* [TAG] Component: Description of the change. #PR_NUMBER
```

### Tags

| Tag | When to use |
|-----|-------------|
| `[FEATURE]` | New user-facing capability or endpoint |
| `[ENHANCEMENT]` | Improvement to an existing feature (performance, UX, observability) |
| `[BUGFIX]` | Fix for a bug or regression |
| `[CHANGE]` | Breaking change, API modification, default value change, or deprecation |

### Component Prefixes

Use the component that best describes where the change lives:

| Component | Scope |
|-----------|-------|
| `Server` | HTTP server, middleware, routing |
| `Config` | Configuration loading, validation, reloading |
| `Content` | Markdown rendering, front matter, scanner, cache |
| `Theme` | Template engine, default templates, CSS |
| `OverlayFS` | Overlay filesystem |
| `GitOps` | Sync strategies (watch, webhook, sidecar), git operations |
| `Handlers` | HTTP route handlers (list, post, page, tag, feed, sitemap) |
| `CLI` | Command-line flags, subcommands (healthcheck, version) |
| `Docker` | Dockerfile, docker-compose |
| `Helm` | Helm chart templates, values |
| `K8s` | Kubernetes manifests, probes, PDB |
| `CI` | GitHub Actions workflows, linting, testing |
| `Docs` | Documentation, guides, README |
| `Security` | Security headers, CSP, HMAC, rate limiting |

For changes spanning multiple components, use the primary component prefix.

### Examples

```markdown
* [FEATURE] Server: Add Prometheus `/metrics` endpoint with request counter and duration histogram. #79
* [FEATURE] GitOps: Implement git-sync sidecar strategy with fsnotify symlink swap detection. #87
* [ENHANCEMENT] Security: Add Permissions-Policy and HSTS headers gated behind `tls_terminated` config. #77
* [BUGFIX] OverlayFS: Return nil for ErrNotExist in checkSymlinkSafe to unblock layer fallthrough. #63
* [BUGFIX] Content: Fix readingTime function signature to accept template.HTML type. #85
* [CHANGE] Config: NewLoader now accepts variadic LoaderOption instead of positional parameters. #101
* [ENHANCEMENT] CI: Add container smoke test verifying /healthz, /, /feed.xml, /metrics, and 404. #103
```

---

## Phase 1: Gather Change Context

### 1.1 Identify the PR

Determine the PR number and branch name from the current git context:

```bash
gh pr view --json number,title,body,labels
```

If no PR exists yet (pre-PR invocation), note that the PR number will be added after creation.

### 1.2 Identify the Changes

Determine what changed by examining:

1. **Issue title and labels**: Extract the `type:*` label to determine the tag:
   - `type:feature` → `[FEATURE]`
   - `type:bug` → `[BUGFIX]`
   - `type:enhancement` → `[ENHANCEMENT]`
   - `type:breaking` → `[CHANGE]`
   - No label → infer from commit prefix (`feat:` → `[FEATURE]`, `fix:` → `[BUGFIX]`, etc.)

2. **Commit messages**: Parse the conventional commit prefix:
   - `feat:` → `[FEATURE]`
   - `fix:` → `[BUGFIX]`
   - `chore:` / `refactor:` → `[ENHANCEMENT]`
   - `docs:` → `[ENHANCEMENT] Docs:`

3. **Changed files**: Determine the component from file paths:
   - `internal/server/` → `Server`
   - `internal/config/` → `Config`
   - `internal/content/` → `Content`
   - `internal/theme/` → `Theme`
   - `internal/overlayfs/` → `OverlayFS`
   - `internal/gitops/` → `GitOps`
   - `internal/server/handlers/` → `Handlers`
   - `cmd/blogflow/` → `CLI`
   - `Dockerfile` / `docker-compose*` → `Docker`
   - `deploy/helm/` → `Helm`
   - `examples/k8s/` → `K8s`
   - `.github/workflows/` → `CI`
   - `docs/` → `Docs`

### 1.3 Draft Entries

Create one entry per logical change. A single PR may have multiple entries if it spans
components or delivers multiple user-visible changes. Keep entries concise — one line each.

For multi-issue PRs (e.g., `Fixes #41, Fixes #42, Fixes #43, Fixes #44`), create one entry
per issue unless they're tightly coupled.

---

## Phase 2: Update CHANGELOG.md

### 2.1 Read the Existing Changelog

```bash
head -30 CHANGELOG.md
```

Verify the `## main / unreleased` section exists. If not, create it after the `# Changelog` heading.

### 2.2 Determine Insertion Point

Find the appropriate section heading. New entries go under `## main / unreleased` and within
the correct subsection (`### BlogFlow`, `### Helm Chart`, `### CI/CD`).

If no subsection exists for the component, create one following the existing order.

### 2.3 Insert Entries

Append entries at the **end** of the relevant subsection (newest entries last within a release,
matching Prometheus/Cortex convention). Entries within a section are ordered: `[CHANGE]` → `[FEATURE]` →
`[ENHANCEMENT]` → `[BUGFIX]`.

### 2.4 Validate

- No duplicate entries for the same PR number
- Tag matches the change type
- Component prefix is consistent with existing entries
- PR number is present (add `#TBD` if PR not yet created)

---

## Phase 3: Commit

### 3.1 Stage and Commit

The changelog update should be included in the same commit as the code change, NOT as a
separate commit. If the PR is already committed:

```bash
git add CHANGELOG.md
git commit --amend --no-edit
```

### 3.2 Output Summary

```
📝 Changelog Updated
━━━━━━━━━━━━━━━━━━━━
Section:  ## main / unreleased
Entries:  N added
Tags:     [FEATURE] ×1, [BUGFIX] ×1

New entries:
  * [FEATURE] GitOps: Implement git-sync sidecar strategy. #87
  * [BUGFIX] Server: Fix readyz body assertion in smoke test. #103
```

---

## Integration with Other Skills

### Called from implement-work-item

After Phase 7 (Build-Test-Fix Loop) succeeds and before Phase 8 (PR creation):

1. Invoke `update-changelog` with the issue context and changed files
2. The changelog entries are included in the implementation commit
3. The PR is created with changelog already present

### Called from review-fix-loop

If an RFL round results in code changes that warrant a changelog entry (e.g., a bugfix
discovered during review), the RFL skill may invoke `update-changelog` to add a `[BUGFIX]`
entry before the amended commit is pushed.

### Called standalone

Can be invoked directly to retroactively add changelog entries for PRs that were merged
without them. In this case, create a dedicated `chore: update changelog` commit.
