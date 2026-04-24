package handlers

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Content type constants for the blogflow_content_views_total metric.
const (
	ContentTypePost      = "post"
	ContentTypePage      = "page"
	ContentTypeTag       = "tag"
	ContentTypeList      = "list"
	ContentTypePostsList = "posts_list"
	ContentTypeHome      = "home"
)

var contentViewsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "blogflow_content_views_total",
		Help: "Total content views by type and slug.",
	},
	[]string{"type", "slug"},
)

func init() {
	prometheus.MustRegister(contentViewsTotal)
}

// RecordContentView increments the content views counter and annotates the
// active OTel span with content metadata. Call after a successful content
// lookup, before template rendering. The counter records that content was
// found and matched to the request — not that the response was delivered.
//
// contentType is one of the ContentType* constants.
// slug is the content identifier (empty for list and home views).
// title is the human-readable content title (empty for tags and lists).
// tags is the content item's tag list; pass nil for content types where
// tagging is not meaningful (tag views, list views).
//
// Cardinality note: each unique (type, slug) pair creates one Prometheus time
// series. For typical blogs (< 1000 posts) this is well within Prometheus
// comfort. For larger sites, consider scrape-time aggregation or rely on
// OTel span attributes for per-slug drill-down.
func RecordContentView(r *http.Request, contentType, slug, title string, tags []string) {
	contentViewsTotal.WithLabelValues(contentType, slug).Inc()

	span := trace.SpanFromContext(r.Context())
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("content.type", contentType),
			attribute.String("content.slug", slug),
			attribute.String("content.title", title),
		)
		if len(tags) > 0 {
			span.SetAttributes(attribute.StringSlice("content.tags", tags))
		}
	}
}
