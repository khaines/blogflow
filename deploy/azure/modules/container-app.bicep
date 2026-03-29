// ============================================================================
// Container App — BlogFlow with OTel Collector sidecar
// ============================================================================
// Runs the BlogFlow container with embedded content (docs site baked into the
// image). An OTel Collector sidecar receives traces and metrics from BlogFlow
// via OTLP HTTP, scrapes Prometheus metrics, and exports to Azure Monitor.
//
// Identity: System-assigned managed identity for Azure integration.
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

@description('App Insights connection string for OTel Collector export')
@secure()
param appInsightsConnectionString string

@description('Minimum replica count')
param scaleMinReplicas int = 0

@description('Maximum replica count')
param scaleMaxReplicas int = 2

// ---------------------------------------------------------------------------
// Container App
// ---------------------------------------------------------------------------
resource containerApp 'Microsoft.App/containerApps@2024-03-01' = {
  name: appName
  location: location
  identity: {
    type: 'SystemAssigned'
  }
  properties: {
    managedEnvironmentId: environmentId
    configuration: {
      activeRevisionsMode: 'Single'

      // --- Ingress: external HTTPS on port 8080 ---
      ingress: {
        external: true
        targetPort: 8080
        transport: 'auto'
        allowInsecure: false
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
            '/data/content/docs'
            '--config'
            '/data/content/docs/config'
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
              name: 'OTEL_TRACES_EXPORTER'
              value: 'otlp'
            }
            {
              name: 'OTEL_METRICS_EXPORTER'
              value: 'otlp'
            }
            {
              name: 'OTEL_SERVICE_NAME'
              value: 'blogflow'
            }
            {
              name: 'OTEL_EXPORTER_OTLP_ENDPOINT'
              value: 'http://localhost:4318'
            }
            {
              name: 'BLOGFLOW_METRICS_PORT'
              value: '9090'
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
              name: 'BLOGFLOW_SYNC_SPARSE_DIRS'
              value: 'docs'
            }
            {
              name: 'BLOGFLOW_SITE_HOMEPAGE'
              value: 'static:index.html'
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
              initialDelaySeconds: 5
              periodSeconds: 15
            }
            {
              type: 'Readiness'
              httpGet: {
                path: '/readyz'
                port: 8080
              }
              initialDelaySeconds: 3
              periodSeconds: 10
            }
          ]
        }
        // --- OTel Collector sidecar ---
        {
          name: 'otel-collector'
          image: 'ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-contrib:0.121.0'
          args: [
            '--config'
            'env:OTEL_COLLECTOR_CONFIG'
          ]
          resources: {
            cpu: json('0.25')
            memory: '0.5Gi'
          }
          env: [
            {
              name: 'APPLICATIONINSIGHTS_CONNECTION_STRING'
              secretRef: 'appinsights-cs'
            }
            {
              name: 'OTEL_COLLECTOR_CONFIG'
              value: loadTextContent('../otel/collector-config.yaml')
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
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

@description('Container App FQDN')
output fqdn string = containerApp.properties.configuration.ingress.fqdn

@description('Container App resource name')
output name string = containerApp.name

@description('System-assigned managed identity principal ID')
output principalId string = containerApp.identity.principalId
