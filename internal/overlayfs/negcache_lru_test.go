package overlayfs

import (
	"fmt"
	"sync"
	"testing"
	"testing/fstest"
)

// TestNegativeCachePopulates verifies negCache entries are created when a file
// is absent from layer 0 but found in a lower layer (layer index > 0).
func TestNegativeCachePopulates(t *testing.T) {
	t.Parallel()

	// Three-layer setup: layer 0 (theme) shadows nothing in the test paths,
	// layer 1 (defaults) has some files, layer 2 (content) has others.
	// A path absent from layer 0 but present in layer 1 triggers negCache
	// with firstCandidateLayer=1.
	theme := fstest.MapFS{
		"config.yaml":      {Data: []byte("base_theme")},
		"static/theme.css": {Data: []byte("theme-css")},
	}

	defaults := fstest.MapFS{
		"static/reset.css":    {Data: []byte("*{margin:0}")},
		"templates/base.html": {Data: []byte("<html>")},
	}

	content := fstest.MapFS{
		"posts/hello.md": {Data: []byte("hello")},
		"static/app.js":  {Data: []byte("1+1")},
	}

	ofs := NewOverlayFS(theme, defaults, content).WithLayerNames([]string{"theme", "defaults", "content"})

	// Read paths absent from layer 0 (theme) but present in layer 1 (defaults).
	// These should create negCache entries at firstCandidateLayer = 1.
	fallthroughFrom0to1 := []string{
		"static/reset.css",
		"templates/base.html",
	}

	for _, name := range fallthroughFrom0to1 {
		data, err := ofs.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile(%q) unexpectedly failed: %v", name, err)
		}
		if len(data) == 0 {
			t.Errorf("ReadFile(%q) returned empty data", name)
		}
	}

	// Also read paths absent from layer 0 and layer 1 but present in layer 2.
	// These should create negCache entries at firstCandidateLayer = 2.
	fallthroughFrom0to2 := []string{
		"posts/hello.md",
		"static/app.js",
	}

	for _, name := range fallthroughFrom0to2 {
		data, err := ofs.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile(%q) unexpectedly failed: %v", name, err)
		}
		if len(data) == 0 {
			t.Errorf("ReadFile(%q) returned empty data", name)
		}
	}

	// Verify negCache populated: we expect at least 2 entries for fallthrough paths,
	// possibly more if layer 0 had paths that don't exist anywhere.
	count := ofs.negCacheCount.Load()
	if count < 2 {
		t.Errorf("negCacheEntry count = %d, want >= 2 (paths absent from layer 0 must populate negCache)", count)
	}

	// Verify that a path existing in layer 0 does NOT create a negCache entry,
	// because it was found at layer index 0 (not > 0).
	_, err := ofs.ReadFile("config.yaml")
	if err != nil {
		t.Fatalf("ReadFile(config.yaml) unexpectedly failed: %v", err)
	}
	gotBefore := ofs.negCacheCount.Load()

	// Reading the same path again should not increase count (already cached).
	_, err = ofs.ReadFile("config.yaml")
	if err != nil {
		t.Fatalf("ReadFile(config.yaml) 2nd try unexpectedly failed: %v", err)
	}
	gotAfter := ofs.negCacheCount.Load()

	if gotBefore != gotAfter {
		t.Errorf("double-read of layer-0 path changed negCache count from %d to %d; expected no change", gotBefore, gotAfter)
	}
}

// TestNegativeCacheLRUEvictsLeastRecentlyUsed verifies a full negative cache
// evicts the oldest not-recently-used entry, not the newest insertion.
func TestNegativeCacheLRUEvictsLeastRecentlyUsed(t *testing.T) {
	t.Parallel()

	const testLimit = 3

	emptyLayer := fstest.MapFS{}
	lowerLayer := fstest.MapFS{}
	for i := 1; i <= 4; i++ {
		key := fmt.Sprintf("file%02d", i)
		lowerLayer[key] = &fstest.MapFile{Data: []byte(fmt.Sprintf("data-%d", i))}
	}

	ofs := NewOverlayFS(emptyLayer, lowerLayer).WithLayerNames([]string{"theme", "defaults"})
	ofs.maxNegCacheEntries = testLimit

	for i := 1; i <= testLimit; i++ {
		name := fmt.Sprintf("file%02d", i)
		data, err := ofs.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile(%q) unexpectedly failed: %v", name, err)
		}
		if len(data) == 0 {
			t.Errorf("ReadFile(%q) returned empty data", name)
		}
	}

	if count := ofs.negCacheCount.Load(); count != int64(testLimit) {
		t.Fatalf("negCacheEntry count after warmup = %d, want %d", count, testLimit)
	}

	// Refresh file01 through Stat, which performs lookup-only recency refresh.
	// A true LRU cache should now evict file02, not file01.
	if _, err := ofs.Stat("file01"); err != nil {
		t.Fatalf("Stat(file01) refresh unexpectedly failed: %v", err)
	}
	if _, err := ofs.ReadFile("file04"); err != nil {
		t.Fatalf("ReadFile(file04) insertion unexpectedly failed: %v", err)
	}

	if count := ofs.negCacheCount.Load(); count != int64(testLimit) {
		t.Fatalf("negCacheEntry count after eviction = %d, want %d", count, testLimit)
	}

	assertNegCacheContains(t, ofs, "file01", true)
	assertNegCacheContains(t, ofs, "file02", false)
	assertNegCacheContains(t, ofs, "file03", true)
	assertNegCacheContains(t, ofs, "file04", true)
}

