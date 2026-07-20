// ============================================================================
// Container Apps Environment — Consumption plan + Log Analytics
// ============================================================================
// Creates a serverless Container Apps Environment backed by a Log Analytics
// workspace for container diagnostics and log streaming. Application telemetry
// is emitted to the self-managed OpenTelemetry Collector sidecar in the
// Container App; the ACA managed OpenTelemetry agent is intentionally not used
// because its OTLP destinations support only static headers/API keys.
// ============================================================================

@description('Azure region')
param location string

@description('Base name prefix for resources')
param environmentName string

@description('Log Analytics workspace retention in days (7–730). Lower values reduce storage costs.')
@minValue(7)
@maxValue(730)
param logRetentionDays int = 30

// ---------------------------------------------------------------------------
// Log Analytics Workspace
// ---------------------------------------------------------------------------
resource logAnalytics 'Microsoft.OperationalInsights/workspaces@2023-09-01' = {
  name: '${environmentName}-logs'
  location: location
  properties: {
    sku: {
      name: 'PerGB2018'
    }
    retentionInDays: logRetentionDays
  }
}

// ---------------------------------------------------------------------------
// Container Apps Environment (Consumption tier)
// ---------------------------------------------------------------------------
resource environment 'Microsoft.App/managedEnvironments@2024-03-01' = {
  name: '${environmentName}-env'
  location: location
  properties: {
    appLogsConfiguration: {
      destination: 'log-analytics'
      logAnalyticsConfiguration: {
        customerId: logAnalytics.properties.customerId
        sharedKey: logAnalytics.listKeys().primarySharedKey
      }
    }
  }
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

@description('Container Apps Environment resource ID')
output environmentId string = environment.id

@description('Log Analytics workspace resource ID')
output logAnalyticsWorkspaceId string = logAnalytics.id
