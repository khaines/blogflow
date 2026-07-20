# Azure Container Apps — One-Time Setup

Manual steps required before the GitHub Actions CD workflow can deploy BlogFlow.

---

## 1. Create Resource Group

```bash
az group create --name <rg-name> --location eastus2
```

## 2. Create App Registration for OIDC

```bash
# Create the Azure AD application
az ad app create --display-name "blogflow-github-deploy"

# Note the appId from the output, then create the service principal
az ad sp create --id <APP_ID>
```

## 3. Add Federated Credential (GitHub → Azure OIDC)

```bash
az ad app federated-credential create --id <APP_ID> --parameters '{
  "name": "github-main",
  "issuer": "https://token.actions.githubusercontent.com",
  "subject": "repo:khaines/blogflow:environment:production",
  "audiences": ["api://AzureADTokenExchange"]
}'
```

> **⚠️ Warning:** The `subject` claim must exactly match the repository and branch.
> If you rename the repo or deploy from a different branch, update this credential
> or the OIDC exchange will fail silently.

## 4. Grant Deployment Roles to the Service Principal

The deployment principal creates Azure resources **and** a DCR-scoped role
assignment for the Container App managed identity. `Contributor` alone cannot
create role assignments because it lacks `Microsoft.Authorization/roleAssignments/write`.
Grant `Contributor` plus either `Role Based Access Control Administrator`
(preferred least-privilege role for RBAC management) or `User Access Administrator`
at the target resource group scope.

```bash
az role assignment create \
  --assignee <SP_OBJECT_ID> \
  --role Contributor \
  --scope /subscriptions/<SUB_ID>/resourceGroups/<RG_NAME>

az role assignment create \
  --assignee <SP_OBJECT_ID> \
  --role "Role Based Access Control Administrator" \
  --scope /subscriptions/<SUB_ID>/resourceGroups/<RG_NAME>
```

## 5. Get Application Insights Connection String

If you already have an Application Insights instance (e.g., sharing the
workspace with khainesnet-web), just retrieve the connection string:

```bash
az monitor app-insights component show \
  --app <NAME> \
  --resource-group <RG> \
  --query connectionString -o tsv
```

Otherwise, create one first:

```bash
az monitor app-insights component create \
  --app <NAME> \
  --location eastus2 \
  --resource-group <RG> \
  --kind web \
  --application-type web
```

> **⚠️ Warning:** The connection string contains an instrumentation key.
> Treat it as a secret — do not commit it to source control.

> **ℹ️ Note:** Application Insights is used for **traces only**. Metrics are
> exported through the self-managed OTel Collector sidecar to Azure Monitor for
> Prometheus via DCE/DCR, avoiding the expensive Log Analytics metrics ingestion
> path.

## 6. Create GitHub Environment and Secrets

### Create the environment

Go to **Settings → Environments → New environment** and create `production`.

**Recommended protection rules:**
- ✅ **Required reviewers** — add yourself (approves deploys)
- ✅ **Deployment branches** — select "Selected branches" → add `main`
- This ensures only the `main` branch can deploy, with manual approval

### Add secrets to the environment

In **Settings → Environments → production → Environment secrets**, add:

| Secret | Description |
|---|---|
| `AZURE_CLIENT_ID` | App Registration Application (client) ID from step 2 |
| `AZURE_TENANT_ID` | Azure AD tenant ID (`az account show --query tenantId`) |
| `AZURE_SUBSCRIPTION_ID` | Azure subscription ID (`az account show --query id`) |
| `AZURE_RESOURCE_GROUP` | Resource group name from step 1 |
| `AZURE_ENVIRONMENT_NAME` | Base name prefix for all Azure resources (e.g., `blogflow-prod`) |
| `GHCR_PASSWORD` | GitHub PAT with **`read:packages`** scope only |
| `APPINSIGHTS_CONNECTION_STRING` | Connection string from step 5 |
| `CUSTOM_DOMAIN_NAME` | _(Optional)_ Custom domain hostname, e.g. `www.blogflow.io` (see section 10) |
| `CUSTOM_DOMAIN_CERT_ID` | _(Optional)_ Managed certificate resource ID (see section 10) |

