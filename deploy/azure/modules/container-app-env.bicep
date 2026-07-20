// ============================================================================
// Container Apps Environment — Consumption plan + Log Analytics + OTel routing
// ============================================================================
// Creates a serverless Container Apps Environment backed by:
//   - Log Analytics workspace for container diagnostics and log streaming
//   - Managed OpenTelemetry agent that routes:
//       • Traces → Application Insights
//       • Metrics → DCE/DCR → Azure Monitor workspace
//       • Logs → NOT exported via OTel (container logs go directly to LA)
//
// Uses 2024-10-02-preview API for openTelemetryConfiguration support.
// Metrics use the environment's system-assigned managed identity; this module
// grants it Monitoring Metrics Publisher on the DCR.
// ============================================================================

@description('Azure region')
param location string

@description('Base name prefix for resources')
param environmentName string

@description('Application Insights connection string for trace export')
@secure()
param appInsightsConnectionString string

@description('Log Analytics workspace retention in days (7–730). Lower values reduce storage costs.')
@minValue(7)
@maxValue(730)
param logRetentionDays int = 30

@description('Full OTLP metrics endpoint exposed by the Data Collection Endpoint and Rule')
param otlpMetricsEndpoint string

@description('Data Collection Rule resource name for OTLP metrics ingestion')
param dataCollectionRuleName string

var monitoringMetricsPublisherRoleDefinitionId = '3913510d-42f4-4e42-8a64-420c390055eb'

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
// Container Apps Environment (Consumption tier) with managed OTel agent
// ---------------------------------------------------------------------------
resource metricsDataCollectionRule 'Microsoft.Insights/dataCollectionRules@2024-03-11' existing = {
  name: dataCollectionRuleName
}

resource environment 'Microsoft.App/managedEnvironments@2024-10-02-preview' = {
  name: '${environmentName}-env'
  location: location
  identity: {
    type: 'SystemAssigned'
  }
  properties: {
    appLogsConfiguration: {
      destination: 'log-analytics'
      logAnalyticsConfiguration: {
        customerId: logAnalytics.properties.customerId
        sharedKey: logAnalytics.listKeys().primarySharedKey
      }
    }

    // --- Application Insights for traces ---
    appInsightsConfiguration: {
      connectionString: appInsightsConnectionString
    }

    // --- Managed OpenTelemetry agent configuration ---
    // Traces → App Insights. Metrics → Azure Monitor workspace via OTLP DCE/DCR.
    openTelemetryConfiguration: {
      destinationsConfiguration: {
        otlpConfigurations: [
          {
            name: 'azureMonitorMetrics'
            endpoint: otlpMetricsEndpoint
            insecure: false
          }
        ]
      }
      tracesConfiguration: {
        destinations: ['appInsights']
      }
      metricsConfiguration: {
        destinations: ['azureMonitorMetrics']
      }
      logsConfiguration: {
        destinations: []
      }
    }
  }
}

resource metricsPublisherRoleAssignment 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name: guid(metricsDataCollectionRule.id, environment.name, monitoringMetricsPublisherRoleDefinitionId)
  scope: metricsDataCollectionRule
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', monitoringMetricsPublisherRoleDefinitionId)
    principalId: environment.identity.principalId
    principalType: 'ServicePrincipal'
  }
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

@description('Container Apps Environment resource ID')
output environmentId string = environment.id

@description('Log Analytics workspace resource ID')
output logAnalyticsWorkspaceId string = logAnalytics.id

@description('System-assigned managed identity principal ID for the Container Apps Environment')
output environmentPrincipalId string = environment.identity.principalId
