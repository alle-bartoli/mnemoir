package memory_test

import (
	"bufio"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alle-bartoli/mnemoir/internal/config"
	"github.com/alle-bartoli/mnemoir/internal/embedding"
	"github.com/alle-bartoli/mnemoir/internal/memory"
	redisclient "github.com/alle-bartoli/mnemoir/internal/redis"
	goredis "github.com/redis/go-redis/v9"
)

const (
	testProject       = "test-store"
	searchTestProject = "test-search"
)

// redisPassword reads MNEMOIR_REDIS_PASSWORD from env, falling back to .env file at project root.
func redisPassword() string {
	if pw := os.Getenv("MNEMOIR_REDIS_PASSWORD"); pw != "" {
		return pw
	}
	// Walk up from test/memory/ to find .env at project root
	for _, path := range []string{"../../.env", "../../../.env"} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "MNEMOIR_REDIS_PASSWORD=") {
				return strings.TrimPrefix(line, "MNEMOIR_REDIS_PASSWORD=")
			}
		}
	}
	return ""
}

func newRedisClient() *goredis.Client {
	return goredis.NewClient(&goredis.Options{
		Addr:     "localhost:6379",
		Password: redisPassword(),
	})
}

var defaultMemCfg = config.MemoryConfig{
	DefaultImportance: 5,
	AutoDecay:         true,
	DecayInterval:     "168h",
	DecayFactor:       0.9,
	VectorWeight:      0.60,
	FTSWeight:         0.25,
	ImportanceWeight:  0.15,
	AccessBoostFactor: 0.3,
	AccessBoostCap:    2.0,
}

// newTestStore creates a Store backed by real Redis and local embedder.
// Requires Redis Stack running on localhost:6379.
func newTestStore(t *testing.T) *memory.Store {
	t.Helper()

	rdb := newRedisClient()
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	emb, err := embedding.NewLocalEmbedder(config.EmbeddingLocalConfig{
		Model:    "sentence-transformers/all-MiniLM-L6-v2",
		ModelDir: "~/.mnemoir/models",
	}, 384)
	if err != nil {
		t.Fatalf("NewLocalEmbedder: %v", err)
	}

	store := memory.NewStore(rdb, emb, defaultMemCfg)

	t.Cleanup(func() {
		keys, _ := rdb.Keys(ctx, "mem:test-*").Result()
		if len(keys) > 0 {
			rdb.Del(ctx, keys...)
		}
		// Clean up session keys and sorted sets created by SaveSession
		sessKeys, _ := rdb.Keys(ctx, "session:test-*").Result()
		if len(sessKeys) > 0 {
			rdb.Del(ctx, sessKeys...)
		}
		rdb.Del(ctx, "project_sessions:"+testProject)
		rdb.SRem(ctx, "projects", testProject)
		_ = emb.Close()
		_ = rdb.Close()
	})

	return store
}

// newSearchTestStore creates a Store pre-loaded with diverse test memories.
// Requires Redis Stack running on localhost:6379.
func newSearchTestStore(t *testing.T) (*memory.Store, *goredis.Client) {
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

	mems := testMemories()
	for _, m := range mems {
		if err := store.Save(ctx, m); err != nil {
			t.Fatalf("Save %s: %v", m.ID, err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	t.Cleanup(func() {
		for _, m := range mems {
			_ = rdb.Del(ctx, "mem:"+m.ID).Err()
		}
		// Clean up session keys and sorted sets created by SaveSession
		sessKeys, _ := rdb.Keys(ctx, "session:test-*").Result()
		if len(sessKeys) > 0 {
			rdb.Del(ctx, sessKeys...)
		}
		rdb.Del(ctx, "project_sessions:"+searchTestProject)
		rdb.SRem(ctx, "projects", searchTestProject)
		_ = emb.Close()
		_ = rdb.Close()
	})

	return store, rdb
}

func newTestMemory(id, content string, memType memory.MemoryType) *memory.Memory {
	now := time.Now().Unix()
	return &memory.Memory{
		ID:           id,
		Content:      content,
		Type:         memType,
		Project:      testProject,
		Tags:         "test",
		Importance:   5,
		SessionID:    "",
		CreatedAt:    now,
		LastAccessed: now,
		AccessCount:  0,
	}
}

func testMemories() []*memory.Memory {
	now := time.Now().Unix()
	return []*memory.Memory{
		{ID: "test-s-001", Content: "Redis runs on port 6379 by default", Type: memory.Fact, Project: searchTestProject, Tags: "redis,config", Importance: 7, CreatedAt: now, LastAccessed: now},
		{ID: "test-s-002", Content: "We use the repository pattern to separate storage from business logic", Type: memory.Concept, Project: searchTestProject, Tags: "architecture,pattern", Importance: 6, CreatedAt: now, LastAccessed: now},
		{ID: "test-s-003", Content: "Go goroutines are lightweight threads managed by the Go runtime", Type: memory.Fact, Project: searchTestProject, Tags: "go,concurrency", Importance: 5, CreatedAt: now, LastAccessed: now},
		{ID: "test-s-004", Content: "Docker Compose orchestrates multi-container applications", Type: memory.Fact, Project: searchTestProject, Tags: "docker,infra", Importance: 5, CreatedAt: now, LastAccessed: now},
		{ID: "test-s-005", Content: "HNSW is an approximate nearest neighbor algorithm used for vector search", Type: memory.Concept, Project: searchTestProject, Tags: "search,algorithm", Importance: 8, CreatedAt: now, LastAccessed: now},
		{ID: "test-s-006", Content: "Debugged a connection timeout issue with the Redis client last session", Type: memory.Narrative, Project: searchTestProject, Tags: "debug,redis", Importance: 4, CreatedAt: now, LastAccessed: now},
		{ID: "test-s-007", Content: "TF-IDF stands for Term Frequency Inverse Document Frequency", Type: memory.Fact, Project: searchTestProject, Tags: "search,scoring", Importance: 5, CreatedAt: now, LastAccessed: now},
		{ID: "test-s-008", Content: "Cosine similarity measures the angle between two vectors in high-dimensional space", Type: memory.Concept, Project: searchTestProject, Tags: "math,search", Importance: 7, CreatedAt: now, LastAccessed: now},
	}
}
