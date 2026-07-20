// ============================================================================
// Container App — BlogFlow + self-managed OpenTelemetry Collector sidecar
// ============================================================================
// Runs BlogFlow with an OpenTelemetry Collector sidecar. BlogFlow exports OTLP
// to localhost; the collector exports:
//   - Traces → Application Insights via the azuremonitor exporter
//   - Metrics → DCE/DCR → Azure Monitor workspace via otlphttp + azure_auth
//
// Identity: a user-assigned managed identity is attached to the Container App
// and granted Monitoring Metrics Publisher on the DCR before the app revision is
// created. The collector uses that identity to fetch Entra tokens for
// https://monitor.azure.com/.default.
// ============================================================================

@description('Azure region')
param location string

@description('Container app name')
param appName string

@description('Container Apps Environment resource ID')
param environmentId string

@description('Full container image reference (e.g. ghcr.io/khaines/blogflow:main)')
param containerImage string

@description('GHCR username for pulling container images')
param ghcrUsername string

@description('GHCR password/PAT with read:packages scope')
@secure()
param ghcrPassword string

@description('Application Insights connection string for trace export')
@secure()
param appInsightsConnectionString string

@description('Data Collection Rule resource name for OTLP metrics ingestion')
param dataCollectionRuleName string

@description('Data Collection Rule immutable ID used in OTLP metrics ingestion URLs')
param dataCollectionRuleImmutableId string

@description('DCE metrics ingestion endpoint')
param metricsIngestionEndpoint string

@description('DCR stream name for Azure Monitor OTLP metrics ingestion')
param otelMetricsStreamName string

@description('OpenTelemetry Collector Contrib image repository and tag. The digest is supplied separately so tag-only overrides are impossible.')
param otelCollectorImageRepository string = 'otel/opentelemetry-collector-contrib:0.148.0'

@description('OpenTelemetry Collector Contrib image SHA-256 digest, without the sha256: prefix.')
@minLength(64)
@maxLength(64)
param otelCollectorImageDigest string = '8164eab2e6bca9c9b0837a8d2f118a6618489008a839db7f9d6510e66be3923c'

@description('Minimum replica count')
param scaleMinReplicas int = 0

@description('Maximum replica count')
param scaleMaxReplicas int = 2

@description('Custom domain hostname (e.g. www.blogflow.io). Empty = no custom domain.')
param customDomainName string = ''

@description('Managed certificate ID for custom domain TLS. Required when customDomainName is set.')
param customDomainCertificateId string = ''

var monitoringMetricsPublisherRoleDefinitionId = '3913510d-42f4-4e42-8a64-420c390055eb'
var otelCollectorImage = '${otelCollectorImageRepository}@sha256:${otelCollectorImageDigest}'
var otlpMetricsEndpoint = '${metricsIngestionEndpoint}/datacollectionRules/${dataCollectionRuleImmutableId}/streams/${otelMetricsStreamName}/otlp/v1/metrics'
var otelCollectorConfig = join([
  'receivers:'
  '  otlp:'
  '    protocols:'
  '      grpc:'
  '        endpoint: localhost:4317'
  '      http:'
  '        endpoint: localhost:4318'
  ''
  'processors:'
  '  memory_limiter:'
  '    check_interval: 1s'
  '    limit_mib: 384'
  '    spike_limit_mib: 96'
  '  batch:'
  '    timeout: 10s'
  '    send_batch_size: 256'
  ''
  'extensions:'
  '  health_check:'
  '    # ACA probes originate from the platform, not inside the container process.'
  '    # Keep this on 0.0.0.0; loopback-only binds are not reachable by ACA probes.'
  '    endpoint: 0.0.0.0:13133'
  '  azure_auth:'
  '    managed_identity:'
  '      client_id: ${containerAppIdentity.properties.clientId}'
  '    scopes:'
  '      - https://monitor.azure.com/.default'
  ''
  'exporters:'
  '  azuremonitor/traces: {}'
  '  otlphttp/azuremonitor_metrics:'
  '    metrics_endpoint: "${otlpMetricsEndpoint}"'
  '    auth:'
  '      authenticator: azure_auth'
  '    retry_on_failure:'
  '      enabled: true'
  '      initial_interval: 5s'
  '      max_interval: 30s'
  '      max_elapsed_time: 10m'
  ''
  'service:'
  '  extensions: [health_check, azure_auth]'
  '  pipelines:'
  '    traces:'
  '      receivers: [otlp]'
  '      processors: [memory_limiter, batch]'
  '      exporters: [azuremonitor/traces]'
  '    metrics:'
  '      receivers: [otlp]'
  '      processors: [memory_limiter, batch]'
  '      exporters: [otlphttp/azuremonitor_metrics]'
], '\n')

// ---------------------------------------------------------------------------
// Managed identity and DCR-scoped metrics publishing grant
// ---------------------------------------------------------------------------
resource containerAppIdentity 'Microsoft.ManagedIdentity/userAssignedIdentities@2023-01-31' = {
  name: '${appName}-otel-mi'
  location: location
}

resource metricsDataCollectionRule 'Microsoft.Insights/dataCollectionRules@2024-03-11' existing = {
  name: dataCollectionRuleName
}

resource metricsPublisherRoleAssignment 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name: guid(metricsDataCollectionRule.id, containerAppIdentity.id, monitoringMetricsPublisherRoleDefinitionId)
  scope: metricsDataCollectionRule
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', monitoringMetricsPublisherRoleDefinitionId)
    principalId: containerAppIdentity.properties.principalId
    principalType: 'ServicePrincipal'
  }
}

