package handlers

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
// lookup and before template rendering.
//
// contentType is one of: "post", "page", "tag", "list", "home".
// slug is the content identifier (empty for list/home views).
// title is the human-readable content title (empty for tags and lists).
// tags is the post's tag list (nil for non-post content).
func RecordContentView(r *http.Request, contentType, slug, title string, tags []string) {
	contentViewsTotal.WithLabelValues(contentType, slug).Inc()

	span := trace.SpanFromContext(r.Context())
	span.SetAttributes(
		attribute.String("content.type", contentType),
		attribute.String("content.slug", slug),
		attribute.String("content.title", title),
	)
	if len(tags) > 0 {
		span.SetAttributes(attribute.StringSlice("content.tags", tags))
	}
}
