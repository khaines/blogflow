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

## 4. Grant Contributor Role to the Service Principal

```bash
az role assignment create \
  --assignee <SP_OBJECT_ID> \
  --role Contributor \
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
> exported through the ACA managed OTel agent to Azure Monitor for Prometheus
> via DCE/DCR, avoiding the expensive Log Analytics metrics ingestion path.

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
- **Container Apps Environment** — with managed OTel agent routing:
  - Traces → Application Insights
  - Metrics → DCE/DCR → Azure Monitor workspace
- **Container App** — BlogFlow (single container, no sidecar)

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

After the first deploy, confirm metrics are **not** going to Log Analytics and
then verify that the Azure Monitor workspace receives Prometheus metrics.

```bash
# Check that AppMetrics table is NOT receiving new data in Application Insights / Log Analytics
az monitor app-insights query \
  --app <APP_INSIGHTS_NAME> \
  --resource-group <RG> \
  --analytics-query "AppMetrics | where TimeGenerated > ago(1h) | count"
```

Use the `prometheusQueryEndpoint` deployment output with Azure Managed Grafana
or another PromQL client to confirm BlogFlow metrics appear in the Azure Monitor
workspace after the app has emitted metrics for a few minutes.

## 9. Phase 2: Metrics Export via DCE/DCR

Metrics export is enabled through Azure Monitor native OTLP ingestion:

1. **Data Collection Endpoint (DCE)** — exposes the OTLP metrics ingestion URL
2. **Data Collection Rule (DCR)** — routes the `Custom-Metrics-Otel` stream to
   the Azure Monitor workspace created by `monitor-workspace.bicep`
3. **Managed identity authentication** — the Container Apps Environment has a
   system-assigned managed identity, and Bicep grants it the
   **Monitoring Metrics Publisher** role on the DCR
4. **ACA OTLP configuration** — `container-app-env.bicep` adds an
   `azureMonitorMetrics` OTLP destination under
   `openTelemetryConfiguration.destinationsConfiguration.otlpConfigurations`
   and references it from `metricsConfiguration.destinations`
5. **BlogFlow metrics exporter** — `container-app.bicep` sets
   `OTEL_METRICS_EXPORTER=otlp`, so the app exports metrics to the ACA managed
   OTel agent instead of leaving metrics disabled

No static API keys are configured. The deployment assumes the ACA managed OTel
agent can use the environment's managed identity when sending to the DCE/DCR
endpoint. The managed OTel agent and its `2024-10-02-preview` API remain preview
features; validate ingestion in Azure after deployment.

See [Azure Monitor OTLP ingestion docs](https://learn.microsoft.com/en-us/azure/azure-monitor/containers/opentelemetry-protocol-ingestion)
for the DCE/DCR ingestion model.

## Rollback: Revert to OTel Collector Sidecar

If the ACA managed OTel agent fails to deliver traces (preview API instability,
misconfiguration, or Azure outage), follow these steps to revert:

1. In `container-app-env.bicep`: revert API to `2024-03-01`, remove
   `appInsightsConfiguration` and `openTelemetryConfiguration`
2. In `container-app.bicep`: re-add the OTel Collector sidecar container
   definition (see `otel/collector-config.yaml` for the original config),
   restore the `appinsights-cs` secret, and add back `OTEL_SERVICE_NAME`,
   `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_METRICS_EXPORTER=otlp`, and
   `BLOGFLOW_METRICS_PORT=9090` env vars
3. Restore `otel/collector-config.yaml` to its active (uncommented) state
4. Redeploy with `full_deploy=true`

> **⚠️ Warning:** Rolling back re-enables metrics export to App Insights →
> Log Analytics, which will resume the PerGB2018 ingestion costs this change
> was designed to eliminate. Use as a temporary measure only.

> **Note**: The original sidecar config is preserved (commented out) in
> `otel/collector-config.yaml` for reference.

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

```
BlogFlow app ──OTLP──▶ ACA managed OTel agent ──── traces ──▶ App Insights ──▶ LA workspace
                                      │                                         (AppTraces only)
                                      └──── metrics ──▶ DCE ──▶ DCR ──▶ Azure Monitor workspace

Container Apps runtime ──console logs──▶ Log Analytics workspace (default 30-day retention)
```

**Key design decisions:**
- Metrics do NOT go to App Insights or Log Analytics (fixes the cost issue)
- Traces go to App Insights → Log Analytics `AppTraces` table (acceptable cost)
- Metrics go to Azure Monitor workspace through DCE/DCR OTLP ingestion
- The DCR authorizes the Container Apps Environment managed identity with the
  Monitoring Metrics Publisher role; no static OTLP API keys are stored
