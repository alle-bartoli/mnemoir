package dbops_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alle-bartoli/mnemoir/internal/dbops"
	"github.com/alle-bartoli/mnemoir/internal/memory"
	redisclient "github.com/alle-bartoli/mnemoir/internal/redis"
)

func TestDumpRestore(t *testing.T) {
	t.Run("RoundTrip", func(t *testing.T) {
		rdb := newTestClient(t)
		ctx := context.Background()

		seedTestData(t, rdb)

		var buf bytes.Buffer
		if err := dbops.Dump(ctx, rdb, &buf); err != nil {
			t.Fatalf("Dump: %v", err)
		}

		snapshot := buf.Bytes()
		if len(snapshot) == 0 {
			t.Fatal("Dump produced empty output")
		}

		// Wipe test keys before restore to prove data comes from snapshot.
		cleanupTestKeys(t, rdb)

		res, err := dbops.Restore(ctx, rdb, bytes.NewReader(snapshot))
		if err != nil {
			t.Fatalf("Restore: %v", err)
		}

		if res.Memories < 2 {
			t.Errorf("Memories restored = %d, want >= 2", res.Memories)
		}
		if res.Sessions < 1 {
			t.Errorf("Sessions restored = %d, want >= 1", res.Sessions)
		}
		if res.Projects < 1 {
			t.Errorf("Projects restored = %d, want >= 1", res.Projects)
		}

		// Verify all memory fields survive round-trip exactly.
		vals, err := rdb.HGetAll(ctx, redisclient.KeyPrefixMemory+testMemoryID).Result()
		if err != nil {
			t.Fatalf("HGetAll memory: %v", err)
		}
		if len(vals) == 0 {
			t.Fatal("memory not found after restore")
		}
		checks := map[string]string{
			memory.FieldContent:      "Redis runs on port 6379",
			memory.FieldType:         string(memory.Fact),
			memory.FieldProject:      testProject,
			memory.FieldTags:         testTag,
			memory.FieldImportance:   "7",
			memory.FieldSessionID:    testSessionID,
			memory.FieldCreatedAt:    "1700000000",
			memory.FieldLastAccessed: "1700000100",
			memory.FieldAccessCount:  "3",
		}
		for field, want := range checks {
			if got := vals[field]; got != want {
				t.Errorf("field %q = %q, want %q", field, got, want)
			}
		}

		// Verify session fields.
		sessVals, err := rdb.HGetAll(ctx, redisclient.KeyPrefixSession+testSessionID).Result()
		if err != nil {
			t.Fatalf("HGetAll session: %v", err)
		}
		if len(sessVals) == 0 {
			t.Fatal("session not found after restore")
		}
		if sessVals[memory.FieldSummary] != "Test session summary" {
			t.Errorf("session summary = %q, want %q", sessVals[memory.FieldSummary], "Test session summary")
		}
		if sessVals[memory.FieldMemoryCount] != "2" {
			t.Errorf("session memory_count = %q, want %q", sessVals[memory.FieldMemoryCount], "2")
		}

		// Verify projects set.
		isMember, err := rdb.SIsMember(ctx, redisclient.KeyProjects, testProject).Result()
		if err != nil {
			t.Fatalf("SIsMember: %v", err)
		}
		if !isMember {
			t.Error("project not found in projects set after restore")
		}

		// Verify project_sessions ZSET score preserved exactly.
		score, err := rdb.ZScore(ctx, redisclient.KeyPrefixProjectSessions+testProject, testSessionID).Result()
		if err != nil {
			t.Fatalf("ZScore project_sessions: %v", err)
		}
		if score != 1700000000 {
			t.Errorf("project_sessions score = %f, want 1700000000", score)
		}

		// Verify tags:frequency ZSET.
		tagScore, err := rdb.ZScore(ctx, redisclient.KeyTagFrequency, testTag).Result()
		if err != nil {
			t.Fatalf("ZScore tag_frequencies: %v", err)
		}
		if tagScore != 5 {
			t.Errorf("tag score = %f, want 5", tagScore)
		}

		// Verify RediSearch indexing completed (waitForReindex ran inside Restore).
		if !res.IndexingComplete {
			t.Error("IndexingComplete = false, want true")
		}

		// Verify FT.SEARCH can find the restored memories (proves index is live).
		// Hyphen in project name must be escaped for RediSearch TAG syntax.
		// Allow up to 2s extra in case of slow test environments.
		var searchCount int
		for range 4 {
			result, err := rdb.Do(ctx,
				"FT.SEARCH", redisclient.IndexName,
				"@project:{test\\-dbops}",
				"NOCONTENT", "LIMIT", 0, 0,
			).Result()
			if err == nil {
				if m, ok := result.(map[any]any); ok {
					if total, ok := m["total_results"].(int64); ok {
						searchCount = int(total)
					}
				}
			}
			if searchCount >= 2 {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if searchCount < 2 {
			t.Errorf("FT.SEARCH after restore = %d docs, want >= 2", searchCount)
		}
	})

	t.Run("EmbeddingFidelity", func(t *testing.T) {
		rdb := newTestClient(t)
		ctx := context.Background()

		seedTestData(t, rdb)

		// Capture original embedding bytes before dump.
		originalBytes, err := rdb.HGet(ctx, redisclient.KeyPrefixMemory+testMemoryID, memory.FieldEmbedding).Bytes()
		if err != nil {
			t.Fatalf("HGet embedding before dump: %v", err)
		}

		var buf bytes.Buffer
		if err := dbops.Dump(ctx, rdb, &buf); err != nil {
			t.Fatalf("Dump: %v", err)
		}

		cleanupTestKeys(t, rdb)

		if _, err := dbops.Restore(ctx, rdb, bytes.NewReader(buf.Bytes())); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		restoredBytes, err := rdb.HGet(ctx, redisclient.KeyPrefixMemory+testMemoryID, memory.FieldEmbedding).Bytes()
		if err != nil {
			t.Fatalf("HGet embedding after restore: %v", err)
		}

		if !bytes.Equal(originalBytes, restoredBytes) {
			t.Errorf("embedding bytes mismatch: got %d bytes, want %d bytes", len(restoredBytes), len(originalBytes))
		}
	})

	t.Run("EmbeddingDimInSnapshot", func(t *testing.T) {
		rdb := newTestClient(t)
		ctx := context.Background()

		seedTestData(t, rdb)

		var buf bytes.Buffer
		if err := dbops.Dump(ctx, rdb, &buf); err != nil {
			t.Fatalf("Dump: %v", err)
		}

		var snap map[string]any
		if err := json.Unmarshal(buf.Bytes(), &snap); err != nil {
			t.Fatalf("unmarshal snapshot: %v", err)
		}

		dim, ok := snap["embedding_dim"].(float64)
		if !ok || int(dim) != embeddingDim {
			t.Errorf("embedding_dim = %v, want %d", snap["embedding_dim"], embeddingDim)
		}
	})

	t.Run("DimensionMismatch", func(t *testing.T) {
		rdb := newTestClient(t)
		ctx := context.Background()

		// Craft a snapshot claiming a different embedding dimension.
		wrongDim := `{"version":1,"created_at":0,"embedding_dim":768,"memories":[],"sessions":[],"projects":[],"project_sessions":{},"tag_frequencies":[]}`
		_, err := dbops.Restore(ctx, rdb, strings.NewReader(wrongDim))
		if err == nil {
			t.Fatal("Restore with wrong dimension should return error, got nil")
		}
		if !strings.Contains(err.Error(), "embedding dimension mismatch") {
			t.Errorf("error = %q, want to contain 'embedding dimension mismatch'", err.Error())
		}
	})

	t.Run("VersionMismatch", func(t *testing.T) {
		rdb := newTestClient(t)
		ctx := context.Background()

		bad := `{"version":99,"created_at":0,"embedding_dim":384,"memories":[],"sessions":[],"projects":[],"project_sessions":{},"tag_frequencies":[]}`
		_, err := dbops.Restore(ctx, rdb, strings.NewReader(bad))
		if err == nil {
			t.Fatal("Restore with wrong version should return error, got nil")
		}
		if !strings.Contains(err.Error(), "unsupported snapshot version") {
			t.Errorf("error = %q, want to contain 'unsupported snapshot version'", err.Error())
		}
	})

	t.Run("EmptySnapshot", func(t *testing.T) {
		rdb := newTestClient(t)
		ctx := context.Background()

		// embedding_dim=0 means old snapshot without dim field: skip dimension check.
		empty := `{"version":1,"created_at":0,"embedding_dim":0,"memories":[],"sessions":[],"projects":[],"project_sessions":{},"tag_frequencies":[]}`
		res, err := dbops.Restore(ctx, rdb, strings.NewReader(empty))
		if err != nil {
			t.Fatalf("Restore empty snapshot: %v", err)
		}
		if res.Memories != 0 || res.Sessions != 0 || res.Projects != 0 {
			t.Errorf("expected zero counts, got memories=%d sessions=%d projects=%d",
				res.Memories, res.Sessions, res.Projects)
		}
	})

	t.Run("SnapshotIsValidJSON", func(t *testing.T) {
		rdb := newTestClient(t)
		ctx := context.Background()

		seedTestData(t, rdb)

		var buf bytes.Buffer
		if err := dbops.Dump(ctx, rdb, &buf); err != nil {
			t.Fatalf("Dump: %v", err)
		}

		var snap map[string]any
		if err := json.Unmarshal(buf.Bytes(), &snap); err != nil {
			t.Fatalf("snapshot is not valid JSON: %v", err)
		}
		for _, key := range []string{"version", "created_at", "embedding_dim", "memories", "sessions"} {
			if _, ok := snap[key]; !ok {
				t.Errorf("snapshot missing key %q", key)
			}
		}
	})
}
