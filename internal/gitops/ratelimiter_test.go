package gitops

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestRateLimiter_BasicAllow(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter(3)
	rl.now = func() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) }

	for i := range 3 {
		if !rl.allow("10.0.0.1") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	if rl.allow("10.0.0.1") {
		t.Fatal("4th request should be denied")
	}
}

func TestRateLimiter_TTLEviction(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rl := newRateLimiter(2)
	rl.now = func() time.Time { return now }

	// Fill the limiter for two IPs.
	rl.allow("10.0.0.1")
	rl.allow("10.0.0.2")

	// Advance time past the window — entries should be lazily evicted.
	now = now.Add(rl.window + time.Second)

	if got := rl.lru.Len(); got != 2 {
		t.Fatalf("before eviction: entries = %d, want 2", got)
	}

	// Next request triggers lazy eviction of expired entries.
	if !rl.allow("10.0.0.3") {
		t.Fatal("new IP after expiry should be allowed")
	}

	// Old entries should have been evicted.
	if _, ok := rl.entries["10.0.0.1"]; ok {
		t.Fatal("10.0.0.1 should have been evicted (TTL expired)")
	}
	if _, ok := rl.entries["10.0.0.2"]; ok {
		t.Fatal("10.0.0.2 should have been evicted (TTL expired)")
	}
	if rl.lru.Len() != 1 {
		t.Fatalf("after eviction: entries = %d, want 1", rl.lru.Len())
	}
}

func TestRateLimiter_TTLRestoresQuota(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rl := newRateLimiter(2)
	rl.now = func() time.Time { return now }

	// Exhaust quota.
	rl.allow("10.0.0.1")
	rl.allow("10.0.0.1")
	if rl.allow("10.0.0.1") {
		t.Fatal("should be rate limited")
	}

	// Advance past the window.
	now = now.Add(rl.window + time.Second)

	// Quota should be restored.
	if !rl.allow("10.0.0.1") {
		t.Fatal("after TTL expiry, IP should be allowed again")
	}
}

func TestRateLimiter_LRUEvictionNotNuclear(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rl := newRateLimiter(5)
	rl.maxSize = 4 // small capacity for testing
	rl.now = func() time.Time { return now }

	// Fill to capacity with 4 IPs.
	for i := range 4 {
		ip := fmt.Sprintf("10.0.0.%d", i+1)
		if !rl.allow(ip) {
			t.Fatalf("IP %s should be allowed", ip)
		}
	}
	if len(rl.entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(rl.entries))
	}

	// Add a 5th IP — should evict the LRU entry (10.0.0.1, first inserted).
	if !rl.allow("10.0.0.5") {
		t.Fatal("new IP should be allowed after LRU eviction")
	}

	// Exactly one entry should have been evicted (not nuclear reset).
	if len(rl.entries) != 4 {
		t.Fatalf("expected 4 entries after LRU eviction, got %d", len(rl.entries))
	}

	// The first IP (least recently used) should have been evicted.
	if _, ok := rl.entries["10.0.0.1"]; ok {
		t.Fatal("10.0.0.1 should have been evicted as LRU")
	}

	// More recent IPs should still be present.
	for i := 2; i <= 5; i++ {
		ip := fmt.Sprintf("10.0.0.%d", i)
		if _, ok := rl.entries[ip]; !ok {
			t.Fatalf("%s should still be present", ip)
		}
	}
}

func TestRateLimiter_LRUOrderUpdatedOnAccess(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rl := newRateLimiter(10)
	rl.maxSize = 3
	rl.now = func() time.Time { return now }

	// Insert A, B, C. LRU order (front→back): C, B, A.
	rl.allow("A")
	rl.allow("B")
	rl.allow("C")

	// Access A to move it to front. LRU order: A, C, B.
	rl.allow("A")

	// Add D — should evict B (now the LRU).
	rl.allow("D")

	if _, ok := rl.entries["B"]; ok {
		t.Fatal("B should have been evicted as LRU")
	}
	if _, ok := rl.entries["A"]; !ok {
		t.Fatal("A should still be present (was recently accessed)")
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter(100)
	var wg sync.WaitGroup

	for i := range 50 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ip := fmt.Sprintf("10.0.0.%d", id)
			for range 10 {
				rl.allow(ip)
			}
		}(i)
	}

	wg.Wait()
	// No race condition — test passes if -race doesn't fire.
}

func TestRateLimiter_NilWhenDisabled(t *testing.T) {
	t.Parallel()

	if rl := newRateLimiter(0); rl != nil {
		t.Fatal("expected nil for limit <= 0")
	}
	if rl := newRateLimiter(-1); rl != nil {
		t.Fatal("expected nil for limit <= 0")
	}
}
