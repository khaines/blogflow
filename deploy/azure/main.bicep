// ============================================================================
// BlogFlow — Azure Container Apps deployment orchestrator
// ============================================================================
// Deploys BlogFlow to Azure Container Apps with:
//   - Serverless Container Apps Environment (Consumption plan)
//   - Log Analytics workspace for container diagnostics
//   - Container App with OTel Collector sidecar
//   - System-assigned managed identity
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

// ---------------------------------------------------------------------------
// Parameters — secrets (must come from GitHub Secrets / CLI --parameters)
// ---------------------------------------------------------------------------

@description('GitHub PAT with read:packages scope for GHCR image pulls')
@secure()
param ghcrPassword string

@description('Application Insights connection string for OTel telemetry export')
@secure()
param appInsightsConnectionString string

// ---------------------------------------------------------------------------
// Module: Container Apps Environment + Log Analytics
// ---------------------------------------------------------------------------
module environment 'modules/container-app-env.bicep' = {
  name: 'container-app-env'
  params: {
    location: location
    environmentName: environmentName
  }
}

// ---------------------------------------------------------------------------
// Module: Container App (BlogFlow + OTel Collector sidecar)
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
    appInsightsConnectionString: appInsightsConnectionString
    scaleMinReplicas: scaleMinReplicas
    scaleMaxReplicas: scaleMaxReplicas
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