> **⚠️ Warning:** Use environment secrets (not repository secrets) — environment
> secrets are scoped to the `production` environment and only accessible from
> workflows that declare `environment: production`.
>
> **⚠️ Warning:** `GHCR_PASSWORD` only needs the `read:packages` scope.
> Do not use a PAT with broader permissions than necessary.

## 7. First Deploy

1. Go to **Actions → Deploy → Run workflow**
2. Select the **main** branch
3. Check **full_deploy** (required for first deploy)
4. Click **Run workflow**

The deployment creates:
- **Log Analytics workspace** — container diagnostics (default 30-day retention)
- **Azure Monitor workspace** — Prometheus metrics destination
- **Data Collection Endpoint + Rule** — OTLP metrics ingestion pipeline
- **Container Apps Environment** — runtime host and container diagnostics only
- **User-assigned managed identity** — attached to the Container App for OTel
  Collector authentication
- **DCR-scoped role assignment** — grants that identity
  **Monitoring Metrics Publisher** on the Data Collection Rule
- **Prometheus rule group** — alerts when no `blogflow_*` metrics are ingested
- **Container App** — BlogFlow plus an OTel Collector sidecar

The workflow also runs automatically when the **Publish** workflow completes
on main (i.e., after a new container image is pushed to GHCR).

> **ℹ️ Note: Revision cleanup.** The container app runs in `Multiple`
> active-revisions mode, so each deploy creates a new revision. After a
> successful deploy, the workflow's `Deactivate superseded revisions` step waits
> for the newest revision to report `Healthy`, then deactivates every older
> zero-traffic revision so only the current deployment stays provisioned. A
> failed/unhealthy rollout leaves the prior revisions Active as a fallback (and
> fails the deploy job). To inspect revisions manually:
>
> ```bash
> az containerapp revision list \
>   --name <app-name> --resource-group <rg-name> \
>   --query "[].{name:name, active:properties.active, traffic:properties.trafficWeight, health:properties.healthState}" \
>   -o table
> ```

## 8. Verify Metrics Are Routed to Azure Monitor (Post-Deploy)

After the first deploy, generate a little traffic so BlogFlow emits metrics:

```bash
curl -fsS https://<container-app-fqdn>/healthz
curl -fsS https://<container-app-fqdn>/readyz
```

Confirm metrics are **not** going to Log Analytics / Application Insights:

```bash
az monitor app-insights query \
  --app <APP_INSIGHTS_NAME> \
  --resource-group <RG> \
  --analytics-query "AppMetrics | where TimeGenerated > ago(1h) | count"
```

Then query the Azure Monitor workspace using the deployment output
`prometheusQueryEndpoint`. Query a concrete BlogFlow series (after hitting the app so the counter exists):

```bash
PROM_ENDPOINT="$(az deployment group show \
  --resource-group <RG> \
  --name <DEPLOYMENT_NAME> \
  --query properties.outputs.prometheusQueryEndpoint.value -o tsv)"

az rest --method get \
  --url "${PROM_ENDPOINT}/api/v1/query?query=blogflow_http_requests_total"
```

A successful PromQL response containing `blogflow_http_requests_total` confirms
the app → collector → DCE/DCR → Azure Monitor workspace path is working. If the
query is empty, first verify the app has received traffic and then check the
`otel-collector` container logs for auth or export errors.

## 9. Phase 2: Metrics Export via Self-Managed OTel Collector Sidecar

Metrics export is enabled through Azure Monitor native OTLP ingestion, but **not**
through the ACA managed OpenTelemetry agent. The managed agent's OTLP destination
configuration supports static headers/API keys and does not acquire the Microsoft
Entra bearer token required by the DCE. Managed-identity authentication therefore
requires a self-managed OpenTelemetry Collector sidecar.

The deployed flow is:

1. **BlogFlow app container** — exports traces and metrics over OTLP/HTTP to
   `http://localhost:4318` inside the Container App replica.
