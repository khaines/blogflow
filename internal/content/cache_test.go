package content

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/khaines/blogflow/internal/config"
)

func testCacheConfig() config.CacheConfig {
	return config.CacheConfig{
		Enabled:    true,
		TTL:        1 * time.Hour,
		MaxEntries: 100,
	}
}

func TestCache_SetGet(t *testing.T) {
	t.Parallel()

	c := NewCache(testCacheConfig())

	html := []byte("<h1>Hello</h1>")
	c.Set("post/hello", html)

	got, ok := c.Get("post/hello")
	if !ok {
		t.Fatal("expected cache hit")
	}

	if string(got) != string(html) {
		t.Fatalf("got %q, want %q", got, html)
	}

	// Verify ETag is populated via GetEntry.
	entry, ok := c.GetEntry("post/hello")
	if !ok {
		t.Fatal("expected entry")
	}

	if entry.ETag == "" {
		t.Fatal("expected non-empty ETag")
	}

	if entry.Modified.IsZero() {
		t.Fatal("expected non-zero Modified time")
	}
}

func TestCache_SetGet_Miss(t *testing.T) {
	t.Parallel()

	c := NewCache(testCacheConfig())

	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss for nonexistent key")
	}
}

func TestCache_SetGet_Overwrite(t *testing.T) {
	t.Parallel()

	c := NewCache(testCacheConfig())

	c.Set("k", []byte("v1"))
	c.Set("k", []byte("v2"))

	got, ok := c.Get("k")
	if !ok {
		t.Fatal("expected cache hit")
	}

	if string(got) != "v2" {
		t.Fatalf("got %q, want %q", got, "v2")
	}

	if c.Len() != 1 {
		t.Fatalf("expected 1 entry after overwrite, got %d", c.Len())
	}
}

func TestCache_Expiry(t *testing.T) {
	t.Parallel()

	cfg := testCacheConfig()
	cfg.TTL = 500 * time.Millisecond

	c := NewCache(cfg)

	now := time.Now()
	c.now = func() time.Time { return now }

	c.Set("post/expire", []byte("<p>bye</p>"))

	// Still valid.
	if _, ok := c.Get("post/expire"); !ok {
		t.Fatal("expected cache hit before expiry")
	}

	// Advance clock past TTL.
	c.now = func() time.Time { return now.Add(1 * time.Second) }

	if _, ok := c.Get("post/expire"); ok {
		t.Fatal("expected cache miss after expiry")
	}
}

func TestCache_Invalidate(t *testing.T) {
	t.Parallel()

	c := NewCache(testCacheConfig())

	c.Set("a", []byte("A"))
	c.Set("b", []byte("B"))

	c.Invalidate("a")

	if _, ok := c.Get("a"); ok {
		t.Fatal("expected miss after invalidate")
	}

	if _, ok := c.Get("b"); !ok {
		t.Fatal("expected hit for non-invalidated key")
	}

	if c.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", c.Len())
	}
}

func TestCache_InvalidateAll(t *testing.T) {
	t.Parallel()

	c := NewCache(testCacheConfig())

	for i := range 10 {
		c.Set(fmt.Sprintf("k%d", i), []byte("v"))
	}

	if c.Len() != 10 {
		t.Fatalf("expected 10 entries, got %d", c.Len())
	}

	c.InvalidateAll()

	if c.Len() != 0 {
		t.Fatalf("expected 0 entries after flush, got %d", c.Len())
	}
}

func TestCache_MaxEntries(t *testing.T) {
	t.Parallel()

	cfg := testCacheConfig()
	cfg.MaxEntries = 5

	c := NewCache(cfg)

	// Fill to capacity + overflow.
	for i := range 10 {
		c.Set(fmt.Sprintf("key-%d", i), []byte(fmt.Sprintf("val-%d", i)))
	}

	if c.Len() > cfg.MaxEntries {
		t.Fatalf("cache size %d exceeds max %d", c.Len(), cfg.MaxEntries)
	}

	// The most-recently-set key must still be present.
	if _, ok := c.Get("key-9"); !ok {
		t.Fatal("expected the last inserted key to be present")
	}
}

func TestCache_MaxEntries_OverwriteNoEvict(t *testing.T) {
	t.Parallel()

	cfg := testCacheConfig()
	cfg.MaxEntries = 3

	c := NewCache(cfg)

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3"))

	// Overwriting an existing key should not evict anything.
	c.Set("b", []byte("updated"))

	if c.Len() != 3 {
		t.Fatalf("expected 3 entries, got %d", c.Len())
	}

	for _, key := range []string{"a", "b", "c"} {
		if _, ok := c.Get(key); !ok {
			t.Fatalf("expected key %q to be present after overwrite", key)
		}
	}
}