func TestNegativeCacheStoreUpdatesExistingEntry(t *testing.T) {
	t.Parallel()

	ofs := NewOverlayFS(fstest.MapFS{}, fstest.MapFS{}, fstest.MapFS{})

	ofs.negCacheStore("shared/path.txt", negCacheEntry{firstCandidateLayer: 1})
	ofs.negCacheStore("shared/path.txt", negCacheEntry{firstCandidateLayer: 2})

	entry, ok := ofs.negCacheLookup("shared/path.txt")
	if !ok {
		t.Fatal("negCacheLookup(shared/path.txt) missed after store")
	}
	if entry.firstCandidateLayer != 2 {
		t.Fatalf("firstCandidateLayer = %d, want 2", entry.firstCandidateLayer)
	}
	if count := ofs.negCacheCount.Load(); count != 1 {
		t.Fatalf("negCacheEntry count = %d, want 1", count)
	}
	assertNegCacheListSync(t, ofs)
}

func TestNegativeCacheSelectiveInvalidationKeepsMapAndListInSync(t *testing.T) {
	t.Parallel()

	t.Run("InvalidateLayer", func(t *testing.T) {
		t.Parallel()

		ofs := NewOverlayFS(fstest.MapFS{}, fstest.MapFS{}, fstest.MapFS{})
		ofs.negCacheStore("layer1.txt", negCacheEntry{firstCandidateLayer: 1})
		ofs.negCacheStore("layer2.txt", negCacheEntry{firstCandidateLayer: 2})
		ofs.negCacheStore("layer3.txt", negCacheEntry{firstCandidateLayer: 3})

		ofs.InvalidateLayer(2)

		assertNegCacheContains(t, ofs, "layer1.txt", true)
		assertNegCacheContains(t, ofs, "layer2.txt", false)
		assertNegCacheContains(t, ofs, "layer3.txt", false)
		assertNegCacheListSync(t, ofs)
	})

	t.Run("ReplaceLayer", func(t *testing.T) {
		t.Parallel()

		ofs := NewOverlayFS(fstest.MapFS{}, fstest.MapFS{}, fstest.MapFS{})
		ofs.negCacheStore("layer1.txt", negCacheEntry{firstCandidateLayer: 1})
		ofs.negCacheStore("layer2.txt", negCacheEntry{firstCandidateLayer: 2})
		ofs.negCacheStore("layer3.txt", negCacheEntry{firstCandidateLayer: 3})

		if err := ofs.ReplaceLayer(2, fstest.MapFS{}); err != nil {
			t.Fatalf("ReplaceLayer(2) failed: %v", err)
		}

		assertNegCacheContains(t, ofs, "layer1.txt", true)
		assertNegCacheContains(t, ofs, "layer2.txt", false)
		assertNegCacheContains(t, ofs, "layer3.txt", false)
		assertNegCacheListSync(t, ofs)
	})
}

func TestNegativeCacheInvalidateAllZeroValue(t *testing.T) {
	t.Parallel()

	var ofs OverlayFS
	ofs.InvalidateAll()
	assertNegCacheListSync(t, &ofs)
}

func TestNegativeCacheConcurrentReadFileInvalidateLayer(t *testing.T) {
	t.Parallel()

	ofs := NewOverlayFS(
		fstest.MapFS{},
		fstest.MapFS{"hot.txt": {Data: []byte("hot")}},
	)
	if _, err := ofs.ReadFile("hot.txt"); err != nil {
		t.Fatalf("warm ReadFile(hot.txt) failed: %v", err)
	}

	const iterations = 200
	var wg sync.WaitGroup
	errs := make(chan error, 8*iterations)

	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterations {
				data, err := ofs.ReadFile("hot.txt")
				if err != nil {
					errs <- err
					return
				}
				if string(data) != "hot" {
					errs <- fmt.Errorf("ReadFile(hot.txt) = %q, want hot", data)
					return
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for range iterations {
			ofs.InvalidateLayer(1)
		}
	}()

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	assertNegCacheListSync(t, ofs)
}

func assertNegCacheContains(t *testing.T, ofs *OverlayFS, name string, want bool) {
	t.Helper()

	ofs.negCacheMu.Lock()
	_, got := ofs.negCache[name]
	ofs.negCacheMu.Unlock()
	if got != want {
		t.Fatalf("negCache contains %q = %v, want %v", name, got, want)
	}
}

func assertNegCacheListSync(t *testing.T, ofs *OverlayFS) {
	t.Helper()

	ofs.negCacheMu.Lock()
	mapLen := len(ofs.negCache)
	listLen := 0
	if ofs.negCacheLRU != nil {
		listLen = ofs.negCacheLRU.Len()
	}
	ofs.negCacheMu.Unlock()

	if listLen != mapLen {
		t.Fatalf("negCacheLRU.Len() = %d, want len(negCache) %d", listLen, mapLen)
	}
}
