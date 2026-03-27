# BlogFlow Grafana Dashboard

Pre-built Grafana dashboard for monitoring BlogFlow with Prometheus.

## Panels

| Row | Panels |
|-----|--------|
| **RED Overview** | Request Rate · Error Rate · p50/p95/p99 Latency · In-Flight Requests |
| **HTTP Detail** | Requests by Path (top 10) · Error Rate by Path · Latency by Path (p95) |
| **Overlay Filesystem** | Layer Hit Rate · Cache Hit Ratio · Resolve Duration · Neg-Cache Size · Path Rejections |
| **Go Runtime** | Goroutines · Memory · GC Pause Duration · Open File Descriptors |

## Prerequisites

- Grafana ≥ 9.0
- A Prometheus datasource scraping BlogFlow's `/metrics` endpoint

## Import via Grafana UI

1. Open Grafana → **Dashboards** → **New** → **Import**.
2. Click **Upload JSON file** and select `blogflow-dashboard.json` (or paste its contents).
3. Select your Prometheus datasource in the **DS_PROMETHEUS** dropdown.
4. Click **Import**.

## Import via provisioning

Copy the dashboard JSON to your Grafana provisioning directory and add a
provider in `provisioning/dashboards/blogflow.yaml`:

```yaml
apiVersion: 1
providers:
  - name: blogflow
    orgId: 1
    folder: BlogFlow
    type: file
    options:
      path: /var/lib/grafana/dashboards/blogflow
```

Then place `blogflow-dashboard.json` in `/var/lib/grafana/dashboards/blogflow/`.

## Template variables

The dashboard ships with two template variables that appear as dropdowns at the
top of the page:

| Variable | Description |
|----------|-------------|
| `job` | Prometheus job label (defaults to all) |
| `instance` | Target instance (defaults to all) |

## Customisation

The datasource is referenced as `${DS_PROMETHEUS}` so Grafana will prompt you
to bind it on import. Edit the JSON directly if you need to hard-code a UID
instead.

## Trace correlation

When [OpenTelemetry tracing](../../docs/deployment-guide.md#observability) is
enabled, BlogFlow injects `trace_id` and `span_id` into every log line. If
your logging pipeline forwards these fields to a trace backend (Grafana Tempo,
Jaeger, etc.), you can jump directly from a Grafana log panel to the
corresponding distributed trace.

In Grafana, configure a **Tempo** or **Jaeger** datasource alongside
Prometheus, then add a **Logs** panel filtered by `trace_id` to correlate
dashboard metrics with individual traces.
