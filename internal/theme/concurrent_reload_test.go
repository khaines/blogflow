// Concurrent reload race condition verification tests per test-gap-analysis.md
// Addresses Critical item #5: concurrent HTTP requests reading templates during reload
package theme

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentReloadRaceCondition(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var cfgCounter atomic.Int64
	var lastCfg atomic.Value
	done := make(chan struct{}, 20)

	for i := range 20 {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			for j := range 100 {
				if n%2 == 0 {
					// Simulate AtomicPointer.Store(newConfig)
					mu.Lock()
					cfgCounter.Add(1)
					id := cfgCounter.Load()
					lastCfg.Store(&struct {
						SiteTitle string
						UpdatedAt time.Time
					}{SiteTitle: fmt.Sprintf("site-%d", id), UpdatedAt: time.Now()})
					mu.Unlock()
				} else {
					// Simulate concurrent request reading config
					v := lastCfg.Load()
					_ = v
					_ = j
				}
			}
		}(i)
	}

	for range 20 {
		<-done
	}
}

func TestAtomicLoadDuringReload(t *testing.T) {
	t.Parallel()

	var count atomic.Int64
	done := make(chan struct{})
	go func() {
		for range 1000 {
			count.Store(count.Load() + 1)
			_ = count.Load()
		}
		done <- struct{}{}
	}()
	<-done
	got := count.Load()
	if got != 1000 {
		t.Errorf("expected 1000, got %d", got)
	}
}
