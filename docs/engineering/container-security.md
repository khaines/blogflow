# Container Security Guide

> BlogFlow container hardening: distroless base, rootless execution, read-only
> filesystem, minimal capabilities, supply-chain integrity.

## Table of Contents

- [Base Image](#base-image)
- [Rootless Execution](#rootless-execution)
- [Read-Only Root Filesystem](#read-only-root-filesystem)
- [Volume Strategy](#volume-strategy)
- [Dropped Capabilities](#dropped-capabilities)
- [Image Pinning](#image-pinning)
- [Supply Chain Security](#supply-chain-security)
- [Kubernetes SecurityContext](#kubernetes-securitycontext)
- [Network Policy](#network-policy)
- [Secret Management](#secret-management)
- [Complete Kubernetes Deployment Spec](#complete-kubernetes-deployment-spec)
- [Docker Run with Security Flags](#docker-run-with-security-flags)

---

## Base Image

BlogFlow uses **`gcr.io/distroless/static-debian12:nonroot`** as its runtime
base. Distroless images contain only the application binary and its runtime
dependencies — nothing else.

What is **not** present in the image:

| Absent component | Security benefit |
|---|---|
| Shell (`sh`, `bash`) | No interactive shell access if the container is compromised |
| Package manager (`apt`, `apk`) | Attacker cannot install tools post-exploitation |
| OS utilities (`curl`, `wget`, `nc`) | No network recon or data exfiltration tooling |
| Temp file creators (`mktemp`) | Reduces writable-surface available to exploits |
| Compilers / interpreters | No on-host code compilation or scripting |

The final image is a statically-linked Go binary (`CGO_ENABLED=0`) on a
minimal scratch-like base, typically under **15 MB** total.

```dockerfile
# Runtime stage — distroless, rootless, no shell
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
```

### Why distroless over scratch?

`scratch` contains literally nothing — no CA certificates, no timezone data, no
`/etc/passwd`. Distroless provides these essentials while still eliminating
shells, package managers, and utilities. The `static` variant is purpose-built
for statically-linked Go binaries.

---

## Rootless Execution

The container runs as **UID 65532** (`nonroot`), never as root.

```dockerfile
USER nonroot:nonroot
```

This is enforced at three levels:

1. **Dockerfile**: `USER nonroot:nonroot` sets the default runtime user.
2. **Kubernetes SecurityContext**: `runAsUser: 65532` and `runAsNonRoot: true`
   prevent the kubelet from starting the container if it would run as root.
3. **Base image**: The `nonroot` tag of distroless bakes UID 65532 into the
   image metadata itself.

Even if an attacker escapes the application process, they land as an
unprivileged user with no ability to escalate (see [Dropped
Capabilities](#dropped-capabilities)).

---

## Read-Only Root Filesystem

Kubernetes can mount the container's root filesystem as read-only, preventing
any writes to the image layers at runtime.

```yaml
securityContext:
  readOnlyRootFilesystem: true
```

### What this blocks

- Writing to `/usr`, `/bin`, `/etc`, or any path inside the image
- Creating new files in the container's overlay filesystem
- Dropping malicious binaries onto disk post-exploitation

### What this requires

All writable paths must be explicitly mounted as volumes. BlogFlow needs only a
small set of writable directories (see [Volume Strategy](#volume-strategy)).

---

## Volume Strategy

With `readOnlyRootFilesystem: true`, every writable path must be an explicit
mount. BlogFlow uses the following volume layout:

| Mount path | Type | Purpose | Contents |
|---|---|---|---|
| `/data/content` | PersistentVolume or hostPath | Blog content | Markdown files, static assets synced from git |
| `/data/theme` | PersistentVolume or hostPath | Theme override | Custom templates, CSS, JS overriding embedded defaults |
| `/data/config` | ConfigMap or PersistentVolume | Runtime config | `site.yaml` and environment-specific overrides |
| `/data/cache` | `emptyDir` | Rendered page cache | Evicted on pod restart; rebuilt from content on startup |
| `/tmp` | `emptyDir` (tmpfs) | Temp files | Go's `os.TempDir()`; backed by RAM, never hits disk |

### Volume configuration in Kubernetes

```yaml
volumes:
  - name: content
    persistentVolumeClaim:
      claimName: blogflow-content
  - name: theme
    persistentVolumeClaim:
      claimName: blogflow-theme
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

```yaml
volumeMounts:
  - name: content
    mountPath: /data/content
  - name: theme
    mountPath: /data/theme
  - name: config
    mountPath: /data/config
    readOnly: true
  - name: cache
    mountPath: /data/cache
  - name: tmp
    mountPath: /tmp
```

**Key points:**

- `/data/config` is mounted read-only; the application reads it but never writes to it.
- `/data/cache` uses `emptyDir` so it is ephemeral and scoped to the pod's
  lifetime. Cache is rebuilt on startup from content files.
- `/tmp` uses `emptyDir` with `medium: Memory` to back it with tmpfs (RAM),
  ensuring temp files never touch persistent storage.

---

## Dropped Capabilities

BlogFlow drops **all** Linux capabilities and disables privilege escalation:

```yaml
securityContext:
  capabilities:
    drop:
      - ALL
  allowPrivilegeEscalation: false
```

### What this means

- The process cannot bind to privileged ports (< 1024). BlogFlow listens on
  **port 8080**.
- No `CAP_NET_RAW` — cannot craft raw packets or perform ARP spoofing.
- No `CAP_SYS_ADMIN` — cannot mount filesystems, load kernel modules, or use
  many syscalls.
- No `CAP_DAC_OVERRIDE` — cannot bypass file permission checks.
- `allowPrivilegeEscalation: false` sets `no_new_privs` on the process,
  blocking privilege gains via setuid/setgid binaries. It does **not** prevent
  kernel exploits — keep nodes patched and use seccomp/AppArmor for kernel-level
  defense. (There are no setuid binaries in distroless anyway, but
  defense-in-depth matters.)

No capabilities are added back. BlogFlow's Go binary requires zero Linux
capabilities to serve HTTP on an unprivileged port.

---

## Image Pinning

### Development and CI

Use semver tags for readability in non-production contexts:

```
ghcr.io/your-org/blogflow:1.2.3
```

### Production

**Always pin by SHA256 digest.** Tags are mutable — a compromised registry or
CI pipeline could replace a tagged image with a malicious one. Digests are
content-addressed and immutable.

```
ghcr.io/your-org/blogflow@sha256:abc123def456...
```

### How to obtain the digest

After pushing an image in CI, capture the digest:

```bash
# From docker CLI
docker inspect --format='{{index .RepoDigests 0}}' ghcr.io/your-org/blogflow:1.2.3

# From crane (recommended for CI)
crane digest ghcr.io/your-org/blogflow:1.2.3
```

Store the digest in your Kubernetes manifests, Helm values, or GitOps
repository. Update it only through your CI/CD pipeline after a successful build
and scan.

---

## Supply Chain Security

### Trivy scanning in CI

The `release.yml` workflow scans every release image for known vulnerabilities
before it leaves CI:

```yaml
- name: Scan image with Trivy
  # TODO: verify this digest matches the latest v0.28.0 release
  uses: aquasecurity/trivy-action@0123456789abcdef0123456789abcdef01234567  # v0.28.0
  with:
    image-ref: ghcr.io/${{ github.repository }}:${{ github.ref_name }}
    format: 'sarif'
    output: 'trivy-results.sarif'
    severity: 'CRITICAL,HIGH'

- name: Upload Trivy scan results
  uses: github/codeql-action/upload-sarif@v3
  if: always()
  with:
    sarif_file: 'trivy-results.sarif'
```

- **Severity gate**: Only `CRITICAL` and `HIGH` vulnerabilities are flagged.
- **SARIF upload**: Results appear in the repository's Security tab under Code
  Scanning alerts.
- **`if: always()`**: Scan results are uploaded even if Trivy finds
  vulnerabilities, ensuring visibility.

### SBOM generation

Generate a Software Bill of Materials for every release image to enable
downstream vulnerability tracking:

```bash
# Generate SBOM with Trivy
trivy image --format cyclonedx --output sbom.json ghcr.io/your-org/blogflow:1.2.3

# Or with syft
syft ghcr.io/your-org/blogflow:1.2.3 -o cyclonedx-json > sbom.json
```

Attach the SBOM as a release artifact or push it alongside the image using
cosign's attestation flow.

### Additional recommendations

- **Pin CI action versions** by SHA digest (e.g.,
  `actions/checkout@<sha>`) to prevent supply-chain attacks via
  compromised action tags.
- **Enable Dependabot** for Go module and GitHub Actions version updates.
- **Sign images** with cosign and verify signatures before deployment.

---

## Kubernetes SecurityContext

The full `securityContext` applied to BlogFlow containers:

```yaml
# Pod-level security context
securityContext:
  runAsUser: 65532
  runAsGroup: 65532
  fsGroup: 65532
  runAsNonRoot: true
  seccompProfile:
    type: RuntimeDefault

# Container-level security context
securityContext:
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop:
      - ALL
```

### Field-by-field breakdown

| Field | Value | Purpose |
|---|---|---|
| `runAsUser` | `65532` | Forces UID 65532 (nonroot) regardless of image metadata |
| `runAsGroup` | `65532` | Forces GID 65532 for all processes |
| `fsGroup` | `65532` | Ensures volume files are accessible to the nonroot group |
| `runAsNonRoot` | `true` | Kubelet rejects the pod if the resolved UID is 0 |
| `seccompProfile.type` | `RuntimeDefault` | Applies the container runtime's default seccomp profile, blocking dangerous syscalls |
| `readOnlyRootFilesystem` | `true` | Prevents writes to the image's filesystem layers |
| `allowPrivilegeEscalation` | `false` | Blocks setuid/setgid and ptrace-based escalation |
| `capabilities.drop` | `ALL` | Removes every Linux capability from the process |

---

## Network Policy

Restrict BlogFlow's network exposure to the minimum required:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: blogflow-netpol
  namespace: blogflow
spec:
  podSelector:
    matchLabels:
      app: blogflow
  policyTypes:
    - Ingress
    - Egress

  ingress:
    # Allow traffic only on the HTTP serving port
    - ports:
        - protocol: TCP
          port: 8080
      from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: ingress-nginx
          podSelector:
            matchLabels:
              app.kubernetes.io/name: ingress-nginx

  egress:
    # DNS resolution
    - ports:
        - protocol: UDP
          port: 53
        - protocol: TCP
          port: 53
      to:
        - namespaceSelector: {}
          podSelector:
            matchLabels:
              k8s-app: kube-dns

    # HTTPS to any external endpoint (L4 only — consider Cilium/Calico FQDN policy for production)
    - ports:
        - protocol: TCP
          port: 443
```

### Policy breakdown

| Direction | What | Why |
|---|---|---|
| Ingress TCP 8080 | HTTP from ingress controller only | BlogFlow serves HTTP on 8080; no other port is needed |
| Egress UDP/TCP 53 | DNS to kube-dns | Required for any outbound name resolution |
| Egress TCP 443 | HTTPS to git remotes | git-sync sidecar pulls content repository over HTTPS |

All other traffic — inbound and outbound — is **denied by default** when both
`Ingress` and `Egress` are listed in `policyTypes`.

---

## Secret Management

### Principles

1. **Never store secrets in config files, images, or git repositories.**
2. **Never bake secrets into the container image at build time.**
3. **Use environment variables or Kubernetes Secrets at runtime.**

### Kubernetes Secrets

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: blogflow-secrets
  namespace: blogflow
type: Opaque
data:
  BLOGFLOW_WEBHOOK_SECRET: <base64-encoded-value>
  BLOGFLOW_GIT_TOKEN: <base64-encoded-value>
```

> ⚠️ **Kubernetes Secrets are base64-encoded, not encrypted.** Anyone with
> `get`/`list` access to Secrets in the namespace can decode them trivially.
> Enable [etcd encryption at rest](https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/)
> to protect Secret data on disk, and consider an external secrets manager
> (Vault, AWS Secrets Manager, etc.) for stronger guarantees.

Mount as environment variables in the pod spec:

```yaml
envFrom:
  - secretRef:
      name: blogflow-secrets
```

Or mount individual keys:

```yaml
env:
  - name: BLOGFLOW_WEBHOOK_SECRET
    valueFrom:
      secretKeyRef:
        name: blogflow-secrets
        key: BLOGFLOW_WEBHOOK_SECRET
```

### Recommendations

- **Rotate secrets regularly** and redeploy pods to pick up new values.
- **Use an external secrets operator** (e.g., External Secrets Operator, Sealed
  Secrets, or Vault Agent) to sync secrets from a vault into Kubernetes.
- **Limit RBAC**: Only the BlogFlow service account should have `get` access to
  its secrets.
- **Audit access**: Enable Kubernetes audit logging for Secret read events.

---

## Complete Kubernetes Deployment Spec

A production-ready Deployment spec incorporating every security control
documented above:

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
          image: ghcr.io/your-org/blogflow@sha256:abc123def456...
          ports:
            - containerPort: 8080
              protocol: TCP

          securityContext:
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - ALL

          envFrom:
            - secretRef:
                name: blogflow-secrets

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
            - name: theme
              mountPath: /data/theme
            - name: config
              mountPath: /data/config
              readOnly: true
            - name: cache
              mountPath: /data/cache
            - name: tmp
              mountPath: /tmp

        - name: git-sync
          # TODO: replace with verified digest from
          #   crane digest registry.k8s.io/git-sync/git-sync:v4.4.0
          image: registry.k8s.io/git-sync/git-sync@sha256:REPLACE_WITH_VERIFIED_DIGEST
          args:
            - --repo=https://github.com/your-org/blog-content.git
            - --root=/data/content
            - --period=60s
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
            - name: tmp
              mountPath: /tmp

      volumes:
        - name: content
          persistentVolumeClaim:
            claimName: blogflow-content
        - name: theme
          persistentVolumeClaim:
            claimName: blogflow-theme
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

> ⚠️ **Do not use `hostPath` volumes in production.** `hostPath` mounts bypass
> Pod Security Standards (Restricted) and expose the host filesystem to the
> container. Use `PersistentVolumeClaim` (as shown above) or CSI-backed volumes
> instead. `hostPath` is acceptable only in single-node development clusters.

### Notable settings

- **`automountServiceAccountToken: false`**: BlogFlow does not call the
  Kubernetes API, so the service account token is not mounted. This removes a
  credential from the container's filesystem.
- **Deployment with `replicas: 1`**: Unlike a bare Pod, a Deployment ensures the
  pod is rescheduled if it is evicted or the node fails.
- **Resource limits**: Prevents runaway resource consumption. Adjust based on
  your traffic profile.
- **Health probes**: Kubernetes restarts the pod if it becomes unhealthy;
  traffic is only routed to ready pods.
- **git-sync sidecar**: Runs with the same security constraints as the main
  container (including `runAsUser: 65532`, `runAsNonRoot: true`, and full
  capability drop). Pulls content changes on a 60-second interval.

---

## Docker Run with Security Flags

For local development or non-Kubernetes deployments, apply equivalent security
controls via `docker run`:

```bash
docker run \
  --name blogflow \
  --user 65532:65532 \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  -v blogflow-content:/data/content \
  -v blogflow-theme:/data/theme \
  -v blogflow-config:/data/config:ro \
  -v blogflow-cache:/data/cache \
  -e BLOGFLOW_WEBHOOK_SECRET="${BLOGFLOW_WEBHOOK_SECRET}" \
  -p 8080:8080 \
  ghcr.io/your-org/blogflow@sha256:abc123def456...
```

### Flag breakdown

| Flag | Purpose |
|---|---|
| `--user 65532:65532` | Run as nonroot UID/GID |
| `--read-only` | Mount container root filesystem as read-only |
| `--cap-drop ALL` | Drop all Linux capabilities |
| `--security-opt no-new-privileges:true` | Prevent privilege escalation via setuid/setgid |
| `--tmpfs /tmp:rw,noexec,nosuid,size=64m` | RAM-backed /tmp, no executable or setuid files, 64 MB limit |
| `-v ...:ro` | Config volume is read-only |
| `-e BLOGFLOW_WEBHOOK_SECRET=...` | Inject secrets via environment variable, never in the image |
| `ghcr.io/...@sha256:...` | Pin image by digest, not tag |
