package memory_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alle-bartoli/mnemoir/internal/config"
	"github.com/alle-bartoli/mnemoir/internal/embedding"
	"github.com/alle-bartoli/mnemoir/internal/memory"
	redisclient "github.com/alle-bartoli/mnemoir/internal/redis"
	goredis "github.com/redis/go-redis/v9"
)

const maintProject = "test-maint"

var defaultMaintCfg = config.MaintenanceConfig{
	Enabled:               true,
	ForgetThreshold:       2.0,
	ForgetInactiveDays:    30,
	MaxSessionsPerProject: 3,
	MinRunInterval:        "1s",
}

func newMaintTestStore(t *testing.T) (*memory.Store, *goredis.Client) {
	t.Helper()

	rdb := newRedisClient()
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	rc, err := redisclient.NewClient(config.RedisConfig{Addr: "localhost:6379", Password: redisPassword()})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := redisclient.EnsureIndex(ctx, rc, 384); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	rc.Close()

	emb, err := embedding.NewLocalEmbedder(config.EmbeddingLocalConfig{
		Model:    "sentence-transformers/all-MiniLM-L6-v2",
		ModelDir: "~/.mnemoir/models",
	}, 384)
	if err != nil {
		t.Fatalf("NewLocalEmbedder: %v", err)
	}

	store := memory.NewStore(rdb, emb, defaultMemCfg)

	t.Cleanup(func() {
		keys, _ := rdb.Keys(ctx, "mem:test-maint-*").Result()
		if len(keys) > 0 {
			rdb.Del(ctx, keys...)
		}
		sessKeys, _ := rdb.Keys(ctx, redisclient.KeyPrefixSession+"test-maint-*").Result()
		if len(sessKeys) > 0 {
			rdb.Del(ctx, sessKeys...)
		}
		rdb.Del(ctx, redisclient.KeyPrefixProjectSessions+maintProject)
		rdb.Del(ctx, redisclient.KeyPrefixMaintLastRun+maintProject)
		rdb.SRem(ctx, redisclient.KeyProjects, maintProject)
		_ = emb.Close()
		_ = rdb.Close()
	})

	return store, rdb
}

