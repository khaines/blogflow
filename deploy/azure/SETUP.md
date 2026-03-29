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

> **⚠️ Warning:** Use environment secrets (not repository secrets) — environment
> secrets are scoped to the `production` environment and only accessible from
> workflows that declare `environment: production`.
>
> **⚠️ Warning:** `GHCR_PASSWORD` only needs the `read:packages` scope.
> Do not use a PAT with broader permissions than necessary.

## 7. First Deploy

1. Go to **Actions → Deploy → Run workflow**
2. Select the **main** branch
3. Click **Run workflow**

The workflow also runs automatically when the **Publish** workflow completes
on main (i.e., after a new container image is pushed to GHCR).

## 8. Custom Domain (Optional)

After the first deploy succeeds:

1. Get the Container App FQDN from the Azure Portal (or from the workflow output)
2. Add a **CNAME** record with your DNS provider:
   ```
   blogflow.io  CNAME  <container-app-fqdn>
   ```
3. In the Azure Portal, navigate to the Container App → **Custom domains**
4. Add the custom domain and enable **Managed certificate** for automatic TLS
