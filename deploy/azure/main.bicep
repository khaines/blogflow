// ============================================================================
// BlogFlow — Azure Container Apps deployment orchestrator
// ============================================================================
// Deploys BlogFlow to Azure Container Apps with:
//   - Serverless Container Apps Environment (Consumption plan)
//   - Log Analytics workspace for container diagnostics
//   - Azure Monitor workspace for Prometheus metrics (Phase 2)
//   - ACA managed OpenTelemetry agent:
//       • Traces → Application Insights
//       • Metrics → NOT exported yet (see Phase 2 in SETUP.md)
//   - Container App with system-assigned managed identity
//
// All account-specific values (subscription, resource group, connection
// strings, credentials) are passed as parameters — never hardcoded.
//
// Usage:
//   az deployment group create \
//     --resource-group <RG_NAME> \
//     --template-file deploy/azure/main.bicep \
//     --parameters deploy/azure/parameters/prod.bicepparam \
//     --parameters ghcrPassword='<PAT>' \
//                  appInsightsConnectionString='<CS>'
// ============================================================================

targetScope = 'resourceGroup'

// ---------------------------------------------------------------------------
// Parameters — non-secret
// ---------------------------------------------------------------------------

@description('Azure region for all resources')
param location string = 'eastus2'

@description('Base name prefix for all resources')
param environmentName string

@description('Full container image reference')
param containerImage string = 'ghcr.io/khaines/blogflow:main'

@description('GitHub username for GHCR image pulls')
param ghcrUsername string = 'khaines'

@description('Minimum replica count (0 = scale to zero)')
@minValue(0)
@maxValue(10)
param scaleMinReplicas int = 0

@description('Maximum replica count')
@minValue(1)
@maxValue(10)
param scaleMaxReplicas int = 2

@description('Custom domain hostname (e.g. www.blogflow.io). Empty = no custom domain.')
param customDomainName string = ''

@description('Managed certificate resource ID for custom domain TLS. Empty = no TLS binding.')
param customDomainCertificateId string = ''

// ---------------------------------------------------------------------------
// Parameters — secrets (must come from GitHub Secrets / CLI --parameters)
// ---------------------------------------------------------------------------

@description('GitHub PAT with read:packages scope for GHCR image pulls')
@secure()
param ghcrPassword string

@description('Application Insights connection string for trace export')
@secure()
param appInsightsConnectionString string

@description('Log Analytics workspace retention in days (7–730). Lower values reduce storage costs.')
@minValue(7)
@maxValue(730)
param logRetentionDays int = 30

// ---------------------------------------------------------------------------
// Module: Azure Monitor Workspace (Prometheus metrics destination)
// ---------------------------------------------------------------------------
module monitorWorkspace 'modules/monitor-workspace.bicep' = {
  name: 'monitor-workspace'
  params: {
    location: location
    environmentName: environmentName
  }
}

// ---------------------------------------------------------------------------
// Module: Container Apps Environment + Log Analytics + OTel routing
// ---------------------------------------------------------------------------
module environment 'modules/container-app-env.bicep' = {
  name: 'container-app-env'
  params: {
    location: location
    environmentName: environmentName
    appInsightsConnectionString: appInsightsConnectionString
    logRetentionDays: logRetentionDays
  }
}

// ---------------------------------------------------------------------------
// Module: Container App (BlogFlow — no sidecar)
// ---------------------------------------------------------------------------
module containerApp 'modules/container-app.bicep' = {
  name: 'container-app'
  params: {
    location: location
    appName: '${environmentName}-app'
    environmentId: environment.outputs.environmentId
    containerImage: containerImage
    ghcrUsername: ghcrUsername
    ghcrPassword: ghcrPassword
    scaleMinReplicas: scaleMinReplicas
    scaleMaxReplicas: scaleMaxReplicas
    customDomainName: customDomainName
    customDomainCertificateId: customDomainCertificateId
  }
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

@description('Container App FQDN — point DNS here')
output containerAppFqdn string = containerApp.outputs.fqdn

@description('Container App resource name')
output containerAppName string = containerApp.outputs.name

@description('Log Analytics workspace ID')
output logAnalyticsWorkspaceId string = environment.outputs.logAnalyticsWorkspaceId

@description('Azure Monitor workspace ID (Prometheus metrics)')
output monitorWorkspaceId string = monitorWorkspace.outputs.workspaceId

@description('Prometheus query endpoint (for Grafana / PromQL dashboards)')
output prometheusQueryEndpoint string = monitorWorkspace.outputs.prometheusQueryEndpoint
