package memory

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/alle-bartoli/mnemoir/internal/config"
	goredis "github.com/redis/go-redis/v9"
)

// MaintenanceResult holds the outcome of a maintenance run.
type MaintenanceResult struct {
	Skipped        bool `json:"skipped"`
	ForgottenCount int  `json:"forgotten_count"`
	PrunedSessions int  `json:"pruned_sessions"`
	OrphanCleaned  bool `json:"orphan_cleaned"`
}

// RunMaintenance performs automatic cleanup for a project.
// Skips if maintenance ran recently (controlled by min_run_interval).
// Non-destructive to actively used data: only removes stale low-importance
// memories, old sessions beyond the cap, and orphaned references.
func (s *Store) RunMaintenance(ctx context.Context, project string, maintCfg config.MaintenanceConfig, memCfg config.MemoryConfig) (*MaintenanceResult, error) {
	if !maintCfg.Enabled {
		return &MaintenanceResult{Skipped: true}, nil
	}

	// Throttle: skip if maintenance ran recently for this project
	runKey := "maint:last_run:" + project
	minInterval, _ := maintCfg.ParsedMinRunInterval()

	exists, err := s.rdb.Exists(ctx, runKey).Result()
	if err != nil {
		return nil, fmt.Errorf("check maintenance throttle: %w", err)
	}
	if exists > 0 {
		return &MaintenanceResult{Skipped: true}, nil
	}

	result := &MaintenanceResult{}

	// 1. Auto-forget stale low-importance memories
	forgotten, err := s.autoForget(ctx, project, maintCfg, memCfg)
	if err != nil {
		slog.Error("auto-forget failed", "project", project, "error", err)
		// Non-fatal: continue with other maintenance tasks
	}
	result.ForgottenCount = forgotten

	// 2. Prune old sessions
	pruned, err := s.pruneSessions(ctx, project, maintCfg.MaxSessionsPerProject)
	if err != nil {
		slog.Error("session pruning failed", "project", project, "error", err)
	}
	result.PrunedSessions = pruned

	// 3. Clean orphaned references
	cleaned, err := s.cleanupOrphans(ctx, project)
	if err != nil {
		slog.Error("orphan cleanup failed", "project", project, "error", err)
	}
	result.OrphanCleaned = cleaned

	// Mark maintenance as done with TTL and stats
	now := time.Now()
	pipe := s.rdb.Pipeline()
	pipe.HSet(ctx, runKey, map[string]any{
		"timestamp":       now.Unix(),
		"forgotten_count": result.ForgottenCount,
		"pruned_sessions": result.PrunedSessions,
		"orphan_cleaned":  result.OrphanCleaned,
	})
	pipe.Expire(ctx, runKey, minInterval)
	if _, err := pipe.Exec(ctx); err != nil {
		slog.Error("failed to record maintenance run", "project", project, "error", err)
	}

	return result, nil
}

// autoForgetScript computes effective importance server-side and deletes stale memories.
// KEYS = candidate mem:{ULID} keys
// ARGV[1] = decay_factor, ARGV[2] = decay_interval_seconds, ARGV[3] = boost_factor,
// ARGV[4] = boost_cap, ARGV[5] = threshold, ARGV[6] = now_unix
var autoForgetScript = goredis.NewScript(`
local deleted = 0
for i, key in ipairs(KEYS) do
    local imp = tonumber(redis.call('HGET', key, 'importance') or '5')
    local last = tonumber(redis.call('HGET', key, 'last_accessed') or ARGV[6])
    local acc = tonumber(redis.call('HGET', key, 'access_count') or '0')
    local decay_interval = tonumber(ARGV[2])
    local intervals = 0
    if decay_interval > 0 then
        intervals = (tonumber(ARGV[6]) - last) / decay_interval
    end
    local decayed = imp * math.pow(tonumber(ARGV[1]), intervals)
    local boost = math.min(tonumber(ARGV[4]), acc * tonumber(ARGV[3]))
    local eff = math.max(1.0, math.min(10.0, decayed + boost))
    if eff <= tonumber(ARGV[5]) then
        redis.call('DEL', key)
        deleted = deleted + 1
    end
end
return deleted
`)

