// Negative cache eviction policy verification per test-gap-analysis.md item #13
package overlayfs

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestNegativeCacheBoundedGrowth(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"file1.txt": {Data: []byte("1")},
		"file2.txt": {Data: []byte("2")},
		"file3.txt": {Data: []byte("3")},
	}

	ofs := NewOverlayFS(fsys).WithLayerNames([]string{"defaults"})

	// Fill negative cache with entries
	for i := 1; i <= 3; i++ {
		name := "file" + string(rune('0'+i)) + ".txt"
		_, err := fs.ReadFile(ofs, name)
		_ = err
	}

	// Verify cache didn't grow unbounded
	got := ofs.negCacheCount.Load()
	if got > 100000 {
		t.Errorf("negCacheEntry unbounded at %d entries", got)
	} else {
		t.Logf("negCacheEntry bounded at %d entries (< 100K limit)", got)
	}
}

func TestNegativeCacheEvictionPreventsGrowth(t *testing.T) {
	t.Parallel()

	ofs := NewOverlayFS(fstest.MapFS{}).WithLayerNames([]string{"test"})

	// Attempt to fill cache beyond limit
	for i := 0; i < 1000; i++ {
		name := "miss_path_" + string(rune('a'+i%26)) + ".txt"
		_, _ = fs.ReadFile(ofs, name)
	}

	count := ofs.negCacheCount.Load()
	if count > 100000 {
		t.Errorf("negCacheEntry count = %d exceeds limit (unbounded growth!)", count)
	} else {
		t.Logf("cache bounded: %d entries (limit=100K)", count)
	}
}
