package gitops

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// PollStrategy periodically pulls from a git remote and reloads content
// when changes are detected. This ensures all replicas eventually sync,
// even when webhook delivery only reaches one pod.
type PollStrategy struct {
	puller   PullExecutor
	reloader ContentReloader
	logger   *slog.Logger
	interval time.Duration
	repoURL  string
	branch   string
	destPath string
	cancel   context.CancelFunc
	stopOnce sync.Once
	done     chan struct{}
	mu       sync.Mutex
	started  bool
}

// PullExecutor abstracts the git pull operation for testing.
type PullExecutor interface {
	CloneOrPull(ctx context.Context, repoURL, branch, destPath string) (changed bool, err error)
}

// NewPollStrategy creates a poll-based sync strategy.
// The puller and repo details can be wired after construction via SetPuller.
func NewPollStrategy(interval time.Duration, reloader ContentReloader, logger *slog.Logger) (*PollStrategy, error) {
	if reloader == nil {
		return nil, fmt.Errorf("gitops: poll strategy requires a content reloader")
	}
	if interval < 30*time.Second {
		return nil, fmt.Errorf("gitops: poll interval must be >= 30s, got %s", interval)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &PollStrategy{
		reloader: reloader,
		logger:   logger,
		interval: interval,
		done:     make(chan struct{}),
	}, nil
}

// SetPuller configures the git puller and repo details. Must be called before Start.
func (p *PollStrategy) SetPuller(puller PullExecutor, repoURL, branch, destPath string) {
	p.puller = puller
	p.repoURL = repoURL
	p.branch = branch
	p.destPath = destPath
}

// Start begins periodic polling in a background goroutine.
func (p *PollStrategy) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return fmt.Errorf("gitops: poll strategy already started")
	}
	if p.puller == nil {
		return fmt.Errorf("gitops: poll strategy has no puller configured — call SetPuller before Start")
	}
	ctx, p.cancel = context.WithCancel(ctx) //nolint:gosec // G118: cancel is stored and called in Stop()
	p.started = true
	p.logger.Info("poll strategy started", "interval", p.interval)
	go p.loop(ctx)
	return nil
}

// Stop gracefully shuts down polling. Idempotent via sync.Once.
// Blocks until the background goroutine exits.
func (p *PollStrategy) Stop(_ context.Context) error {
	p.stopOnce.Do(func() {
		if p.cancel != nil {
			p.cancel()
		}
		p.logger.Info("poll strategy stopped")
	})
	p.mu.Lock()
	started := p.started
	p.mu.Unlock()
	if started {
		<-p.done
	}
	return nil
}

// Name returns the strategy name.
func (p *PollStrategy) Name() string { return "poll" }

// NewPollStrategyForTest creates a PollStrategy with no minimum interval
// guard. Intended for unit tests that need sub-second tick intervals.
func NewPollStrategyForTest(interval time.Duration, reloader ContentReloader, logger *slog.Logger) (*PollStrategy, error) {
	if reloader == nil {
		return nil, fmt.Errorf("gitops: poll strategy requires a content reloader")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &PollStrategy{
		reloader: reloader,
		logger:   logger,
		interval: interval,
		done:     make(chan struct{}),
	}, nil
}

func (p *PollStrategy) loop(ctx context.Context) {
	defer close(p.done)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.tick(ctx)
		}
	}
}

func (p *PollStrategy) tick(ctx context.Context) {
	p.logger.Debug("poll tick: pulling content")

	changed, err := p.puller.CloneOrPull(ctx, p.repoURL, p.branch, p.destPath)
	if err != nil {
		p.logger.Error("poll pull failed", "error", err)
		return
	}

	if !changed {
		p.logger.Debug("poll tick: no changes")
		return
	}

	p.logger.Info("poll tick: content changed, reloading")
	if err := p.reloader(); err != nil {
		p.logger.Error("poll content reload failed", "error", err)
	}
}
