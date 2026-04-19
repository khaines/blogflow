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

> **ℹ️ Note:** Application Insights is now used for **traces only**. Metrics
> are NOT exported through the managed OTel agent (Phase 2 will route them to
> Azure Monitor for Prometheus). This stops the expensive Log Analytics
> ingestion for metrics that was running up charges.

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
- **Log Analytics workspace** — container diagnostics (7-day retention)
- **Azure Monitor workspace** — Prometheus metrics destination (provisioned for Phase 2)
- **Container Apps Environment** — with managed OTel agent routing:
  - Traces → Application Insights
  - Metrics → not exported yet (Phase 2: DCE/DCR pipeline)
- **Container App** — BlogFlow (single container, no sidecar)

The workflow also runs automatically when the **Publish** workflow completes
on main (i.e., after a new container image is pushed to GHCR).

## 8. Verify Metrics Are NOT Going to Log Analytics (Post-Deploy)

After the first deploy, confirm metrics are **not** going to Log Analytics:

```bash
# Check that AppMetrics table is NOT receiving new data
az monitor app-insights query \
  --app <APP_INSIGHTS_NAME> \
  --resource-group <RG> \
  --analytics-query "AppMetrics | where TimeGenerated > ago(1h) | count"
```

> **ℹ️ Note:** Metrics are not currently exported via the OTel agent (Phase 2).
> BlogFlow still exposes a `/metrics` endpoint on port 8080 for manual
> inspection or future Prometheus scraping.

## 9. Phase 2: Enable Metrics Export (Future)

To route metrics to Azure Monitor for Prometheus, you need:

1. **Data Collection Endpoint (DCE)** — OTLP ingestion endpoint for metrics
2. **Data Collection Rule (DCR)** — routes metrics to the Azure Monitor workspace
3. **Entra ID authentication** — grant "Monitoring Metrics Publisher" role to the
   environment's managed identity on the DCR
4. **ACA OTLP configuration** — add the DCE endpoint as an `otlpConfiguration`
   in the environment's `openTelemetryConfiguration.destinationsConfiguration.otlpConfigurations`
   array, and reference it by name in `metricsConfiguration.destinations`
5. **Re-enable metrics in BlogFlow** — set `OTEL_METRICS_EXPORTER=otlp` on the container app
   (currently unset to skip metrics initialization entirely)

See [Azure Monitor OTLP ingestion docs](https://learn.microsoft.com/en-us/azure/azure-monitor/containers/opentelemetry-protocol-ingestion)
for the full DCE/DCR setup procedure.

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
                                                                                (AppTraces only)

BlogFlow app ──── /metrics (port 8080) ──── available for future Prometheus scraping

Azure Monitor workspace ──── provisioned, awaiting Phase 2 DCE/DCR setup

Container Apps runtime ──console logs──▶ Log Analytics workspace (7-day retention)
```

**Key design decisions:**
- Metrics do NOT go to App Insights or Log Analytics (fixes the cost issue)
- Traces go to App Insights → Log Analytics `AppTraces` table (acceptable cost)
- Azure Monitor workspace is provisioned for future Prometheus metrics storage
- Phase 2 will add DCE/DCR infrastructure for OTLP metrics ingestion with
  Entra ID authentication