// ---------------------------------------------------------------------------
// Container App
// ---------------------------------------------------------------------------
resource containerApp 'Microsoft.App/containerApps@2024-03-01' = {
  name: appName
  location: location
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${containerAppIdentity.id}': {}
    }
  }
  properties: {
    managedEnvironmentId: environmentId
    configuration: {
      activeRevisionsMode: 'Multiple'

      // --- Ingress: external HTTPS on port 8080 ---
      ingress: {
        external: true
        targetPort: 8080
        transport: 'auto'
        allowInsecure: false
        customDomains: customDomainName != '' ? [
          {
            name: customDomainName
            certificateId: customDomainCertificateId
            bindingType: customDomainCertificateId != '' ? 'SniEnabled' : 'Disabled'
          }
        ] : []
      }

      // --- Container registry credentials (GHCR) ---
      registries: [
        {
          server: 'ghcr.io'
          username: ghcrUsername
          passwordSecretRef: 'ghcr-password'
        }
      ]

      // --- Secrets ---
      secrets: [
        {
          name: 'ghcr-password'
          value: ghcrPassword
        }
        {
          name: 'appinsights-cs'
          value: appInsightsConnectionString
        }
      ]
    }
    template: {
      volumes: [
        {
          name: 'content'
          storageType: 'EmptyDir'
        }
      ]
      containers: [
        // --- BlogFlow application container ---
        {
          name: 'blogflow'
          image: containerImage
          args: [
            '--content'
            '/data/content'
          ]
          volumeMounts: [
            {
              volumeName: 'content'
              mountPath: '/data/content'
            }
          ]
          resources: {
            cpu: json('0.25')
            memory: '0.5Gi'
          }
          env: [
            {
              name: 'OTEL_SERVICE_NAME'
              value: 'blogflow'
            }
            {
              name: 'OTEL_TRACES_EXPORTER'
              value: 'otlp'
            }
            {
              name: 'OTEL_METRICS_EXPORTER'
              value: 'otlp'
            }
            {
              name: 'OTEL_EXPORTER_OTLP_ENDPOINT'
              value: 'http://localhost:4318'
            }
            {
              name: 'OTEL_EXPORTER_OTLP_PROTOCOL'
              value: 'http/protobuf'
            }
            {
              name: 'OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE'
              value: 'cumulative'
            }
            {
              name: 'BLOGFLOW_SYNC_STRATEGY'
              value: 'poll'
            }
            {
              name: 'BLOGFLOW_SYNC_REPO'
              value: 'https://github.com/khaines/blogflow.git'
            }
            {
              name: 'BLOGFLOW_SYNC_BRANCH'
              value: 'main'
            }
            {
              name: 'BLOGFLOW_SYNC_POLL_INTERVAL'
              value: '5m'
            }
            {
              name: 'BLOGFLOW_SITE_HOMEPAGE'
              value: 'static:docs/index.html'
            }
            {
              name: 'BLOGFLOW_CONTENT_POSTS_DIR'
              value: 'docs'
            }
            {
              name: 'BLOGFLOW_SITE_TITLE'
              value: 'blogflow.io'
            }
            {
              name: 'BLOGFLOW_SITE_DESCRIPTION'
              value: 'Documentation for BlogFlow — the anti-WordPress blog engine'
            }
          ]
          probes: [
            {
              type: 'Liveness'
              httpGet: {
                path: '/healthz'
                port: 8080
              }
              initialDelaySeconds: 10
              periodSeconds: 15
            }
            {
              type: 'Readiness'
              httpGet: {
                path: '/readyz?strict=true'
                port: 8080
              }
              initialDelaySeconds: 5
              periodSeconds: 5
              failureThreshold: 30
            }
            {
              type: 'Startup'
              httpGet: {
                path: '/readyz?strict=true'
                port: 8080
              }
              periodSeconds: 2
              failureThreshold: 60
            }
          ]
        }
        // --- OpenTelemetry Collector sidecar ---
        {
          name: 'otel-collector'
          image: otelCollectorImage
          args: [
            '--config=env:OTELCOL_CONFIG'
          ]
          resources: {
            cpu: json('0.25')
            memory: '0.5Gi'
          }
          env: [
            {
              name: 'OTELCOL_CONFIG'
              value: otelCollectorConfig
            }
            {
              name: 'APPLICATIONINSIGHTS_CONNECTION_STRING'
              secretRef: 'appinsights-cs'
            }
          ]
          probes: [
            {
              type: 'Startup'
              httpGet: {
                path: '/'
                port: 13133
              }
              periodSeconds: 2
              failureThreshold: 60
            }
            {
              type: 'Readiness'
              httpGet: {
                path: '/'
                port: 13133
              }
              initialDelaySeconds: 5
              periodSeconds: 5
              failureThreshold: 12
            }
            {
              type: 'Liveness'
              httpGet: {
                path: '/'
                port: 13133
              }
              initialDelaySeconds: 10
              periodSeconds: 15
            }
          ]
        }
      ]

      // --- Scaling ---
      scale: {
        minReplicas: scaleMinReplicas
        maxReplicas: scaleMaxReplicas
      }
    }
  }
  dependsOn: [
    metricsPublisherRoleAssignment
  ]
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

@description('Container App FQDN')
output fqdn string = containerApp.properties.configuration.ingress.fqdn

@description('Container App resource name')
output name string = containerApp.name

@description('User-assigned managed identity principal ID used by the OTel Collector')
output principalId string = containerAppIdentity.properties.principalId

@description('User-assigned managed identity client ID used by the OTel Collector')
output clientId string = containerAppIdentity.properties.clientId
