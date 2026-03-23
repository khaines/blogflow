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
func NewCache(cfg config.CacheConfig) *Cache {
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

	return entry.HTML, true
}

// GetEntry returns the full CacheEntry for key if it exists and has not expired.
func (c *Cache) GetEntry(key string) (*CacheEntry, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok || entry.expired(c.now()) {
		return nil, false
	}

	return entry, true
}

// Set stores rendered HTML under key with the configured TTL.
// If the cache is at capacity, one random entry is evicted first.
func (c *Cache) Set(key string, html []byte) {
	now := c.now()

	h := sha256.Sum256(html)
	etag := `"` + hex.EncodeToString(h[:16]) + `"`

	entry := &CacheEntry{
		HTML:     html,
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

	// Evict one random entry when at capacity.
	if c.cfg.MaxEntries > 0 && len(c.entries) >= c.cfg.MaxEntries {
		for k := range c.entries {
			delete(c.entries, k)
			break
		}
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
