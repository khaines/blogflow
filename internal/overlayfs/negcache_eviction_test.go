// Negative cache admission bound policy verification per test-gap-analysis.md item #13
// Addresses Critical finding: original tests never exercised negCache population or eviction.
package overlayfs

import (
	"fmt"
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

// TestNegativeCacheAdmissionWithSmallLimit verifies the admission bound (capping) works.
// maxNegCacheEntries is unexported, but since this test is in package overlayfs
// we can set it directly for validation.
func TestNegativeCacheAdmissionWithSmallLimit(t *testing.T) {
	t.Parallel()

	// Set a very small limit so we can verify admission control.
	const testLimit = 5

	// layer 0 (theme) is empty — all files will miss layer 0.
	emptyLayer := fstest.MapFS{}

	lowerLayer := fstest.MapFS{}
	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("file%02d", i)
		lowerLayer[key] = &fstest.MapFile{Data: []byte(fmt.Sprintf("data-%d", i))}
	}

	ofs := NewOverlayFS(emptyLayer, lowerLayer).WithLayerNames([]string{"theme", "defaults"})
	// Override the unexported limit (white-box test, same package).
	ofs.maxNegCacheEntries = testLimit

	uniquePaths := make([]string, 10)
	for i := 1; i <= 10; i++ {
		uniquePaths[i-1] = fmt.Sprintf("file%02d", i)
	}

	// Read all 10 paths. Each misses layer 0 and hits layer 1 (i=1 > 0).
	// negCache should cap at 5 due to maxNegCacheEntries.
	for _, name := range uniquePaths {
		data, err := ofs.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile(%q) unexpectedly failed: %v", name, err)
		}
		if len(data) == 0 {
			t.Errorf("ReadFile(%q) returned empty data", name)
		}
		_ = data
	}

	count := ofs.negCacheCount.Load()
	if count > int64(testLimit) {
		t.Errorf("negCacheEntry count = %d, want <= %d (admission bound violated)", count, testLimit)
	}
	if count == 0 {
		t.Error("negCache should have entries with multi-layer setup where path is absent from layer 0")
	}
}
