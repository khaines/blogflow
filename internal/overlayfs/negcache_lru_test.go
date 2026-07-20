package overlayfs

import (
	"fmt"
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"
	"time"
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
	count := ofs.negCacheLen()
	if count < 2 {
		t.Errorf("negCacheEntry count = %d, want >= 2 (paths absent from layer 0 must populate negCache)", count)
	}

	// Verify that a path existing in layer 0 does NOT create a negCache entry,
	// because it was found at layer index 0 (not > 0).
	_, err := ofs.ReadFile("config.yaml")
	if err != nil {
		t.Fatalf("ReadFile(config.yaml) unexpectedly failed: %v", err)
	}
	gotBefore := ofs.negCacheLen()

	// Reading the same path again should not increase count (already cached).
	_, err = ofs.ReadFile("config.yaml")
	if err != nil {
		t.Fatalf("ReadFile(config.yaml) 2nd try unexpectedly failed: %v", err)
	}
	gotAfter := ofs.negCacheLen()

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
	names := sameNegCacheShardNames(t, testLimit+1)
	for i, key := range names {
		lowerLayer[key] = &fstest.MapFile{Data: []byte(fmt.Sprintf("data-%d", i))}
	}

	ofs := NewOverlayFS(emptyLayer, lowerLayer).WithLayerNames([]string{"theme", "defaults"})
	ofs.maxNegCacheEntries = negCacheShardCount * testLimit

	for i := 1; i <= testLimit; i++ {
		name := names[i-1]
		data, err := ofs.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile(%q) unexpectedly failed: %v", name, err)
		}
		if len(data) == 0 {
			t.Errorf("ReadFile(%q) returned empty data", name)
		}
	}

	if count := ofs.negCacheLen(); count != int64(testLimit) {
		t.Fatalf("negCacheEntry count after warmup = %d, want %d", count, testLimit)
	}

	// Refresh the first file through Stat, which performs lookup-only recency refresh.
	// A true per-shard LRU cache should now evict the second file, not the first.
	if _, err := ofs.Stat(names[0]); err != nil {
		t.Fatalf("Stat(%s) refresh unexpectedly failed: %v", names[0], err)
	}
	if _, err := ofs.ReadFile(names[3]); err != nil {
		t.Fatalf("ReadFile(%s) insertion unexpectedly failed: %v", names[3], err)
	}

	if count := ofs.negCacheLen(); count != int64(testLimit) {
		t.Fatalf("negCacheEntry count after eviction = %d, want %d", count, testLimit)
	}

	assertNegCacheContains(t, ofs, names[0], true)
	assertNegCacheContains(t, ofs, names[1], false)
	assertNegCacheContains(t, ofs, names[2], true)
	assertNegCacheContains(t, ofs, names[3], true)
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
	if count := ofs.negCacheLen(); count != 1 {
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

func TestNegativeCacheInvalidationGenerationPreventsStaleStore(t *testing.T) {
	const name = "posts/race.md"
	upper := fstest.MapFS{}
	lower := &blockingOpenFS{
		base:      fstest.MapFS{name: {Data: []byte("lower")}},
		blockName: name,
		opened:    make(chan struct{}),
		release:   make(chan struct{}),
	}
	ofs := NewOverlayFS(upper, lower).WithLayerNames([]string{"theme", "defaults"})

	errCh := make(chan error, 1)
	go func() {
		data, err := ofs.ReadFile(name)
		if err != nil {
			errCh <- err
			return
		}
		if string(data) != "lower" {
			errCh <- fmt.Errorf("in-flight ReadFile(%q) = %q, want lower", name, data)
			return
		}
		errCh <- nil
	}()

	<-lower.opened
	upper[name] = &fstest.MapFile{Data: []byte("upper")}
	ofs.InvalidateLayer(0)
	close(lower.release)

	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	assertNegCacheContains(t, ofs, name, false)
	data, err := ofs.ReadFile(name)
	if err != nil {
		t.Fatalf("ReadFile(%q) after invalidation failed: %v", name, err)
	}
	if string(data) != "upper" {
		t.Fatalf("ReadFile(%q) after invalidation = %q, want upper", name, data)
	}
}

func TestNegativeCacheReplaceLayerGenerationSnapshotPreventsStaleStore(t *testing.T) {
	const name = "posts/replace-race.md"
	snapshot := make(chan struct{})
	proceed := make(chan struct{})
	var once sync.Once
	setNegCacheAfterLayerSnapshotHook(t, func() {
		once.Do(func() {
			close(snapshot)
			<-proceed
		})
	})

	ofs := NewOverlayFS(
		fstest.MapFS{},
		fstest.MapFS{name: {Data: []byte("lower")}},
	).WithLayerNames([]string{"theme", "defaults"})

	errCh := make(chan error, 1)
	go func() {
		data, err := ofs.ReadFile(name)
		if err != nil {
			errCh <- err
			return
		}
		if string(data) != "lower" {
			errCh <- fmt.Errorf("in-flight ReadFile(%q) = %q, want lower", name, data)
			return
		}
		errCh <- nil
	}()

	<-snapshot
	if err := ofs.ReplaceLayer(0, fstest.MapFS{name: {Data: []byte("upper")}}); err != nil {
		t.Fatalf("ReplaceLayer(0) failed: %v", err)
	}
	close(proceed)

	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	assertNegCacheContains(t, ofs, name, false)
	data, err := ofs.ReadFile(name)
	if err != nil {
		t.Fatalf("ReadFile(%q) after ReplaceLayer failed: %v", name, err)
	}
	if string(data) != "upper" {
		t.Fatalf("ReadFile(%q) after ReplaceLayer = %q, want upper", name, data)
	}
}

func TestNegativeCacheInvalidateLayerLocksAllShardsBeforePruning(t *testing.T) {
	const name = "posts/partial-invalidation.md"
	upper := fstest.MapFS{}
	lower := fstest.MapFS{name: {Data: []byte("lower")}}
	ofs := NewOverlayFS(upper, lower).WithLayerNames([]string{"theme", "defaults"})

	if data, err := ofs.ReadFile(name); err != nil {
		t.Fatalf("warm ReadFile(%q) failed: %v", name, err)
	} else if string(data) != "lower" {
		t.Fatalf("warm ReadFile(%q) = %q, want lower", name, data)
	}
	assertNegCacheContains(t, ofs, name, true)

	upper[name] = &fstest.MapFile{Data: []byte("upper")}
	invalidating := make(chan struct{})
	releaseInvalidation := make(chan struct{})
	var once sync.Once
	setNegCacheInvalidationLockedHook(t, func() {
		once.Do(func() {
			close(invalidating)
			<-releaseInvalidation
		})
	})

	invalidationDone := make(chan struct{})
	go func() {
		ofs.InvalidateLayer(0)
		close(invalidationDone)
	}()

	<-invalidating
	readDone := make(chan readResult, 1)
	go func() {
		data, err := ofs.ReadFile(name)
		readDone <- readResult{data: string(data), err: err}
	}()

	select {
	case result := <-readDone:
		t.Fatalf("ReadFile completed while invalidation held all shard locks: data=%q err=%v", result.data, result.err)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseInvalidation)
	<-invalidationDone
	result := <-readDone
	if result.err != nil {
		t.Fatalf("ReadFile(%q) during invalidation failed: %v", name, result.err)
	}
	if result.data != "upper" {
		t.Fatalf("ReadFile(%q) during invalidation = %q, want upper", name, result.data)
	}
	assertNegCacheContains(t, ofs, name, false)
}

func assertNegCacheContains(t *testing.T, ofs *OverlayFS, name string, want bool) {
	t.Helper()

	shard := ofs.negCacheShard(name)
	shard.mu.Lock()
	_, got := shard.m[name]
	shard.mu.Unlock()
	if got != want {
		t.Fatalf("negCache contains %q = %v, want %v", name, got, want)
	}
}

func assertNegCacheListSync(t *testing.T, ofs *OverlayFS) {
	t.Helper()

	for i := range ofs.negCacheShards {
		shard := &ofs.negCacheShards[i]
		shard.mu.Lock()
		mapLen := 0
		if shard.m != nil {
			mapLen = len(shard.m)
		}
		listLen := 0
		if shard.lru != nil {
			listLen = shard.lru.Len()
		}
		shard.mu.Unlock()

		if listLen != mapLen {
			t.Fatalf("negCache shard %d lru.Len() = %d, want len(map) %d", i, listLen, mapLen)
		}
	}
}

func sameNegCacheShardNames(t *testing.T, count int) []string {
	t.Helper()

	byShard := make(map[uint32][]string)
	for i := 0; i < 10_000; i++ {
		name := fmt.Sprintf("file%05d", i)
		shard := negCacheShardIndex(name)
		byShard[shard] = append(byShard[shard], name)
		if len(byShard[shard]) == count {
			return byShard[shard]
		}
	}
	t.Fatalf("could not find %d names in one negative-cache shard", count)
	return nil
}

type blockingOpenFS struct {
	base      fs.FS
	blockName string
	opened    chan struct{}
	release   chan struct{}
	once      sync.Once
}

func (b *blockingOpenFS) Open(name string) (fs.File, error) {
	if name == b.blockName {
		b.once.Do(func() { close(b.opened) })
		<-b.release
	}
	return b.base.Open(name)
}

type readResult struct {
	data string
	err  error
}

func setNegCacheAfterLayerSnapshotHook(t *testing.T, hook func()) {
	t.Helper()

	testHook := negCacheTestHook(hook)
	negCacheAfterLayerSnapshotHook.Store(&testHook)
	t.Cleanup(func() {
		negCacheAfterLayerSnapshotHook.Store(nil)
	})
}

func setNegCacheInvalidationLockedHook(t *testing.T, hook func()) {
	t.Helper()

	testHook := negCacheTestHook(hook)
	negCacheInvalidationLockedHook.Store(&testHook)
	t.Cleanup(func() {
		negCacheInvalidationLockedHook.Store(nil)
	})
}
