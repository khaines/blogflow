package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	blogflow "github.com/khaines/blogflow"
	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/content"
	"github.com/khaines/blogflow/internal/search"
	"github.com/khaines/blogflow/internal/server/handlers"
	"github.com/khaines/blogflow/internal/theme"
)

// pageDeps builds Deps with the real embedded theme and a configurable
// router-build search-enabled value, for testing global header affordances.
func pageDeps(t *testing.T, searchEnabled bool, posts ...*content.Post) *handlers.Deps {
	t.Helper()
	eng, err := theme.NewEngine(blogflow.Defaults)
	if err != nil {
		t.Fatalf("theme engine: %v", err)
	}
	cfg := config.Default()
	cfg.Search.Enabled = searchEnabled
	idx := searchIndex(posts...)
	deps := handlers.NewDeps(cfg, idx, eng)
	deps.SetSearchEnabled(searchEnabled)
	if searchEnabled {
		si := search.Build(context.Background(), idx, cfg.Search, 1)
		deps.SetSnapshot(1, idx, si)
	} else {
		deps.SetSnapshot(1, idx, nil)
	}
	return deps
}

func TestGlobalSearchAffordance_ShownWhenEnabled(t *testing.T) {
	deps := pageDeps(t, true, sampleCorpus()...)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handlers.ListHandler(deps)(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `class="site-search"`) {
		t.Error("enabled search should render a header search form on normal pages")
	}
	if !strings.Contains(body, `action="/search"`) {
		t.Error("header search form should post to /search")
	}
}

func TestGlobalSearchAffordance_HiddenWhenDisabled(t *testing.T) {
	deps := pageDeps(t, false, sampleCorpus()...)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handlers.ListHandler(deps)(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, `class="site-search"`) {
		t.Error("disabled search must not render a header search form")
	}
	if strings.Contains(body, `action="/search"`) {
		t.Error("disabled search must not emit any link/form to /search")
	}
}

// TestSnapshot_ReloadSwapsSearchableTerms verifies a published reload makes new
// terms searchable and old terms disappear, using one atomic snapshot swap.
func TestSnapshot_ReloadSwapsSearchableTerms(t *testing.T) {
	deps := searchDeps(t, true, mkSearchPost("a", "Alpha topic", nil, 1, "<p>alpha content</p>"))

	rec := doSearch(t, deps, "/search?q=alpha")
	if !strings.Contains(rec.Body.String(), "Alpha topic") {
		t.Fatal("initial term should be searchable")
	}

	// Publish a new generation with different content.
	newIdx := searchIndex(mkSearchPost("b", "Beta topic", nil, 2, "<p>beta content</p>"))
	cfg := config.Default().Search
	cfg.Enabled = true
	gen := deps.NextGeneration()
	newSI := search.Build(context.Background(), newIdx, cfg, gen)
	deps.SetSnapshot(gen, newIdx, newSI)

	if got := doSearch(t, deps, "/search?q=alpha"); !strings.Contains(got.Body.String(), "No results") {
		t.Error("old term should no longer be searchable after reload")
	}
	if got := doSearch(t, deps, "/search?q=beta"); !strings.Contains(got.Body.String(), "Beta topic") {
		t.Error("new term should be searchable after reload")
	}
}

// TestSnapshot_ConcurrentReloadAndQuery runs concurrent publishes and queries;
// it must be free of data races (run with -race) and mixed generations.
func TestSnapshot_ConcurrentReloadAndQuery(t *testing.T) {
	deps := searchDeps(t, true, sampleCorpus()...)
	cfg := config.Default().Search
	cfg.Enabled = true

	var reloaders sync.WaitGroup
	var queriers sync.WaitGroup
	stop := make(chan struct{})

	// Reloaders: repeatedly build and publish new generations until stopped.
	for r := 0; r < 3; r++ {
		reloaders.Add(1)
		go func() {
			defer reloaders.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				idx := searchIndex(
					mkSearchPost("x", "Concurrent go", []string{"go"}, 1, "<p>go overlay content</p>"),
					mkSearchPost("y", "Overlay", []string{"go"}, 2, "<p>overlay filesystem</p>"),
				)
				gen := deps.NextGeneration()
				si := search.Build(context.Background(), idx, cfg, gen)
				deps.SetSnapshot(gen, idx, si)
			}
		}()
	}

	// Queriers: a fixed number of queries each, then finish.
	for q := 0; q < 4; q++ {
		queriers.Add(1)
		go func() {
			defer queriers.Done()
			for i := 0; i < 200; i++ {
				rec := doSearch(t, deps, "/search?q=go")
				if rec.Code != http.StatusOK {
					t.Errorf("query status = %d", rec.Code)
					return
				}
			}
		}()
	}

	queriers.Wait() // let all queries complete under concurrent reloads
	close(stop)     // then stop the reloaders
	reloaders.Wait()
}
