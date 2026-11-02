// Package scheduler runs the product/customer sync for every active branch
// on a fixed interval, in-process - no external cron needed.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"ecommerce/backend/internal/db"
	syncpkg "ecommerce/backend/internal/sync"
)

type Scheduler struct {
	engine      *syncpkg.Engine
	queries     *db.Queries
	interval    time.Duration
	concurrency int
	logger      *slog.Logger
}

func New(engine *syncpkg.Engine, queries *db.Queries, interval time.Duration, concurrency int, logger *slog.Logger) *Scheduler {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Scheduler{
		engine:      engine,
		queries:     queries,
		interval:    interval,
		concurrency: concurrency,
		logger:      logger,
	}
}

// Run blocks, syncing immediately and then on every tick, until ctx is
// cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	s.RunOnce(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.RunOnce(ctx)
		}
	}
}

// RunOnce syncs every active branch, bounded by s.concurrency, and blocks
// until all of them finish or ctx is cancelled.
func (s *Scheduler) RunOnce(ctx context.Context) {
	branches, err := s.queries.ListActiveBranches(ctx)
	if err != nil {
		s.logger.Error("list active branches failed", "error", err)
		return
	}
	if len(branches) == 0 {
		return
	}

	sem := make(chan struct{}, s.concurrency)
	var wg sync.WaitGroup

	for _, branch := range branches {
		select {
		case <-ctx.Done():
			return
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(b db.Branch) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := s.engine.SyncBranch(ctx, b); err != nil {
				s.logger.Error("branch sync finished with errors", "branch_id", b.ID, "branch", b.Name, "error", err)
			}
		}(branch)
	}

	wg.Wait()
}