2. **OTel Collector sidecar** — image
   `otel/opentelemetry-collector-contrib:0.148.0@sha256:8164eab2e6bca9c9b0837a8d2f118a6618489008a839db7f9d6510e66be3923c`
   receives OTLP on localhost ports 4317/4318 and exposes a health endpoint on
   port 13133 for the ACA liveness probe.
3. **Traces** — exported to Application Insights using the collector's
   `azuremonitor` exporter and the `APPLICATIONINSIGHTS_CONNECTION_STRING`
   secret.
4. **Metrics** — exported with `otlphttp` to the DCE metrics-ingestion endpoint:
   `.../datacollectionRules/<dcr-immutable-id>/streams/Custom-Metrics-Otel/otlp/v1/metrics`.
5. **Authentication** — the collector uses the contrib `azure_auth` extension,
   a user-assigned managed identity attached to the Container App, and the token
   scope `https://monitor.azure.com/.default`.
6. **Authorization** — Bicep grants **Monitoring Metrics Publisher** to that
   managed identity at the DCR scope only, using a deterministic `guid()` role
   assignment and `principalType: 'ServicePrincipal'`.

### Collector config delivery

Azure Container Apps does not provide a simple config-file mount for this sidecar,
so Bicep passes the collector YAML in the `OTELCOL_CONFIG` environment variable
and starts the collector with:

```bash
otelcol-contrib --config=env:OTELCOL_CONFIG
```

This keeps the deployment self-contained and avoids building a custom collector
image. The assumption is that the collector config remains small enough for ACA
environment-variable limits; if the config grows substantially, switch to a small
custom image with the YAML baked in.

### Collector image pinning

The collector image is assembled from two Bicep parameters instead of one free-form
image string:

- `otelCollectorImageRepository` — repository plus tag, default
  `otel/opentelemetry-collector-contrib:0.148.0`; do not include `@` or an
  embedded digest
- `otelCollectorImageDigest` — 64-character lowercase-hex SHA-256 digest without
  the `sha256:` prefix

The deployed image is always rendered as
`<repository>@sha256:<digest>`, so a tag-only collector override cannot be
deployed accidentally. The Bicep template rejects an embedded digest in the repository parameter and
requires the digest to match `^[0-9a-f]{64}$` before rendering the final image
reference. `deploy/azure/validate-collector-image-pin.py` also runs in CI/deploy
validation to catch bad checked-in defaults before deployment. Update both
parameters together when intentionally moving to a newer collector release.

### Collector health probes and bind address

The `otel-collector` sidecar has Startup, Readiness, and Liveness probes against
the collector `health_check` extension on port 13133. Startup gives the collector
up to two minutes to initialize, Readiness gates the Container App revision until
the sidecar health endpoint is responding, and Liveness restarts a wedged
collector. The Readiness probe intentionally has no `initialDelaySeconds` because
the Startup probe gates it.

The health endpoint confirms the collector service and configured extensions are
running, but it does **not** wait for the `azure_auth` extension to acquire an
Entra token or prove the DCE export path. Early 401/403s during managed-identity
RBAC propagation can still occur after Readiness succeeds. That cold-start loss
window is mitigated by the configured `batch` processor plus `otlphttp`
`retry_on_failure` backoff (`initial_interval: 5s`, `max_interval: 30s`,
`max_elapsed_time: 10m`), not by the probe itself.

The collector `health_check` extension intentionally binds `0.0.0.0:13133` rather
than `localhost:13133`. Azure Container Apps probes originate from the platform
against the container, not from inside the collector process; loopback-only
listeners are not reliably reachable by ACA HTTP probes. The port is still only
used as an in-replica health endpoint and is not exposed through public ingress.

### DCR stream name

The DCR uses the native OTLP custom metrics stream `Custom-Metrics-Otel` for the
Azure Monitor workspace (`monitoringAccounts`) destination. The collector uses
the Azure Monitor OTLP `metrics_endpoint` form documented by Microsoft, with the
DCE metrics endpoint, DCR immutable ID, and the same case-sensitive stream name:

```text
https://<metrics-dce-domain>/datacollectionRules/<dcr-immutable-id>/streams/Custom-Metrics-Otel/otlp/v1/metrics
```

