// ============================================================================
// Container Apps Environment — Consumption plan + Log Analytics + OTel routing
// ============================================================================
// Creates a serverless Container Apps Environment backed by:
//   - Log Analytics workspace for container diagnostics and log streaming
//   - Managed OpenTelemetry agent that routes:
//       • Traces → Application Insights
//       • Metrics → NOT exported (Phase 2: DCE/DCR → Azure Monitor workspace)
//       • Logs → NOT exported via OTel (container logs go directly to LA)
//
// Uses 2024-10-02-preview API for openTelemetryConfiguration support.
//
// Why metrics are not exported yet:
//   ACA managed OTel agent only supports static-header OTLP auth, but Azure
//   Monitor OTLP ingestion requires Entra ID bearer tokens. Proper metrics
//   export needs DCE + DCR infrastructure (Phase 2). Meanwhile, the app still
//   exposes /metrics on port 8080 for manual or future Prometheus scraping.
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
resource environment 'Microsoft.App/managedEnvironments@2024-10-02-preview' = {
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

    // --- Application Insights for traces ---
    appInsightsConfiguration: {
      connectionString: appInsightsConnectionString
    }

    // --- Managed OpenTelemetry agent configuration ---
    // Traces → App Insights. Metrics are NOT routed through the managed agent.
    // App Insights does NOT accept metrics via the managed OTel agent (by design).
    // OTLP metrics export to Azure Monitor workspace requires DCE/DCR with
    // Entra ID auth, which is not yet supported by ACA's static-header OTLP
    // configuration. See Phase 2 in SETUP.md.
    openTelemetryConfiguration: {
      tracesConfiguration: {
        destinations: ['appInsights']
      }
      metricsConfiguration: {
        destinations: []
      }
      logsConfiguration: {
        destinations: []
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
