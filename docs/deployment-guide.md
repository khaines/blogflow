# Deployment Guide

> How to deploy BlogFlow in four patterns: local development, Kubernetes with
> git-sync sidecar, Kubernetes with webhook + go-git, and Docker production.

## Table of Contents

- [Overview](#overview)
- [Pattern 1: Local Development (watch)](#pattern-1-local-development-watch)
- [Pattern 2: Kubernetes — git-sync Sidecar](#pattern-2-kubernetes--git-sync-sidecar)
- [Pattern 3: Kubernetes — Webhook + go-git Pull](#pattern-3-kubernetes--webhook--go-git-pull)
- [Pattern 4: Docker Production (Webhook)](#pattern-4-docker-production-webhook)
- [Authentication Reference](#authentication-reference)
- [Environment Variable Reference](#environment-variable-reference)

---

## Overview

BlogFlow supports three content sync strategies plus an in-process git puller.
Choose a deployment pattern based on your environment and update-latency needs:

| Pattern | Strategy | Trigger | Latency | Network requirement |
|---|---|---|---|---|
| Local dev | `watch` | fsnotify file events | Instant (< 1 s) | None |
| K8s sidecar | `sidecar` | git-sync symlink swap | Poll interval (default 60 s) | Egress to git remote |
| K8s webhook | `webhook` | GitHub push webhook | Instant on push | Ingress (webhook) + egress (git clone) |
| Docker prod | `webhook` | GitHub push webhook | Instant on push | Ingress (webhook) + egress (git clone) |

All patterns share the same binary and container image. Only the `sync.strategy`
config value and surrounding infrastructure differ.

---

## Pattern 1: Local Development (watch)

### Architecture

```
┌─────────────┐       fsnotify        ┌──────────────┐
│  Local disk  │ ───── file events ──▶ │   BlogFlow   │
│  ./content/  │                       │  (watch mode) │
└─────────────┘                       └──────────────┘
      ▲                                      │
      │  You edit files                      │  Serves on :8080
```

The `watch` strategy uses [fsnotify](https://github.com/fsnotify/fsnotify) to
recursively monitor content directories. Changes to `.md`, `.html`, `.css`, and
`.yaml` files trigger a debounced content reload (500 ms window). Temporary
files, swap files, and `.git` paths are ignored.

### Option A: docker compose

```bash
# Start with live-reload (volumes mounted read-write, --dev flag)
docker compose -f docker-compose.yml -f docker-compose.dev.yml up
```

This mounts `./examples/content` and `./examples/config` into the container
with read-write access and passes the `--dev` flag for development niceties.
The port is bound to `127.0.0.1:8080` only.

### Option B: make dev

```bash
# Build and run locally (no Docker)
make dev
```

This compiles the binary and runs it with `--dev`. If `./data/content` or
`./data/theme` directories exist, they are passed automatically.

### Config (site.yaml)

```yaml
site:
  title: "My Dev Blog"
  base_url: "http://localhost:8080"

sync:
  strategy: "watch"

cache:
  enabled: false          # disable cache during development
```

### Volume strategy

| Path | Source | Access |
|---|---|---|
| `/data/content` | Bind mount to your local content dir | Read-write |
| `/data/config` | Bind mount to your config dir | Read-write |

No persistent volumes are needed — you are editing files directly on your host.

### Security notes

- The dev compose overlay disables `read_only` on the root filesystem for
  convenience. Do **not** use `docker-compose.dev.yml` in production.
- Port is bound to `127.0.0.1` only in dev mode.

---

## Pattern 2: Kubernetes — git-sync Sidecar

### Architecture

```
┌──────────────┐    HTTPS poll     ┌──────────────────────────────────────────────┐
│  Git remote   │ ◀─── (60 s) ─── │  Pod                                         │
│  (GitHub)     │                  │  ┌──────────────┐    ┌──────────────────┐    │
└──────────────┘                  │  │  git-sync     │    │  BlogFlow        │    │
                                  │  │  sidecar      │    │  (sidecar mode)  │    │
                                  │  │               │    │                  │    │
                                  │  │  clones repo  │    │  watches for     │    │
                                  │  │  swaps symlink│──▶ │  symlink swap    │    │
                                  │  │               │    │  reloads content │    │
                                  │  └───────┬───────┘    └────────┬─────────┘    │
                                  │          │  shared volume      │              │
                                  │          └─────── /data/content ──────────────┘
                                  └──────────────────────────────────────────────┘
```

**How it works:** The [git-sync](https://github.com/kubernetes/git-sync)
sidecar container polls the git remote on a configurable interval (default
60 s). When new commits are found, it clones them into a new directory and
atomically swaps a symlink to point at the new content. BlogFlow's `sidecar`
strategy watches `/data/content` with fsnotify and detects the symlink
Create/Remove/Rename events, then triggers a debounced content reload.

**Why choose this pattern:**
- No inbound webhook needed — ideal for restricted networks or air-gapped clusters
- git-sync is a mature, well-tested Kubernetes-native tool
- BlogFlow never needs git credentials — git-sync handles authentication
- Clean separation of concerns: git-sync manages git, BlogFlow manages content

### Kubernetes Deployment YAML

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: blogflow
  namespace: blogflow
  labels:
    app: blogflow
spec:
  replicas: 1
  selector:
    matchLabels:
      app: blogflow
  template:
    metadata:
      labels:
        app: blogflow
    spec:
      automountServiceAccountToken: false

      securityContext:
        runAsUser: 65532
        runAsGroup: 65532
        fsGroup: 65532
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault

      containers:
        # --- BlogFlow application container ---
        - name: blogflow
          image: ghcr.io/your-org/blogflow:latest  # pin by digest in production
          args: ["serve", "--content", "/data/content", "--config", "/data/config"]
          ports:
            - containerPort: 8080
              protocol: TCP

          securityContext:
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - ALL

          env:
            - name: BLOGFLOW_SYNC_STRATEGY
              value: "sidecar"

          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 128Mi

          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10

          readinessProbe:
            httpGet:
              path: /readyz
              port: 8080
            initialDelaySeconds: 3
            periodSeconds: 5

          volumeMounts:
            - name: content
              mountPath: /data/content
              readOnly: true
            - name: config
              mountPath: /data/config
              readOnly: true
            - name: cache
              mountPath: /data/cache
            - name: tmp
              mountPath: /tmp

        # --- git-sync sidecar container ---
        - name: git-sync
          image: registry.k8s.io/git-sync/git-sync:v4.4.0  # pin by digest in production
          args:
            - --repo=https://github.com/your-org/blog-content.git
            - --ref=main
            - --root=/data/content
            - --period=60s
            - --link=current
          env:
            - name: GITSYNC_USERNAME
              value: "x-access-token"
            - name: GITSYNC_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: blogflow-git-credentials
                  key: token

          securityContext:
            runAsUser: 65532
            runAsGroup: 65532
            runAsNonRoot: true
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - ALL

          resources:
            requests:
              cpu: 25m
              memory: 32Mi
            limits:
              cpu: 100m
              memory: 64Mi

          volumeMounts:
            - name: content
              mountPath: /data/content
            - name: tmp-gitsync
              mountPath: /tmp

      volumes:
        - name: content
          emptyDir: {}           # shared between git-sync (writer) and blogflow (reader)
        - name: config
          configMap:
            name: blogflow-config
        - name: cache
          emptyDir: {}
        - name: tmp
          emptyDir:
            medium: Memory
            sizeLimit: 64Mi
        - name: tmp-gitsync
          emptyDir:
            medium: Memory
            sizeLimit: 64Mi
```

### Service and Ingress

```yaml
apiVersion: v1
kind: Service
metadata:
  name: blogflow
  namespace: blogflow
spec:
  selector:
    app: blogflow
  ports:
    - port: 80
      targetPort: 8080
      protocol: TCP
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: blogflow
  namespace: blogflow
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - blog.example.com
      secretName: blogflow-tls
  rules:
    - host: blog.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: blogflow
                port:
                  number: 80
```

### Config (site.yaml)

```yaml
site:
  title: "My Blog"
  base_url: "https://blog.example.com"

server:
  tls_terminated: true        # behind ingress TLS termination
  hsts_max_age: 63072000

sync:
  strategy: "sidecar"

cache:
  enabled: true
  ttl: "1h"
```

Deploy this as a ConfigMap:

```bash
kubectl create configmap blogflow-config \
  --from-file=site.yaml=site.yaml \
  -n blogflow
```

### Volume strategy

| Volume | Type | Writer | Reader | Purpose |
|---|---|---|---|---|
| `content` | `emptyDir` | git-sync | BlogFlow (read-only mount) | Shared content via symlink swap |
| `config` | `ConfigMap` | — | BlogFlow | `site.yaml` configuration |
| `cache` | `emptyDir` | BlogFlow | BlogFlow | Rendered page cache (ephemeral) |
| `tmp` | `emptyDir` (Memory) | BlogFlow | BlogFlow | Go temp files in RAM |
| `tmp-gitsync` | `emptyDir` (Memory) | git-sync | git-sync | git-sync temp files in RAM |

The content volume is an `emptyDir` shared between both containers. git-sync
writes to it; BlogFlow mounts it as `readOnly: true`. Using `emptyDir` instead
of a PVC is intentional — git-sync fully repopulates it on startup, so there is
nothing to persist across pod restarts.

### Authentication setup

git-sync handles all git authentication. BlogFlow itself needs no git
credentials in this pattern.

**Option A: Personal Access Token (PAT) or GitHub App token**

```bash
kubectl create secret generic blogflow-git-credentials \
  --from-literal=token=ghp_YourTokenHere \
  -n blogflow
```

The Deployment YAML above references this secret via `GITSYNC_PASSWORD`.

**Option B: SSH key**

```bash
kubectl create secret generic blogflow-git-ssh \
  --from-file=ssh-key=/path/to/deploy_key \
  --from-file=known_hosts=/path/to/known_hosts \
  -n blogflow
```

Update the git-sync container to use SSH:

```yaml
args:
  - --repo=git@github.com:your-org/blog-content.git
  - --ref=main
  - --root=/data/content
  - --period=60s
  - --ssh-key-file=/etc/git-secret/ssh-key
  - --ssh-known-hosts-file=/etc/git-secret/known_hosts
volumeMounts:
  - name: git-ssh
    mountPath: /etc/git-secret
    readOnly: true

# Add to volumes:
- name: git-ssh
  secret:
    secretName: blogflow-git-ssh
    defaultMode: 0400
```

### Security notes

- Both containers run as UID 65532 (nonroot) with all capabilities dropped.
- Root filesystem is read-only; only explicit `emptyDir` volumes are writable.
- No inbound webhook endpoint is needed — no ingress path to protect.
- Network policy: allow egress TCP 443 (git HTTPS) and UDP/TCP 53 (DNS) only.
- See [Container Security Guide](engineering/container-security.md) for the
  full security context reference.

---

## Pattern 3: Kubernetes — Webhook + go-git Pull

### Architecture

```
┌──────────────┐   push event    ┌───────────┐   POST /api/webhook   ┌──────────────┐
│  Developer    │ ──────────────▶ │  GitHub    │ ─────────────────────▶│  BlogFlow    │
│  git push     │                │  Webhooks  │                       │  (webhook    │
└──────────────┘                └───────────┘                       │   mode)      │
                                                                     │              │
                                                     go-git clone    │  clones/pulls│
                                      ┌──────────── ◀──────────────  │  to R/W vol  │
                                      │ Git remote                   │              │
                                      └────────────────────────────▶ │  reloads     │
                                                                     └──────────────┘
```

**How it works:** GitHub sends a push webhook to BlogFlow's `/api/webhook`
endpoint. BlogFlow validates the HMAC-SHA256 signature, checks the branch
filter, and then uses [go-git](https://github.com/go-git/go-git) to clone
(first time) or pull (subsequent) the content repository to a read-write
volume. Content is reloaded immediately after pull completes.

**Why choose this pattern:**
- Instant updates — no polling delay
- Single container — simpler than the sidecar pattern
- BlogFlow manages git directly via go-git (shallow clone, single-branch)

**Trade-offs:**
- Requires inbound webhook access (Ingress must expose `/api/webhook`)
- Requires egress to git remote (HTTPS or SSH)
- BlogFlow needs git credentials (unlike the sidecar pattern)

### Kubernetes Deployment YAML

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: blogflow
  namespace: blogflow
  labels:
    app: blogflow
spec:
  replicas: 1
  selector:
    matchLabels:
      app: blogflow
  template:
    metadata:
      labels:
        app: blogflow
    spec:
      automountServiceAccountToken: false

      securityContext:
        runAsUser: 65532
        runAsGroup: 65532
        fsGroup: 65532
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault

      containers:
        - name: blogflow
          image: ghcr.io/your-org/blogflow:latest  # pin by digest in production
          args: ["serve", "--content", "/data/content", "--config", "/data/config"]
          ports:
            - containerPort: 8080
              protocol: TCP

          securityContext:
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - ALL

          env:
            - name: BLOGFLOW_SYNC_STRATEGY
              value: "webhook"
            - name: BLOGFLOW_WEBHOOK_SECRET
              valueFrom:
                secretKeyRef:
                  name: blogflow-secrets
                  key: webhook-secret
            - name: BLOGFLOW_GIT_TOKEN
              valueFrom:
                secretKeyRef:
                  name: blogflow-secrets
                  key: git-token

          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 128Mi

          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10

          readinessProbe:
            httpGet:
              path: /readyz
              port: 8080
            initialDelaySeconds: 3
            periodSeconds: 5

          volumeMounts:
            - name: content
              mountPath: /data/content
            - name: config
              mountPath: /data/config
              readOnly: true
            - name: cache
              mountPath: /data/cache
            - name: tmp
              mountPath: /tmp

      volumes:
        - name: content
          persistentVolumeClaim:
            claimName: blogflow-content    # R/W — go-git clones into this
        - name: config
          configMap:
            name: blogflow-config
        - name: cache
          emptyDir: {}
        - name: tmp
          emptyDir:
            medium: Memory
            sizeLimit: 64Mi
```

### PersistentVolumeClaim

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: blogflow-content
  namespace: blogflow
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi               # adjust based on content repo size
```

### Service and Ingress

```yaml
apiVersion: v1
kind: Service
metadata:
  name: blogflow
  namespace: blogflow
spec:
  selector:
    app: blogflow
  ports:
    - port: 80
      targetPort: 8080
      protocol: TCP
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: blogflow
  namespace: blogflow
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    # Rate-limit the webhook path at the ingress level
    nginx.ingress.kubernetes.io/limit-rps: "5"
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - blog.example.com
      secretName: blogflow-tls
  rules:
    - host: blog.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: blogflow
                port:
                  number: 80
```

### Secrets

```bash
# Generate a strong webhook secret (min 32 characters)
WEBHOOK_SECRET=$(openssl rand -hex 32)

kubectl create secret generic blogflow-secrets \
  --from-literal=webhook-secret="$WEBHOOK_SECRET" \
  --from-literal=git-token=ghp_YourTokenHere \
  -n blogflow
```

### Config (site.yaml)

```yaml
site:
  title: "My Blog"
  base_url: "https://blog.example.com"

server:
  tls_terminated: true
  hsts_max_age: 63072000

sync:
  strategy: "webhook"
  webhook:
    path: "/api/webhook"
    branch_filter: "main"
    allowed_events:
      - push
    rate_limit: 10            # max webhook requests per minute per IP

cache:
  enabled: true
  ttl: "5m"                   # lower TTL to reduce stale window after deploys
```

### GitHub Webhook Setup

1. Go to your **content repository** → Settings → Webhooks → Add webhook.
2. Set **Payload URL** to `https://blog.example.com/api/webhook`.
3. Set **Content type** to `application/json`.
4. Set **Secret** to the same value as `BLOGFLOW_WEBHOOK_SECRET`.
5. Under **Which events?**, select **Just the push event**.
6. Save.

Alternatively, use the GitHub Actions workflow from
[Content Deploy Setup](content-deploy-setup.md) for a CI-driven webhook that
computes the HMAC signature via a GitHub Actions secret.

**Verifying the webhook:**

```bash
# Send a test payload
echo '{"ref":"refs/heads/main"}' | \
  curl -s -X POST \
    -H "Content-Type: application/json" \
    -H "X-Hub-Signature-256: sha256=$(echo -n '{"ref":"refs/heads/main"}' | openssl dgst -sha256 -hmac "$WEBHOOK_SECRET" | cut -d' ' -f2)" \
    -d @- \
    https://blog.example.com/api/webhook
```

### Volume strategy

| Volume | Type | Access | Purpose |
|---|---|---|---|
| `content` | PVC (`ReadWriteOnce`) | Read-write | go-git clone/pull target |
| `config` | `ConfigMap` | Read-only | `site.yaml` |
| `cache` | `emptyDir` | Read-write | Rendered page cache |
| `tmp` | `emptyDir` (Memory) | Read-write | Temp files in RAM |

The content volume uses a PVC (not `emptyDir`) so that cloned content survives
pod restarts. On startup, go-git detects the existing `.git` directory and
pulls instead of re-cloning. If the pull fails (e.g., shallow clone corruption),
it falls back to a full re-clone automatically.

### Authentication setup

BlogFlow's go-git puller needs credentials to access private content repos.
Credentials are loaded from environment variables at startup.

**Option A: Personal Access Token (PAT) or GitHub App installation token**

```bash
# Set via environment variable
BLOGFLOW_GIT_TOKEN=ghp_YourTokenHere
```

go-git uses this as `x-access-token` basic auth (GitHub convention).

**Option B: SSH deploy key**

```bash
# Set via environment variable
BLOGFLOW_GIT_SSH_KEY=/etc/blogflow/ssh/deploy_key
```

Mount the key as a Kubernetes Secret:

```yaml
env:
  - name: BLOGFLOW_GIT_SSH_KEY
    value: /etc/blogflow/ssh/deploy_key
volumeMounts:
  - name: git-ssh
    mountPath: /etc/blogflow/ssh
    readOnly: true

# Add to volumes:
volumes:
  - name: git-ssh
    secret:
      secretName: blogflow-git-ssh
      defaultMode: 0400   # required: key must be 0600 or 0400
```

> **SSH key permissions:** BlogFlow validates that the SSH key file has
> permissions `0600` or `0400`. Keys with group/other access are rejected.

**Option C: No auth (public repos)**

If your content repository is public, no credentials are needed. BlogFlow
defaults to `AuthNone`.

### Security notes

- The webhook endpoint validates HMAC-SHA256 signatures on every request.
  Requests with missing or invalid signatures are rejected with 401.
- Branch filtering prevents non-target branches from triggering reloads.
- Rate limiting (default: 10 requests/minute/IP) prevents abuse.
- Request body size is capped at 1 MB by default.
- The webhook secret must be set via `BLOGFLOW_WEBHOOK_SECRET` environment
  variable — never in `site.yaml` (BlogFlow rejects YAML containing secrets).
- Consider restricting webhook ingress to [GitHub's webhook IP
  ranges](https://api.github.com/meta) via NetworkPolicy or ingress annotations.

---

## Pattern 4: Docker Production (Webhook)

### Architecture

```
┌──────────────┐   push event    ┌───────────┐   POST /api/webhook   ┌──────────────┐
│  Developer    │ ──────────────▶ │  GitHub    │ ─────────────────────▶│  BlogFlow    │
│  git push     │                │  Webhooks  │                       │  (Docker)    │
└──────────────┘                └───────────┘                       │              │
                                                                     │  go-git pull │
                                                                     │  to volume   │
                                                                     │              │
                                                                     │  :8080       │
                                                                     └──────────────┘
```

This is the same webhook + go-git pull pattern as Pattern 3, but deployed with
docker compose instead of Kubernetes.

### docker-compose.yml

```yaml
services:
  blogflow:
    image: ghcr.io/your-org/blogflow:latest   # pin by digest in production
    command: ["serve", "--content", "/data/content", "--config", "/data/config"]
    ports:
      - "8080:8080"
    volumes:
      - blogflow-content:/data/content        # persistent R/W volume for go-git
      - ./config:/data/config:ro              # site.yaml
    environment:
      - BLOGFLOW_SYNC_STRATEGY=webhook
      - BLOGFLOW_WEBHOOK_SECRET=${BLOGFLOW_WEBHOOK_SECRET}
      - BLOGFLOW_GIT_TOKEN=${BLOGFLOW_GIT_TOKEN}
      - BLOGFLOW_SERVER_PORT=8080
    restart: unless-stopped
    read_only: true
    cap_drop:
      - ALL
    security_opt:
      - no-new-privileges:true
    tmpfs:
      - /tmp:rw,noexec,nosuid,size=64m
      - /data/cache:rw,noexec,nosuid,size=128m
    deploy:
      resources:
        limits:
          memory: 256M
          cpus: "1.0"
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"

volumes:
  blogflow-content:               # named volume persists across restarts
```

### Config (site.yaml)

Place this in `./config/site.yaml` on the Docker host:

```yaml
site:
  title: "My Blog"
  base_url: "https://blog.example.com"

server:
  tls_terminated: true
  hsts_max_age: 63072000

sync:
  strategy: "webhook"
  webhook:
    path: "/api/webhook"
    branch_filter: "main"
    allowed_events:
      - push
    rate_limit: 10

cache:
  enabled: true
  ttl: "5m"
```

### Environment file

Create a `.env` file alongside `docker-compose.yml` (never commit this):

```bash
BLOGFLOW_WEBHOOK_SECRET=your-webhook-secret-min-32-chars
BLOGFLOW_GIT_TOKEN=ghp_YourTokenHere
```

### Reverse proxy

Place a reverse proxy (nginx, Caddy, Traefik) in front of BlogFlow for TLS
termination. Example with Caddy added to the compose file:

```yaml
services:
  caddy:
    image: caddy:2-alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy-data:/data
      - caddy-config:/config
    depends_on:
      - blogflow

volumes:
  caddy-data:
  caddy-config:
```

```
# Caddyfile
blog.example.com {
    reverse_proxy blogflow:8080
}
```

### GitHub Webhook Setup

Follow the same steps as [Pattern 3](#github-webhook-setup):

1. Content repo → Settings → Webhooks → Add webhook.
2. Payload URL: `https://blog.example.com/api/webhook`
3. Content type: `application/json`
4. Secret: same as `BLOGFLOW_WEBHOOK_SECRET`
5. Events: push only.

### Volume strategy

| Volume | Type | Access | Purpose |
|---|---|---|---|
| `blogflow-content` | Named Docker volume | Read-write | go-git clone/pull target; persists across restarts |
| `./config` | Bind mount | Read-only | `site.yaml` |
| `/tmp` (tmpfs) | tmpfs | Read-write | Temp files in RAM, 64 MB |
| `/data/cache` (tmpfs) | tmpfs | Read-write | Render cache in RAM, 128 MB |

The named volume `blogflow-content` persists the cloned content repo across
container restarts. go-git detects the existing clone and pulls incrementally.

### Authentication setup

Set credentials via environment variables in `.env`:

| Variable | Purpose |
|---|---|
| `BLOGFLOW_GIT_TOKEN` | PAT or GitHub App token for private content repos |
| `BLOGFLOW_GIT_SSH_KEY` | Path to SSH deploy key (mount into container) |
| `BLOGFLOW_WEBHOOK_SECRET` | HMAC-SHA256 shared secret (min 32 chars) |

For SSH auth, mount the key into the container:

```yaml
volumes:
  - /path/to/deploy_key:/etc/blogflow/ssh/deploy_key:ro
environment:
  - BLOGFLOW_GIT_SSH_KEY=/etc/blogflow/ssh/deploy_key
```

### Security notes

- Container runs with `read_only: true`, `cap_drop: ALL`, and
  `no-new-privileges`.
- Secrets are injected via environment variables, never baked into the image.
- Use a reverse proxy for TLS termination — BlogFlow listens on plain HTTP
  (port 8080).
- Restrict webhook access at the reverse proxy level (e.g., IP allowlist for
  GitHub's webhook ranges).
- Pin the image by SHA256 digest in production (see
  [Container Security Guide](engineering/container-security.md#image-pinning)).

---

## Authentication Reference

BlogFlow loads git authentication from environment variables at startup via
`LoadAuthFromEnv()`. The precedence order:

| Priority | Environment variable | Auth method | Use case |
|---|---|---|---|
| 1 | `BLOGFLOW_GIT_SSH_KEY` | SSH public key | Deploy keys, machine users |
| 2 | `BLOGFLOW_GIT_TOKEN` | Token (basic auth) | PATs, GitHub App installation tokens |
| — | *(neither set)* | None | Public repositories |

### SSH key requirements

- File permissions must be `0600` or `0400` (no group/other access).
- BlogFlow validates permissions at startup and rejects unsafe keys.
- The key is passed to go-git's SSH transport as the `git` user.

### Token auth

- Tokens are sent as HTTP basic auth with username `x-access-token` (GitHub
  convention for fine-grained PATs and GitHub App tokens).
- Classic PATs (`ghp_*`) and fine-grained PATs both work.
- For GitHub Apps: generate an installation access token and set it as
  `BLOGFLOW_GIT_TOKEN`. Tokens expire (typically 1 hour), so use a sidecar or
  cron job to refresh them.

### Webhook secret

- Set via `BLOGFLOW_WEBHOOK_SECRET` (environment variable only — never YAML).
- Must be at least 32 characters.
- Used for HMAC-SHA256 signature validation on incoming webhooks.
- Must match the secret configured in GitHub's webhook settings.

---

## Environment Variable Reference

| Variable | Description | Used by |
|---|---|---|
| `BLOGFLOW_SYNC_STRATEGY` | Sync strategy: `watch`, `webhook`, `sidecar` | All patterns |
| `BLOGFLOW_WEBHOOK_SECRET` | HMAC-SHA256 webhook secret | Webhook patterns (3, 4) |
| `BLOGFLOW_GIT_TOKEN` | Git PAT or GitHub App token | Webhook patterns (3, 4) |
| `BLOGFLOW_GIT_SSH_KEY` | Path to SSH private key | Webhook patterns (3, 4) |
| `BLOGFLOW_SERVER_PORT` | HTTP listen port (default: 8080) | All patterns |
| `BLOGFLOW_SERVER_TLS_TERMINATED` | Enable HSTS header | Production patterns |
| `BLOGFLOW_SERVER_HSTS_MAX_AGE` | HSTS max-age in seconds | Production patterns |
| `BLOGFLOW_SITE_BASE_URL` | Canonical site URL | All patterns |
| `BLOGFLOW_CACHE_ENABLED` | Enable/disable render cache | All patterns |
