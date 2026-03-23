package content

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/kenhaines/blogflow/internal/config"
)

// CacheEntry holds a rendered HTML fragment and its metadata.
type CacheEntry struct {
	HTML     []byte
	ETag     string
	Modified time.Time
	Expiry   time.Time
}

// expired reports whether the entry has passed its TTL.
func (e *CacheEntry) expired(now time.Time) bool {
	return now.After(e.Expiry)
}

// Cache is a thread-safe, TTL-bounded rendered-HTML cache.
// When the number of entries reaches MaxEntries, a random existing
// entry is evicted to make room (simplest LRU-style approximation).
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
	cfg     config.CacheConfig
	now     func() time.Time // injectable clock for testing
}

// NewCache creates a Cache governed by cfg.
// Defensive defaults are applied when MaxEntries or TTL are zero/negative.
func NewCache(cfg config.CacheConfig) *Cache {
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 1000
	}
	if cfg.TTL <= 0 {
		cfg.TTL = time.Hour
	}

	return &Cache{
		entries: make(map[string]*CacheEntry),
		cfg:     cfg,
		now:     time.Now,
	}
}

// Get returns the cached HTML for key if it exists and has not expired.
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok || entry.expired(c.now()) {
		return nil, false
	}

	return append([]byte(nil), entry.HTML...), true
}

// GetEntry returns a snapshot of the CacheEntry for key if it exists and has
// not expired. The returned pointer is a shallow copy with a cloned HTML slice,
// so callers may mutate it without affecting the cache.
//
// NOTE (relaxed consistency): the read lock is released before the snapshot is
// built, so a concurrent Set on the same key may interleave. This is acceptable
// for an HTTP cache where a slightly stale or refreshed response is harmless.
func (c *Cache) GetEntry(key string) (*CacheEntry, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok || entry.expired(c.now()) {
		return nil, false
	}

	snapshot := *entry
	snapshot.HTML = append([]byte(nil), entry.HTML...)
	return &snapshot, true
}

// Set stores rendered HTML under key with the configured TTL.
// The caller's slice is copied on ingest, so subsequent mutations to html
// do not affect the cached data.
// If the cache is at capacity, an expired entry is preferred for eviction;
// otherwise one random entry is removed.
func (c *Cache) Set(key string, html []byte) {
	now := c.now()

	buf := make([]byte, len(html))
	copy(buf, html)

	h := sha256.Sum256(buf)
	etag := `"` + hex.EncodeToString(h[:16]) + `"`

	entry := &CacheEntry{
		HTML:     buf,
		ETag:     etag,
		Modified: now,
		Expiry:   now.Add(c.cfg.TTL),
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// If the key already exists, overwrite without eviction.
	if _, exists := c.entries[key]; exists {
		c.entries[key] = entry
		return
	}

	// Evict one entry when at capacity; prefer expired entries.
	if len(c.entries) >= c.cfg.MaxEntries {
		victim := ""
		for k, e := range c.entries {
			victim = k
			if e.expired(now) {
				break
			}
		}
		delete(c.entries, victim)
	}

	c.entries[key] = entry
}

// Invalidate removes a single entry by key.
func (c *Cache) Invalidate(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// InvalidateAll removes every entry from the cache.
func (c *Cache) InvalidateAll() {
	c.mu.Lock()
	c.entries = make(map[string]*CacheEntry)
	c.mu.Unlock()
}

// Len returns the number of entries currently stored (including expired).
func (c *Cache) Len() int {
	c.mu.RLock()
	n := len(c.entries)
	c.mu.RUnlock()

	return n
}