// @dev autoForget finds memories not accessed in forgetInactiveDays, then uses a Lua
// script to compute effective importance server-side and delete those below threshold.
func (s *Store) autoForget(ctx context.Context, project string, maintCfg config.MaintenanceConfig, memCfg config.MemoryConfig) (int, error) {
	cutoff := time.Now().Add(-time.Duration(maintCfg.ForgetInactiveDays) * 24 * time.Hour).Unix()
	query := fmt.Sprintf("@project:{%s} @last_accessed:[-inf %d]", escapeTag(project), cutoff)

	decayInterval, _ := memCfg.ParsedDecayInterval()
	now := time.Now().Unix()
	totalDeleted := 0

	const batchSize = 200 // Lua script batch size (keep KEYS count reasonable)
	const maxIterations = 50

	for iteration := 0; iteration < maxIterations; iteration++ {
		args := []any{"FT.SEARCH", "idx:memories", query, "NOCONTENT", "LIMIT", 0, batchSize}
		res, err := s.rdb.Do(ctx, args...).Result()
		if err != nil {
			return totalDeleted, fmt.Errorf("search for auto-forget: %w", err)
		}

		ids := extractIDsFromSearch(res)
		if len(ids) == 0 {
			break
		}

		// Run Lua script with candidate keys
		deleted, err := autoForgetScript.Run(ctx, s.rdb, ids,
			memCfg.DecayFactor,
			decayInterval.Seconds(),
			memCfg.AccessBoostFactor,
			memCfg.AccessBoostCap,
			maintCfg.ForgetThreshold,
			now,
		).Int()
		if err != nil {
			return totalDeleted, fmt.Errorf("auto-forget script: %w", err)
		}
		totalDeleted += deleted

		// If the script didn't delete anything, remaining candidates are above threshold
		if deleted == 0 {
			break
		}
	}

	return totalDeleted, nil
}

// @dev pruneSessions keeps only the N most recent sessions for a project.
// Deletes older session hashes and removes their sorted set entries.
func (s *Store) pruneSessions(ctx context.Context, project string, maxSessions int) (int, error) {
	setKey := "project_sessions:" + project

	count, err := s.rdb.ZCard(ctx, setKey).Result()
	if err != nil {
		return 0, fmt.Errorf("zcard %s: %w", setKey, err)
	}
	if int(count) <= maxSessions {
		return 0, nil
	}

	// Get IDs of oldest sessions to prune (everything except the N newest)
	pruneCount := int(count) - maxSessions
	oldIDs, err := s.rdb.ZRange(ctx, setKey, 0, int64(pruneCount-1)).Result()
	if err != nil {
		return 0, fmt.Errorf("zrange %s: %w", setKey, err)
	}
	if len(oldIDs) == 0 {
		return 0, nil
	}

	// Delete session hashes and remove sorted set entries
	pipe := s.rdb.Pipeline()
	for _, id := range oldIDs {
		pipe.Del(ctx, "session:"+id)
	}
	pipe.ZRemRangeByRank(ctx, setKey, 0, int64(pruneCount-1))
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("prune sessions: %w", err)
	}

	return len(oldIDs), nil
}

// @dev cleanupOrphans removes orphaned references for a project:
// - Stale entries in project_sessions sorted set (pointing to deleted session hashes)
// - Project from "projects" SET if it has 0 memories left
func (s *Store) cleanupOrphans(ctx context.Context, project string) (bool, error) {
	cleaned := false

	// 1. Clean stale sorted set entries
	setKey := "project_sessions:" + project
	members, err := s.rdb.ZRange(ctx, setKey, 0, -1).Result()
	if err != nil {
		return false, fmt.Errorf("zrange orphans: %w", err)
	}

	if len(members) > 0 {
		// Check which session hashes still exist
		pipe := s.rdb.Pipeline()
		existsCmds := make([]*goredis.IntCmd, len(members))
		for i, id := range members {
			existsCmds[i] = pipe.Exists(ctx, "session:"+id)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return false, fmt.Errorf("check session existence: %w", err)
		}

		var stale []any
		for i, cmd := range existsCmds {
			if cmd.Val() == 0 {
				stale = append(stale, members[i])
			}
		}
		if len(stale) > 0 {
			if err := s.rdb.ZRem(ctx, setKey, stale...).Err(); err != nil {
				return false, fmt.Errorf("zrem stale sessions: %w", err)
			}
			slog.Info("cleaned stale session references", "project", project, "count", len(stale))
			cleaned = true
		}
	}

	// 2. Remove project from SET if it has 0 memories
	memCount, err := s.CountByProject(ctx, project)
	if err != nil {
		return cleaned, fmt.Errorf("count for orphan check: %w", err)
	}
	if memCount == 0 {
		if err := s.rdb.SRem(ctx, "projects", project).Err(); err != nil {
			return cleaned, fmt.Errorf("srem orphan project: %w", err)
		}
		slog.Info("removed empty project", "project", project)
		cleaned = true
	}

	return cleaned, nil
}
