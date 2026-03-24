package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/gitops"
)

// bootstrapContent clones (or pulls) the configured content repository into
// the content directory before the first content scan. This solves the
// cold-start problem for webhook deployments where no content exists until
// the first push event.
//
// Clone failures are logged but non-fatal — the server continues with
// embedded defaults for graceful degradation.
func bootstrapContent(cfg *config.Config, contentPath string, logger *slog.Logger) {
	repoURL := cfg.Sync.Repo
	branch := cfg.Sync.Branch
	if branch == "" {
		branch = "main"
	}

	dest := contentPath
	if dest == "" {
		dest = "content"
	}

	logger.Info("content bootstrap: cloning repository", "repo", repoURL, "branch", branch, "dest", dest)

	authCfg, err := gitops.LoadAuthFromEnv(logger)
	if err != nil {
		logger.Error("content bootstrap: failed to load git auth", "error", err)
		return
	}

	puller, err := gitops.NewPuller(authCfg, logger)
	if err != nil {
		logger.Error("content bootstrap: failed to create git puller", "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	changed, err := puller.CloneOrPull(ctx, repoURL, branch, dest)
	if err != nil {
		logger.Error("content bootstrap: clone/pull failed — continuing with defaults",
			"repo", repoURL, "error", err)
		return
	}

	if changed {
		logger.Info("content bootstrap: repository cloned successfully", "repo", repoURL, "branch", branch)
	} else {
		logger.Info("content bootstrap: repository already up to date", "repo", repoURL, "branch", branch)
	}
}
