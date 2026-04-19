// ============================================================================
// Azure Monitor Workspace — Prometheus metrics destination
// ============================================================================
// Creates an Azure Monitor workspace (Microsoft.Monitor/accounts) for
// Prometheus metrics. This workspace stores time-series data with a cost
// model appropriate for metrics (unlike Log Analytics PerGB2018).
//
// Phase 1 (current): The workspace is provisioned but metrics are NOT yet
//   routed here. The managed OTel agent only exports traces → App Insights.
//   Metrics export requires DCE/DCR infrastructure and Entra ID auth, which
//   is a Phase 2 follow-up.
//
// Phase 2 (future): Set up Data Collection Endpoint + Data Collection Rule
//   for OTLP metrics ingestion, then configure the ACA managed OTel agent
//   to export metrics to the DCE endpoint with proper authentication.
// ============================================================================

@description('Azure region')
param location string

@description('Base name prefix for resources')
param environmentName string

// ---------------------------------------------------------------------------
// Azure Monitor Workspace
// ---------------------------------------------------------------------------
resource monitorWorkspace 'Microsoft.Monitor/accounts@2023-04-03' = {
  name: '${environmentName}-metrics'
  location: location
  properties: {}
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

@description('Azure Monitor workspace resource ID')
output workspaceId string = monitorWorkspace.id

@description('Prometheus query endpoint (for Grafana / PromQL)')
output prometheusQueryEndpoint string = monitorWorkspace.properties.metrics.prometheusQueryEndpoint
