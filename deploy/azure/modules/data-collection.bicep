// ============================================================================
// Data Collection — OTLP metrics ingestion for Azure Monitor workspace
// ============================================================================
// Creates the Azure Monitor Data Collection Endpoint (DCE) and Data Collection
// Rule (DCR) used by the ACA managed OpenTelemetry agent to send OTLP metrics
// to the Azure Monitor workspace. Authentication is handled by Entra ID via
// role assignment in main.bicep; no static API keys are configured here.
// ============================================================================

@description('Azure region')
param location string

@description('Base name prefix for resources')
param environmentName string

@description('Azure Monitor workspace resource ID for Prometheus metrics')
param monitorWorkspaceId string

var otelMetricsStreamName = 'Custom-Metrics-Otel'
var monitorWorkspaceDestinationName = 'azureMonitorWorkspace'

// ---------------------------------------------------------------------------
// Data Collection Endpoint (OTLP ingestion)
// ---------------------------------------------------------------------------
resource dataCollectionEndpoint 'Microsoft.Insights/dataCollectionEndpoints@2024-03-11' = {
  name: '${environmentName}-metrics-dce'
  location: location
  properties: {
    description: 'OTLP metrics ingestion endpoint for BlogFlow'
    networkAcls: {
      publicNetworkAccess: 'Enabled'
    }
  }
}

// ---------------------------------------------------------------------------
// Data Collection Rule (route OTLP metrics to Azure Monitor workspace)
// ---------------------------------------------------------------------------
resource dataCollectionRule 'Microsoft.Insights/dataCollectionRules@2024-03-11' = {
  name: '${environmentName}-metrics-dcr'
  location: location
  properties: {
    description: 'Routes BlogFlow OTLP metrics to the Azure Monitor workspace'
    dataCollectionEndpointId: dataCollectionEndpoint.id
    directDataSources: {
      otelMetrics: [
        {
          name: 'blogflowOtelMetrics'
          streams: [
            otelMetricsStreamName
          ]
          enrichWithResourceAttributes: [
            '*'
          ]
        }
      ]
    }
    destinations: {
      monitoringAccounts: [
        {
          name: monitorWorkspaceDestinationName
          accountResourceId: monitorWorkspaceId
        }
      ]
    }
    dataFlows: [
      {
        streams: [
          otelMetricsStreamName
        ]
        destinations: [
          monitorWorkspaceDestinationName
        ]
      }
    ]
  }
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

@description('Data Collection Endpoint resource ID')
output dataCollectionEndpointId string = dataCollectionEndpoint.id

@description('Data Collection Rule resource name')
output dataCollectionRuleName string = dataCollectionRule.name

@description('Data Collection Rule resource ID')
output dataCollectionRuleId string = dataCollectionRule.id

@description('Data Collection Rule immutable ID used in OTLP ingestion URLs')
output dataCollectionRuleImmutableId string = dataCollectionRule.properties.immutableId

@description('DCE logs ingestion endpoint')
output logsIngestionEndpoint string = dataCollectionEndpoint.properties.logsIngestion.endpoint

@description('DCE metrics ingestion endpoint')
output metricsIngestionEndpoint string = dataCollectionEndpoint.properties.metricsIngestion.endpoint

@description('Full OTLP metrics endpoint for the ACA managed OpenTelemetry agent')
output otlpMetricsEndpoint string = '${dataCollectionEndpoint.properties.metricsIngestion.endpoint}/dataCollectionRules/${dataCollectionRule.properties.immutableId}/streams/${otelMetricsStreamName}/otlp/v1/metrics'