The stream name must exactly match the DCR `dataFlows` stream. A mismatch can
result in silently dropped metrics.

### DCE network exposure

The DCE defaults to `dcePublicNetworkAccess=Enabled` for compatibility with the
standard Container Apps egress path to Azure Monitor ingestion. In this mode,
ingestion is still gated by Microsoft Entra authorization: the collector's
user-assigned managed identity must have **Monitoring Metrics Publisher** on the
DCR scope, and no static OTLP API keys are configured.

For stricter threat models, set `dcePublicNetworkAccess=Disabled` and provide a
private endpoint / private DNS path that allows the Container App environment to
reach the DCE metrics ingestion endpoint. Do not disable public network access
until that private ingestion path is in place, or collector exports will fail.

### Temporality

Azure Monitor workspace / Prometheus metrics are cumulative, so the app sets
`OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=cumulative`. The collector does
not use `cumulativetodelta`; its metrics pipeline starts with `memory_limiter`,
then `batch`, preserving cumulative temporality.

### Rollout ordering and RBAC propagation

The Container App module creates the user-assigned managed identity, creates the
DCR-scoped `Monitoring Metrics Publisher` role assignment, and the Container App
resource explicitly depends on that role assignment before creating the app
revision. The app module consumes DCR and Container Apps Environment outputs, so
Bicep orders those modules first. Microsoft Entra RBAC propagation can still lag
after role assignment creation, so initial collector samples may receive 401/403
during the first few minutes. The collector `otlphttp` exporter has
`retry_on_failure` enabled with bounded backoff to reduce early metric loss while
RBAC propagates.