func TestCache_DefaultConfig(t *testing.T) {
	t.Parallel()

	// F3: zero MaxEntries and TTL get defensive defaults.
	c := NewCache(config.CacheConfig{Enabled: true})

	if c.cfg.MaxEntries != 1000 {
		t.Fatalf("expected default MaxEntries=1000, got %d", c.cfg.MaxEntries)
	}

	if c.cfg.TTL != time.Hour {
		t.Fatalf("expected default TTL=1h, got %v", c.cfg.TTL)
	}
}

func TestCache_SetCopiesInput(t *testing.T) {
	t.Parallel()

	// F1: mutating the caller's slice after Set must not affect the cache.
	c := NewCache(testCacheConfig())

	html := []byte("<p>original</p>")
	c.Set("k", html)

	// Mutate the caller's slice.
	copy(html, "XXXXXXXXXXXXXXX")

	got, ok := c.Get("k")
	if !ok {
		t.Fatal("expected cache hit")
	}

	if string(got) != "<p>original</p>" {
		t.Fatalf("cache data corrupted: got %q", got)
	}
}

func TestCache_GetEntryReturnsSnapshot(t *testing.T) {
	t.Parallel()

	// F2: mutating the returned entry must not affect the cache.
	c := NewCache(testCacheConfig())

	c.Set("k", []byte("<p>hello</p>"))

	entry, ok := c.GetEntry("k")
	if !ok {
		t.Fatal("expected cache hit")
	}

	// Mutate the snapshot.
	entry.HTML[0] = 'X'
	entry.ETag = "tampered"

	// Re-fetch and verify the cache is untouched.
	entry2, ok := c.GetEntry("k")
	if !ok {
		t.Fatal("expected cache hit")
	}

	if string(entry2.HTML) != "<p>hello</p>" {
		t.Fatalf("cache HTML corrupted: got %q", entry2.HTML)
	}

	if entry2.ETag == "tampered" {
		t.Fatal("cache ETag corrupted by caller mutation")
	}
}

func TestCache_GetReturnsDefensiveCopy(t *testing.T) {
	t.Parallel()

	c := NewCache(testCacheConfig())
	c.Set("k", []byte("<p>safe</p>"))

	got, _ := c.Get("k")
	got[0] = 'X'

	got2, _ := c.Get("k")
	if string(got2) != "<p>safe</p>" {
		t.Fatalf("cache HTML corrupted via Get return: got %q", got2)
	}
}

func TestCache_EvictExpiredFirst(t *testing.T) {
	t.Parallel()

	// F4: when at capacity, an expired entry should be evicted rather than a live one.
	cfg := testCacheConfig()
	cfg.MaxEntries = 3
	cfg.TTL = 500 * time.Millisecond

	c := NewCache(cfg)

	t0 := time.Now()
	c.now = func() time.Time { return t0 }

	c.Set("fresh-a", []byte("a"))
	c.Set("fresh-b", []byte("b"))
	c.Set("expire-c", []byte("c"))

	// Advance clock so expire-c's TTL has passed, but fresh-a/b will be re-set.
	t1 := t0.Add(1 * time.Second)
	c.now = func() time.Time { return t1 }

	// Re-set fresh entries so they get a new expiry.
	c.Set("fresh-a", []byte("a2"))
	c.Set("fresh-b", []byte("b2"))

	// Cache is full (3). Insert a new key — the expired entry should be evicted.
	c.Set("fresh-d", []byte("d"))

	if c.Len() != 3 {
		t.Fatalf("expected 3 entries, got %d", c.Len())
	}

	// The expired key should have been the eviction victim.
	if _, ok := c.Get("expire-c"); ok {
		t.Fatal("expected expired key 'expire-c' to have been evicted")
	}

	// Fresh keys must still be present.
	for _, key := range []string{"fresh-a", "fresh-b", "fresh-d"} {
		if _, ok := c.Get(key); !ok {
			t.Fatalf("expected fresh key %q to still be present", key)
		}
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	c := NewCache(testCacheConfig())

	const goroutines = 50
	const ops = 200

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for g := range goroutines {
		go func(id int) {
			defer wg.Done()

			for i := range ops {
				key := fmt.Sprintf("g%d-k%d", id, i)
				c.Set(key, []byte(fmt.Sprintf("v%d", i)))
				c.Get(key)

				if i%10 == 0 {
					c.Invalidate(key)
				}

				if i%50 == 0 {
					c.InvalidateAll()
				}

				_ = c.Len()
			}
		}(g)
	}

	wg.Wait()
}
