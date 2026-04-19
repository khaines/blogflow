// ============================================================================
// Container App — BlogFlow (sidecar-free)
// ============================================================================
// Runs the BlogFlow container. Telemetry is handled by the ACA managed
// OpenTelemetry agent (configured on the environment):
//   - Traces → Application Insights
//   - Metrics → not exported via OTel (app still exposes /metrics on :8080)
//
// The ACA managed OTel agent automatically injects OTEL_EXPORTER_OTLP_ENDPOINT
// and other standard OTel env vars at runtime. BlogFlow's OTel SDK discovers
// the agent automatically.
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

@description('Minimum replica count')
param scaleMinReplicas int = 0

@description('Maximum replica count')
param scaleMaxReplicas int = 2

@description('Custom domain hostname (e.g. www.blogflow.io). Empty = no custom domain.')
param customDomainName string = ''

@description('Managed certificate ID for custom domain TLS. Required when customDomainName is set.')
param customDomainCertificateId string = ''

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
      // NOTE: 'appinsights-cs' is retained as a placeholder during the transition
      // from sidecar to managed OTel agent. Active revisions still reference it.
      // Remove after all old revisions using it have been deactivated.
      secrets: [
        {
          name: 'ghcr-password'
          value: ghcrPassword
        }
        {
          name: 'appinsights-cs'
          value: 'deprecated-see-env-level-config'
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
            // The ACA managed OTel agent auto-injects OTEL_EXPORTER_OTLP_ENDPOINT.
            // BlogFlow hardcodes OTEL_SERVICE_NAME='blogflow' in code (cmd/blogflow/main.go).
            // We only set the trace exporter type so BlogFlow's OTel SDK initializes tracing.
            // OTEL_METRICS_EXPORTER is intentionally UNSET (not "none") because BlogFlow's
            // init code checks `os.Getenv != ""` — any non-empty value enables the metrics
            // bridge. Leaving it unset skips metrics initialization entirely. See Phase 2
            // in SETUP.md for future metrics export.
            {
              name: 'OTEL_TRACES_EXPORTER'
              value: 'otlp'
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
        // OTel Collector sidecar REMOVED — the ACA managed OTel agent
        // handles trace/metrics routing at the environment level.
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
