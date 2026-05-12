package dbops_test

import (
	"bufio"
	"context"
	"encoding/binary"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/alle-bartoli/mnemoir/internal/config"
	"github.com/alle-bartoli/mnemoir/internal/memory"
	redisclient "github.com/alle-bartoli/mnemoir/internal/redis"
	goredis "github.com/redis/go-redis/v9"
)

const (
	testProject    = "test-dbops"
	testSessionID  = "test-dbops-sess-001"
	testMemoryID   = "test-dbops-mem-001"
	testMemoryID2  = "test-dbops-mem-002"
	testTag        = "test-dbops-tag"
	embeddingDim   = 384
)

// redisPassword reads MNEMOIR_REDIS_PASSWORD from env, falling back to .env at project root.
func redisPassword() string {
	if pw := os.Getenv("MNEMOIR_REDIS_PASSWORD"); pw != "" {
		return pw
	}
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

// newTestClient connects to local Redis and skips if unavailable.
// Ensures the RediSearch index exists with the test dimension.
func newTestClient(t *testing.T) *goredis.Client {
	t.Helper()

	rdb := goredis.NewClient(&goredis.Options{
		Addr:     "localhost:6379",
		Password: redisPassword(),
	})

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	rc, err := redisclient.NewClient(config.RedisConfig{Addr: "localhost:6379", Password: redisPassword()})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := redisclient.EnsureIndex(ctx, rc, embeddingDim); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	rc.Close()

	t.Cleanup(func() {
		cleanupTestKeys(t, rdb)
		_ = rdb.Close()
	})

	return rdb
}

// seedTestData inserts known memories, a session, project_sessions, and tag_frequencies
// directly into Redis without going through the Store or embedder.
func seedTestData(t *testing.T, rdb *goredis.Client) {
	t.Helper()
	ctx := context.Background()

	emb1 := fakeEmbedding(1.0)
	emb2 := fakeEmbedding(0.5)

	pipe := rdb.Pipeline()

	// Memory 1: fact
	pipe.HSet(ctx, redisclient.KeyPrefixMemory+testMemoryID, map[string]any{
		memory.FieldContent:      "Redis runs on port 6379",
		memory.FieldType:         string(memory.Fact),
		memory.FieldProject:      testProject,
		memory.FieldTags:         testTag,
		memory.FieldImportance:   "7",
		memory.FieldSessionID:    testSessionID,
		memory.FieldCreatedAt:    "1700000000",
		memory.FieldLastAccessed: "1700000100",
		memory.FieldAccessCount:  "3",
		memory.FieldEmbedding:    emb1,
	})

	// Memory 2: concept
	pipe.HSet(ctx, redisclient.KeyPrefixMemory+testMemoryID2, map[string]any{
		memory.FieldContent:      "HNSW is an approximate nearest neighbor algorithm",
		memory.FieldType:         string(memory.Concept),
		memory.FieldProject:      testProject,
		memory.FieldTags:         testTag,
		memory.FieldImportance:   "9",
		memory.FieldSessionID:    "",
		memory.FieldCreatedAt:    "1700000200",
		memory.FieldLastAccessed: "1700000300",
		memory.FieldAccessCount:  "1",
		memory.FieldEmbedding:    emb2,
	})

	// Session
	pipe.HSet(ctx, redisclient.KeyPrefixSession+testSessionID, map[string]any{
		memory.FieldProject:     testProject,
		memory.FieldStartedAt:   "1700000000",
		memory.FieldEndedAt:     "1700003600",
		memory.FieldSummary:     "Test session summary",
		memory.FieldMemoryCount: "2",
	})

	// Projects set
	pipe.SAdd(ctx, redisclient.KeyProjects, testProject)

	// project_sessions ZSET
	pipe.ZAdd(ctx, redisclient.KeyPrefixProjectSessions+testProject, goredis.Z{
		Score:  1700000000,
		Member: testSessionID,
	})

	// tags:frequency ZSET
	pipe.ZAdd(ctx, redisclient.KeyTagFrequency, goredis.Z{Score: 5, Member: testTag})

	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatalf("seedTestData: %v", err)
	}
}

// fakeEmbedding produces a deterministic float32 vector of embeddingDim dimensions.
// Each element is value * sin(i) to create a non-trivial binary pattern.
func fakeEmbedding(value float64) []byte {
	buf := make([]byte, embeddingDim*4)
	for i := range embeddingDim {
		f := float32(value * math.Sin(float64(i)+1))
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func cleanupTestKeys(t *testing.T, rdb *goredis.Client) {
	t.Helper()
	ctx := context.Background()

	pipe := rdb.Pipeline()
	pipe.Del(ctx, redisclient.KeyPrefixMemory+testMemoryID)
	pipe.Del(ctx, redisclient.KeyPrefixMemory+testMemoryID2)
	pipe.Del(ctx, redisclient.KeyPrefixSession+testSessionID)
	pipe.Del(ctx, redisclient.KeyPrefixProjectSessions+testProject)
	pipe.SRem(ctx, redisclient.KeyProjects, testProject)
	pipe.ZRem(ctx, redisclient.KeyTagFrequency, testTag)
	if _, err := pipe.Exec(ctx); err != nil {
		t.Logf("cleanup warning: %v", err)
	}
}
