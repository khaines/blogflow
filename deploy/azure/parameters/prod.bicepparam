// ============================================================================
// BlogFlow — Production parameters (non-secret values only)
// ============================================================================
// Secret values MUST be provided via GitHub Secrets → az CLI --parameters:
//
//   Required GitHub Secrets:
//     AZURE_CLIENT_ID          — Service principal client ID (OIDC federation)
//     AZURE_TENANT_ID          — Azure AD tenant ID
//     AZURE_SUBSCRIPTION_ID    — Azure subscription ID
//     AZURE_RESOURCE_GROUP     — Target resource group name
//     AZURE_ENVIRONMENT_NAME   — Base name prefix for all Azure resources
//     GHCR_PASSWORD            — GitHub PAT with read:packages scope
//     APPINSIGHTS_CONNECTION_STRING — Application Insights connection string
//
//   Optional non-secret overrides:
//     otelCollectorImageRepository — repository plus tag, no @/digest
//     otelCollectorImageDigest     — 64-character lowercase hex digest, no sha256: prefix
//     dcePublicNetworkAccess       — Enabled (default) or Disabled with private endpoint
//     enableMetricsIngestionAbsenceAlert — true (default); false for scale-to-zero
//     metricsIngestionAbsenceActionGroupId — optional Azure Monitor action group ID
//
// ============================================================================

using '../main.bicep'

param location = 'eastus2'
param scaleMinReplicas = 0
param scaleMaxReplicas = 2