func TestMaintenance(t *testing.T) {
	store, rdb := newMaintTestStore(t)
	ctx := context.Background()

	t.Run("AutoForget", func(t *testing.T) {
		now := time.Now().Unix()
		staleTime := now - (91 * 24 * 3600) // 91 days ago

		// Memory with low importance and stale access: should be forgotten
		stale := &memory.Memory{
			ID: "test-maint-stale", Content: "stale low importance memory",
			Type: memory.Fact, Project: maintProject, Tags: "test",
			Importance: 1, CreatedAt: staleTime, LastAccessed: staleTime, AccessCount: 0,
		}
		// Memory with high importance and stale access: should survive
		important := &memory.Memory{
			ID: "test-maint-important", Content: "important stale memory",
			Type: memory.Fact, Project: maintProject, Tags: "test",
			Importance: 8, CreatedAt: staleTime, LastAccessed: staleTime, AccessCount: 0,
		}
		// Memory with low importance but recent access: should survive
		recent := &memory.Memory{
			ID: "test-maint-recent", Content: "recent low importance memory",
			Type: memory.Fact, Project: maintProject, Tags: "test",
			Importance: 1, CreatedAt: now, LastAccessed: now, AccessCount: 0,
		}

		for _, m := range []*memory.Memory{stale, important, recent} {
			if err := store.Save(ctx, m); err != nil {
				t.Fatalf("Save %s: %v", m.ID, err)
			}
		}

		// Wait for index to update
		time.Sleep(500 * time.Millisecond)

		result, err := store.RunMaintenance(ctx, maintProject, defaultMaintCfg, defaultMemCfg)
		if err != nil {
			t.Fatalf("RunMaintenance: %v", err)
		}
		if result.Skipped {
			t.Fatal("expected maintenance to run, got skipped")
		}
		if result.ForgottenCount != 1 {
			t.Errorf("expected 1 forgotten, got %d", result.ForgottenCount)
		}

		// Verify stale is gone
		if _, err := store.Get(ctx, "test-maint-stale"); err == nil {
			t.Error("stale memory should have been deleted")
		}
		// Verify important survived
		if _, err := store.Get(ctx, "test-maint-important"); err != nil {
			t.Errorf("important memory should survive: %v", err)
		}
		// Verify recent survived
		if _, err := store.Get(ctx, "test-maint-recent"); err != nil {
			t.Errorf("recent memory should survive: %v", err)
		}

		// Cleanup for next subtests
		store.Delete(ctx, "test-maint-important")
		store.Delete(ctx, "test-maint-recent")
		rdb.Del(ctx, redisclient.KeyPrefixMaintLastRun+maintProject)
	})

	t.Run("PruneSessions", func(t *testing.T) {
		// Create 5 sessions, max is 3
		for i := 0; i < 5; i++ {
			sess := &memory.Session{
				ID:        fmt.Sprintf("test-maint-sess-%d", i),
				Project:   maintProject,
				StartedAt: time.Now().Unix() + int64(i), // increasing time
			}
			if err := store.SaveSession(ctx, sess); err != nil {
				t.Fatalf("SaveSession %d: %v", i, err)
			}
		}

		result, err := store.RunMaintenance(ctx, maintProject, defaultMaintCfg, defaultMemCfg)
		if err != nil {
			t.Fatalf("RunMaintenance: %v", err)
		}
		if result.PrunedSessions != 2 {
			t.Errorf("expected 2 pruned sessions, got %d", result.PrunedSessions)
		}

		// Verify only 3 sessions remain in sorted set
		remaining, _ := rdb.ZCard(ctx, redisclient.KeyPrefixProjectSessions+maintProject).Result()
		if remaining != 3 {
			t.Errorf("expected 3 remaining sessions, got %d", remaining)
		}

		// Verify the oldest 2 session hashes are deleted
		for i := 0; i < 2; i++ {
			exists, _ := rdb.Exists(ctx, fmt.Sprintf(redisclient.KeyPrefixSession+"test-maint-sess-%d", i)).Result()
			if exists > 0 {
				t.Errorf("session test-maint-sess-%d should have been pruned", i)
			}
		}
		// Verify newest 3 survive
		for i := 2; i < 5; i++ {
			exists, _ := rdb.Exists(ctx, fmt.Sprintf(redisclient.KeyPrefixSession+"test-maint-sess-%d", i)).Result()
			if exists == 0 {
				t.Errorf("session test-maint-sess-%d should survive", i)
			}
		}

		rdb.Del(ctx, redisclient.KeyPrefixMaintLastRun+maintProject)
	})

	t.Run("CleanupOrphans", func(t *testing.T) {
		// Add a stale sorted set entry pointing to a non-existent session.
		// Use a high score so pruneSessions keeps it (it prunes oldest first).
		rdb.ZAdd(ctx, redisclient.KeyPrefixProjectSessions+maintProject, goredis.Z{
			Score:  float64(time.Now().Unix() + 9999),
			Member: "test-maint-ghost",
		})

		// Clear throttle and run
		rdb.Del(ctx, redisclient.KeyPrefixMaintLastRun+maintProject)

		result, err := store.RunMaintenance(ctx, maintProject, defaultMaintCfg, defaultMemCfg)
		if err != nil {
			t.Fatalf("RunMaintenance: %v", err)
		}
		if !result.OrphanCleaned {
			t.Error("expected orphan cleanup to report cleaned=true")
		}

		// Verify ghost is removed from sorted set
		members, _ := rdb.ZRange(ctx, redisclient.KeyPrefixProjectSessions+maintProject, 0, -1).Result()
		for _, m := range members {
			if m == "test-maint-ghost" {
				t.Error("ghost session should have been removed from sorted set")
			}
		}
	})

	t.Run("LastRunStats", func(t *testing.T) {
		rdb.Del(ctx, redisclient.KeyPrefixMaintLastRun+maintProject)

		_, err := store.RunMaintenance(ctx, maintProject, defaultMaintCfg, defaultMemCfg)
		if err != nil {
			t.Fatalf("RunMaintenance: %v", err)
		}

		// Verify last_run is a hash with expected fields
		runKey := redisclient.KeyPrefixMaintLastRun + maintProject
		keyType, err := rdb.Type(ctx, runKey).Result()
		if err != nil {
			t.Fatalf("TYPE: %v", err)
		}
		if keyType != "hash" {
			t.Fatalf("expected hash, got %s", keyType)
		}

		fields, err := rdb.HGetAll(ctx, runKey).Result()
		if err != nil {
			t.Fatalf("HGETALL: %v", err)
		}

		for _, field := range []string{"timestamp", "forgotten_count", "pruned_sessions", "orphan_cleaned"} {
			if _, ok := fields[field]; !ok {
				t.Errorf("missing field %q in last_run hash", field)
			}
		}

		if fields["timestamp"] == "" || fields["timestamp"] == "0" {
			t.Error("timestamp should be a non-zero value")
		}

		// Verify TTL is set
		ttl, err := rdb.TTL(ctx, runKey).Result()
		if err != nil {
			t.Fatalf("TTL: %v", err)
		}
		if ttl <= 0 {
			t.Errorf("expected positive TTL, got %v", ttl)
		}
	})

	t.Run("SkipWhenRecent", func(t *testing.T) {
		// First run sets the throttle key
		rdb.Del(ctx, redisclient.KeyPrefixMaintLastRun+maintProject)
		_, err := store.RunMaintenance(ctx, maintProject, defaultMaintCfg, defaultMemCfg)
		if err != nil {
			t.Fatalf("first run: %v", err)
		}

		// Second run should be skipped
		result, err := store.RunMaintenance(ctx, maintProject, defaultMaintCfg, defaultMemCfg)
		if err != nil {
			t.Fatalf("second run: %v", err)
		}
		if !result.Skipped {
			t.Error("expected second run to be skipped")
		}
	})

	t.Run("DisabledConfig", func(t *testing.T) {
		disabled := defaultMaintCfg
		disabled.Enabled = false

		result, err := store.RunMaintenance(ctx, maintProject, disabled, defaultMemCfg)
		if err != nil {
			t.Fatalf("RunMaintenance: %v", err)
		}
		if !result.Skipped {
			t.Error("expected disabled maintenance to be skipped")
		}
	})
}