See [Azure Monitor OTLP ingestion docs](https://learn.microsoft.com/en-us/azure/azure-monitor/containers/opentelemetry-protocol-ingestion)
for the DCE/DCR ingestion model and the Entra-authenticated collector pattern.

### Metrics ingestion absence alert

Bicep deploys a Prometheus rule group named
`<environmentName>-metrics-ingestion` by default. The alert
`BlogFlowMetricsIngestionAbsent` evaluates this per-environment PromQL expression every minute:

```promql
absent_over_time({__name__=~"blogflow_.*",deployment_environment="<environmentName>"}[30m])
```

The app container sets `OTEL_RESOURCE_ATTRIBUTES=deployment.environment=<environmentName>`.
OTLP resource attributes are not automatically labels on every metric series, so
the collector metrics pipeline includes a targeted `transform/promote_env`
processor that copies that single resource attribute onto each datapoint as
`deployment_environment`. This keeps shared Azure Monitor workspaces from masking
one environment's broken pipeline with another environment's healthy
`blogflow_*` series.

Verify the promoted label after deploy with:

```promql
count by (deployment_environment) (blogflow_http_requests_total)
```

The result should include the current `<environmentName>` value before enabling
or depending on the absence alert.

If the expression remains active for `PT5M`, Azure Monitor raises a severity
2 alert indicating no BlogFlow custom metrics have arrived for the previous 30
minutes. This is intended to catch persistent collector authentication/export
failures, including 401/403 responses that otherwise appear only in
`otel-collector` logs.

Optional parameters:

- `enableMetricsIngestionAbsenceAlert` — default `true`; the deployed rule is
  effectively enabled only when `scaleMinReplicas > 0`. Bicep always deploys the
  rule group and passes the effective boolean to `properties.enabled`, so toggling
  this parameter to `false` disables an existing alert during incremental deploys
  instead of leaving a previously-created rule active.
- `metricsIngestionAbsenceActionGroupId` — Azure Monitor action group resource
  ID for notifications. Leave empty to create the alert rule without notification
  actions, then attach actions later in Azure Monitor.

If `scaleMinReplicas=0`, the rule group is created but disabled by default to
avoid idle scale-to-zero false positives. To alert for a scale-to-zero app, keep
`enableMetricsIngestionAbsenceAlert=true`, set `scaleMinReplicas > 0`, or add an
external synthetic check that keeps metrics flowing before enabling the rule.

## Rollback: Disable DCE Metrics Export

If the collector-sidecar metrics path fails (preview API instability, collector
image regression, RBAC propagation issue, or Azure ingestion outage), temporarily
redeploy with `OTEL_METRICS_EXPORTER` unset in `container-app.bicep` to stop
metrics export while keeping BlogFlow serving traffic and traces flowing to
Application Insights. Re-enable metrics after fixing the collector/DCR issue.

> **⚠️ Warning:** Do not roll back to exporting metrics with the collector's
> `azuremonitor` exporter unless you intentionally accept App Insights → Log
> Analytics `AppMetrics` ingestion costs.

## 10. Custom Domain + TLS (Optional)

After the first deploy succeeds:

### DNS Setup

1. Get the Container App FQDN from the deploy workflow output or Azure Portal
2. Add a **CNAME** record at your DNS provider:
   ```
   www.blogflow.io  CNAME  <container-app-fqdn>
   ```
   > **Note:** GoDaddy doesn't support CNAME on apex domains. Use `www` subdomain
   > + domain forwarding (`blogflow.io` → `https://www.blogflow.io`).

### Bind Domain + Managed TLS Certificate

```bash
# 1. Add the hostname
az containerapp hostname add \
  --name <app-name> \
  --resource-group <rg-name> \
  --hostname www.blogflow.io

# 2. Bind with managed certificate (free, auto-renewed)
az containerapp hostname bind \
  --name <app-name> \
  --resource-group <rg-name> \
  --hostname www.blogflow.io \
  --environment <env-name> \
  --validation-method CNAME
```

### Make it Reproducible via Bicep (optional)

After the managed certificate is provisioned, get its resource ID:
```bash
az containerapp env certificate list \
  --name <env-name> \
  --resource-group <rg-name> \
  --query "[?properties.subjectName=='www.blogflow.io'].id" -o tsv
```

Then add these environment secrets:
- `CUSTOM_DOMAIN_NAME` = `www.blogflow.io`
- `CUSTOM_DOMAIN_CERT_ID` = the certificate resource ID

And pass them in the deploy workflow's full Bicep step:
```
customDomainName="${{ secrets.CUSTOM_DOMAIN_NAME }}"
customDomainCertificateId="${{ secrets.CUSTOM_DOMAIN_CERT_ID }}"
```

This ensures the domain binding survives a full infrastructure redeploy.

---

## Architecture: Telemetry Flow

```text
BlogFlow app ──OTLP localhost:4318──▶ OTel Collector sidecar ── traces ──▶ App Insights ──▶ LA workspace
                                                   │                         (AppTraces only)
                                                   └─ metrics + Entra token ─▶ DCE ─▶ DCR ─▶ Azure Monitor workspace

Container Apps runtime ──console logs──▶ Log Analytics workspace (default 30-day retention)
```

**Key design decisions:**
- Metrics do NOT go to App Insights or Log Analytics (fixes the cost issue)
- Traces go to App Insights → Log Analytics `AppTraces` table (acceptable cost)
- Metrics go to Azure Monitor workspace through DCE/DCR OTLP ingestion
- The app sets `OTEL_SERVICE_NAME=blogflow` and
  `OTEL_RESOURCE_ATTRIBUTES=deployment.environment=<environmentName>`; the
  collector `transform/promote_env` processor promotes that resource attribute to
  the per-series `deployment_environment` metric label
- The collector exposes `health_check` on port 13133 for Startup, Readiness,
  and Liveness probing and starts telemetry pipelines with `memory_limiter` to
  reduce OOM risk
- The collector uses the Container App's user-assigned managed identity and the
  Azure auth extension to acquire Entra tokens for Azure Monitor
- The DCR authorizes that identity with Monitoring Metrics Publisher at DCR
  scope only; no static OTLP API keys are stored
- The DCE defaults to public network access for compatibility; use
  `dcePublicNetworkAccess=Disabled` only with a working private ingestion path
- A Prometheus rule group alerts when environment-scoped `blogflow_*` metrics
  disappear from the Azure Monitor workspace; it is disabled automatically when
  `scaleMinReplicas=0` to avoid idle false positives
