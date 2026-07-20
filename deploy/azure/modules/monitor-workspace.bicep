// ============================================================================
// Azure Monitor Workspace — Prometheus metrics destination
// ============================================================================
// Creates an Azure Monitor workspace (Microsoft.Monitor/accounts) for
// Prometheus metrics. This workspace stores time-series data with a cost
// model appropriate for metrics (unlike Log Analytics PerGB2018).
//
// Metrics are routed here through the DCE/DCR resources and a self-managed
// OpenTelemetry Collector sidecar. The collector authenticates to Azure Monitor
// with the Container App's managed identity through the Azure auth extension.
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
