package agentplatform

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

func deletedSessionCleanupEnabled(cfg *config) bool {
	if cfg == nil {
		return false
	}
	return cfg.Storage.Cleanup.Enabled
}

func deletedSessionCleanupInterval(cfg *config) time.Duration {
	if cfg == nil || cfg.Storage.Cleanup.IntervalMinutes <= 0 {
		return 24 * time.Hour
	}
	return time.Duration(cfg.Storage.Cleanup.IntervalMinutes) * time.Minute
}

func deletedSessionRetention(cfg *config) time.Duration {
	if cfg == nil || cfg.Storage.Cleanup.DeletedRetentionHours <= 0 {
		return 7 * 24 * time.Hour
	}
	return time.Duration(cfg.Storage.Cleanup.DeletedRetentionHours) * time.Hour
}

func deletedSessionCleanupBatchSize(cfg *config) int {
	if cfg == nil || cfg.Storage.Cleanup.BatchSize <= 0 {
		return 100
	}
	return cfg.Storage.Cleanup.BatchSize
}

func (s *Service) startDeletedSessionCleanupWorker() {
	if s == nil || s.repo == nil || !deletedSessionCleanupEnabled(s.cfg) {
		return
	}
	go func() {
		s.runDeletedSessionCleanup(context.Background())
		ticker := time.NewTicker(deletedSessionCleanupInterval(s.cfg))
		defer ticker.Stop()
		for range ticker.C {
			s.runDeletedSessionCleanup(context.Background())
		}
	}()
}

func (s *Service) runDeletedSessionCleanup(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	batchSize := deletedSessionCleanupBatchSize(s.cfg)
	cutoff := time.Now().Add(-deletedSessionRetention(s.cfg))
	total := 0
	for {
		deleted, err := s.repo.PurgeDeletedSessions(ctx, cutoff, batchSize)
		if err != nil {
			g.Log().Warningf(ctx, "agent deleted-session cleanup failed, cutoff=%s err=%v", cutoff.Format(time.RFC3339), err)
			return
		}
		if deleted == 0 {
			if total > 0 {
				g.Log().Infof(ctx, "agent deleted-session cleanup finished, purged=%d cutoff=%s", total, cutoff.Format(time.RFC3339))
			}
			return
		}
		total += deleted
		if deleted < batchSize {
			g.Log().Infof(ctx, "agent deleted-session cleanup finished, purged=%d cutoff=%s", total, cutoff.Format(time.RFC3339))
			return
		}
	}
}
