package handlers

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/khaines/blogflow/internal/search"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// maxSearchPage bounds the parsed page number so that computing the offset
// (page-1)*limit cannot overflow int for any parseable input.
const maxSearchPage = 1_000_000

// SearchHandler returns the handler for GET /search. It is registered only when
// search.enabled was true at router build time; when search is disabled the
// route is not registered and requests fall through to normal 404 handling.
// Go's ServeMux serves HEAD via the GET handler and returns 405 with an
// Allow: GET header for other methods automatically.
//
// The handler owns validation of query rune length and page parameters; the
// search index owns validation of index availability and query term count.
// Response statuses follow the design's API surface:
//
//	200 landing form (empty/whitespace query), inline validation (below min),
//	    results, no-results, and page-overflow pages
//	400 query over max length or over max terms
//	503 route registered but this generation's search index is nil
//	500 template render failure
func SearchHandler(deps *Deps) http.HandlerFunc {
	tracer := otel.Tracer("github.com/khaines/blogflow/search")
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// Per design §7.3: create a search.Query span. The raw query string is
		// deliberately not attached as an attribute (§5.3). search.duration_ms is
		// set at the end so it is recorded on every return path.
		ctx, span := tracer.Start(r.Context(), "search.Query")
		defer func() {
			span.SetAttributes(attribute.Int64("search.duration_ms", time.Since(start).Milliseconds()))
			span.End()
		}()

		cfg := deps.LoadConfig()
		sc := cfg.Search

		rawQuery := r.URL.Query().Get("q")
		normalized := search.NormalizeQuery(rawQuery)

		data := &SearchData{
			Enabled: true,
			Query:   rawQuery,
			Page:    1,
		}
		span.SetAttributes(attribute.Bool("search.enabled", true))

		// Empty or whitespace-only query: render the landing form (200). This is
		// normal navigation rather than a search, so it is not counted in the
		// search query metric.
		if normalized == "" {
			span.SetAttributes(attribute.Bool("search.landing", true))
			_ = deps.renderSearch(w, r, data, http.StatusOK)
			return
		}

		queryLen := search.RuneLen(normalized)
		span.SetAttributes(attribute.Int("search.query_terms_count", search.CountQueryTerms(normalized)))

		// Over max length: reject before tokenization (400).
		if queryLen > sc.MaxQueryLength {
			data.Error = "Search query is too long. Please shorten it and try again."
			deps.finishSearch(w, r, span, data, http.StatusBadRequest, search.StatusInvalid, start)
			return
		}

		// Below min length: inline accessible validation message (200); do not
		// scan the index.
		if queryLen < sc.MinQueryLength {
			data.Error = "Please enter at least " + strconv.Itoa(sc.MinQueryLength) + " characters to search."
			deps.finishSearch(w, r, span, data, http.StatusOK, search.StatusInvalid, start)
			return
		}

		// Page parameter: invalid or below 1 clamps to page 1; huge values clamp
		// to maxSearchPage so the offset cannot overflow. Pages beyond the last
		// are not pre-clamped so pagination can expose the true last page.
		page := parseSearchPage(r)
		limit := sc.MaxResults
		if limit < 1 {
			limit = 20
		}
		span.SetAttributes(attribute.Int("search.page", page), attribute.Int("search.limit", limit))

		snap := deps.LoadSnapshot()
		var si *search.SearchIndex
		if snap != nil {
			si = snap.Search
		}

		// Route registered but this generation's search index is nil: 503.
		if si == nil {
			logSearchUnavailable(span, snapshotGeneration(snap))
			data.Error = "Search is temporarily unavailable. Please try again shortly."
			span.SetStatus(codes.Error, "search index unavailable")
			deps.finishSearch(w, r, span, data, http.StatusServiceUnavailable, search.StatusError, start)
			return
		}
		span.SetAttributes(attribute.Int("search.index_docs", si.DocCount()))

		resp, err := si.Search(ctx, snap.Content, rawQuery, search.SearchOptions{
			Limit:  limit,
			Offset: (page - 1) * limit,
		})
		if err != nil {
			switch {
			case errors.Is(err, search.ErrQueryTooComplex):
				data.Error = "Search query is too complex. Please use fewer distinct words."
				deps.finishSearch(w, r, span, data, http.StatusBadRequest, search.StatusInvalid, start)
			case errors.Is(err, search.ErrIndexUnavailable):
				logSearchUnavailable(span, snap.Generation)
				data.Error = "Search is temporarily unavailable. Please try again shortly."
				span.SetStatus(codes.Error, err.Error())
				deps.finishSearch(w, r, span, data, http.StatusServiceUnavailable, search.StatusError, start)
			default:
				slog.Error("search failed", "error", err, "trace_id", traceID(span))
				span.SetStatus(codes.Error, err.Error())
				search.RecordQuery(search.StatusError, time.Since(start))
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		populateSearchResults(data, resp)
		span.SetAttributes(
			attribute.Int("search.results_count", resp.Total),
			attribute.Bool("search.zero_results", resp.Total == 0),
		)
		// Record the result-count distribution only when the response actually
		// renders successfully, so it stays consistent with queries_total{ok}.
		if deps.finishSearch(w, r, span, data, http.StatusOK, search.StatusOK, start) {
			search.RecordResults(resp.Total)
		}
	}
}

// finishSearch renders the search page and records the query metric based on
// the actual outcome. If the render fails (500 already written), the request is
// recorded as an error regardless of the intended logical status, so a template
// failure is never counted as a successful search. It returns true when the
// render succeeded so callers can record success-only metrics consistently.
func (d *Deps) finishSearch(w http.ResponseWriter, r *http.Request, span trace.Span, data *SearchData, httpStatus int, logicalStatus string, start time.Time) bool {
	if err := d.renderSearch(w, r, data, httpStatus); err != nil {
		span.SetStatus(codes.Error, "search render failed")
		search.RecordQuery(search.StatusError, time.Since(start))
		return false
	}
	search.RecordQuery(logicalStatus, time.Since(start))
	return true
}

// renderSearch renders the search page with the given SearchData and status.
// It returns any render error after having written a 500 response, so callers
// can record accurate metrics.
func (d *Deps) renderSearch(w http.ResponseWriter, r *http.Request, data *SearchData, status int) error {
	cfg := d.LoadConfig()
	pd := &PageData{
		Site:   cfg.Site,
		Feed:   cfg.Feed,
		Title:  searchTitle(data.Query),
		Search: data,
	}
	var buf bytes.Buffer
	if err := d.Theme.Render(r.Context(), &buf, "templates/search.html", pd); err != nil {
		if r.Context().Err() != nil {
			slog.Debug("search render aborted: client disconnected", "error", err)
			return err
		}
		slog.Error("template render failed", "template", "templates/search.html", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
	return nil
}

// logSearchUnavailable emits the ERROR log required by design §7.1/§2.5 when
// the search route is registered but the index is unavailable (503).
func logSearchUnavailable(span trace.Span, generation uint64) {
	slog.Error("search index unavailable", "generation", generation, "trace_id", traceID(span))
}

// snapshotGeneration returns the snapshot's generation, or 0 when nil.
func snapshotGeneration(snap *SiteSnapshot) uint64 {
	if snap == nil {
		return 0
	}
	return snap.Generation
}

// traceID returns the span's trace ID as a string for log correlation.
func traceID(span trace.Span) string {
	sc := span.SpanContext()
	if !sc.HasTraceID() {
		return ""
	}
	return sc.TraceID().String()
}

// populateSearchResults copies a SearchResponse into template data and computes
// pagination URLs.
func populateSearchResults(data *SearchData, resp search.SearchResponse) {
	data.Executed = true
	data.Results = resp.Results
	data.Total = resp.Total
	data.Page = resp.Page
	data.TotalPages = resp.TotalPages
	data.Truncated = resp.Truncated
	data.HasPrev = resp.Page > 1
	data.HasNext = resp.Page < resp.TotalPages
	if data.HasPrev {
		// On a page beyond the last, target the true last page so the previous
		// link returns readers to real results instead of another empty page.
		prev := resp.Page - 1
		if prev > resp.TotalPages {
			prev = resp.TotalPages
		}
		data.PrevURL = searchPageURL(resp.Query, prev)
	}
	if data.HasNext {
		data.NextURL = searchPageURL(resp.Query, resp.Page+1)
	}
}

// searchTitle returns the page title for a search request.
func searchTitle(query string) string {
	if query == "" {
		return "Search"
	}
	return "Search results for \"" + query + "\""
}

// parseSearchPage reads the ?page= parameter, clamping invalid or below-1 values
// to 1 and very large values to maxSearchPage (to prevent offset overflow).
// Values beyond the last page are intentionally not clamped to the last page.
func parseSearchPage(r *http.Request) int {
	p := r.URL.Query().Get("page")
	if p == "" {
		return 1
	}
	v, err := strconv.Atoi(p)
	if err != nil || v < 1 {
		return 1
	}
	if v > maxSearchPage {
		return maxSearchPage
	}
	return v
}

// searchPageURL builds a canonical /search URL for a page, preserving the query.
func searchPageURL(query string, page int) string {
	u := "/search?q=" + url.QueryEscape(query)
	if page > 1 {
		u += "&page=" + strconv.Itoa(page)
	}
	return u
}
