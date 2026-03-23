# BlogFlow Kubernetes Examples

Plain YAML manifests for deploying BlogFlow on Kubernetes without Helm.
Two deployment patterns are provided — pick the one that matches your content
workflow.

## Patterns

### Sidecar (`sidecar/`)

A git-sync sidecar container polls your content repository on a fixed interval
(default 60 s) and writes updates to a shared volume. BlogFlow watches the
volume for changes and re-renders automatically.

**Best for:** simple setups, private clusters, or environments without an
ingress controller.

### Webhook (`webhook/`)

BlogFlow exposes a webhook endpoint (`/api/webhook`) that receives push events
from GitHub (or any Git host). On each event it pulls the latest content and
re-renders.

**Best for:** public-facing sites where you want instant updates on push.

## Quick start

```bash
# Sidecar pattern
kubectl apply -k examples/k8s/sidecar/

# Webhook pattern
kubectl apply -k examples/k8s/webhook/
```

## Customisation

1. Search each manifest for `CHANGE-ME` and replace with your values.
2. Adjust resource requests/limits to match your traffic profile.
3. Pin the BlogFlow image to a specific semver version (already set to `0.1.0`).
   For production, pin by SHA256 digest instead of a tag (see
   `docs/engineering/container-security.md`).
4. For production, pin the git-sync image by digest (see comments in the
   sidecar deployment).

All manifests follow the security baseline from
[docs/engineering/container-security.md](../../docs/engineering/container-security.md):
non-root user (UID 65532), read-only root filesystem, all capabilities dropped,
seccomp RuntimeDefault profile.
