package content

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kenhaines/blogflow/internal/config"
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
