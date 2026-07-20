// ============================================================================
// Metrics Alerts — Azure Monitor workspace / Prometheus rule groups
// ============================================================================
// Detects persistent absence of BlogFlow custom metric series. Collector export
// auth failures (401/403) are visible in sidecar logs, but this alert surfaces a
// broken app → collector → DCE/DCR → workspace path proactively.
// ============================================================================

@description('Azure region')
param location string

@description('Base name prefix for resources')
param environmentName string

@description('Azure Monitor workspace resource ID for Prometheus metrics')
param monitorWorkspaceId string

@description('Optional Azure Monitor action group resource ID. Empty creates an alert rule without notification actions.')
param actionGroupId string = ''

var noBlogflowMetricsExpression = 'absent_over_time({__name__=~"blogflow_.*"}[30m])'

resource metricsIngestionAbsenceRuleGroup 'Microsoft.AlertsManagement/prometheusRuleGroups@2023-03-01' = {
  name: '${environmentName}-metrics-ingestion'
  location: location
  properties: {
    clusterName: ''
    description: 'Detects when no blogflow_* metrics are ingested into the Azure Monitor workspace.'
    enabled: true
    interval: 'PT1M'
    scopes: [
      monitorWorkspaceId
    ]
    rules: [
      {
        alert: 'BlogFlowMetricsIngestionAbsent'
        enabled: true
        expression: noBlogflowMetricsExpression
        for: '10m'
        severity: 2
        labels: {
          service: 'blogflow'
          signal: 'metrics'
        }
        annotations: {
          summary: 'No blogflow_* metrics have been ingested for at least 30 minutes.'
          description: 'The BlogFlow OTel metrics pipeline may be broken. Check Container App traffic, scale-to-zero settings, otel-collector logs, managed identity auth, and DCE/DCR configuration.'
        }
        actions: actionGroupId != '' ? [
          {
            actionGroupId: actionGroupId
          }
        ] : []
        resolveConfiguration: {
          autoResolved: true
          timeToResolve: 'PT10M'
        }
      }
    ]
  }
}
