package memory

import (
	"context"
	"testing"
	"time"

	"github.com/alle-bartoli/agentmem/internal/config"
	"github.com/alle-bartoli/agentmem/internal/embedding"
	redisclient "github.com/alle-bartoli/agentmem/internal/redis"
	goredis "github.com/redis/go-redis/v9"
)

const searchTestProject = "test-search"

// testMemories returns a set of diverse memories for search testing.
func testMemories() []*Memory {
	now := time.Now().Unix()
	return []*Memory{
		{ID: "test-s-001", Content: "Redis runs on port 6379 by default", Type: Fact, Project: searchTestProject, Tags: "redis,config", Importance: 7, CreatedAt: now, LastAccessed: now},
		{ID: "test-s-002", Content: "We use the repository pattern to separate storage from business logic", Type: Concept, Project: searchTestProject, Tags: "architecture,pattern", Importance: 6, CreatedAt: now, LastAccessed: now},
		{ID: "test-s-003", Content: "Go goroutines are lightweight threads managed by the Go runtime", Type: Fact, Project: searchTestProject, Tags: "go,concurrency", Importance: 5, CreatedAt: now, LastAccessed: now},
		{ID: "test-s-004", Content: "Docker Compose orchestrates multi-container applications", Type: Fact, Project: searchTestProject, Tags: "docker,infra", Importance: 5, CreatedAt: now, LastAccessed: now},
		{ID: "test-s-005", Content: "HNSW is an approximate nearest neighbor algorithm used for vector search", Type: Concept, Project: searchTestProject, Tags: "search,algorithm", Importance: 8, CreatedAt: now, LastAccessed: now},
		{ID: "test-s-006", Content: "Debugged a connection timeout issue with the Redis client last session", Type: Narrative, Project: searchTestProject, Tags: "debug,redis", Importance: 4, CreatedAt: now, LastAccessed: now},
		{ID: "test-s-007", Content: "TF-IDF stands for Term Frequency Inverse Document Frequency", Type: Fact, Project: searchTestProject, Tags: "search,scoring", Importance: 5, CreatedAt: now, LastAccessed: now},
		{ID: "test-s-008", Content: "Cosine similarity measures the angle between two vectors in high-dimensional space", Type: Concept, Project: searchTestProject, Tags: "math,search", Importance: 7, CreatedAt: now, LastAccessed: now},
	}
}

func newSearchTestStore(t *testing.T) *Store {
	t.Helper()

	rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// Ensure index exists with correct dimension
	rc, err := redisclient.NewClient(config.RedisConfig{Addr: "localhost:6379"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := redisclient.EnsureIndex(ctx, rc, 384); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	rc.Close()

	emb, err := embedding.NewLocalEmbedder(config.EmbeddingLocalConfig{
		Model:    "sentence-transformers/all-MiniLM-L6-v2",
		ModelDir: "~/.agentmem/models",
	}, 384)
	if err != nil {
		t.Fatalf("NewLocalEmbedder: %v", err)
	}

	store := NewStore(rdb, emb)

	// Save all test memories
	mems := testMemories()
	for _, m := range mems {
		if err := store.Save(ctx, m); err != nil {
			t.Fatalf("Save %s: %v", m.ID, err)
		}
	}

	// Wait for RediSearch to index
	time.Sleep(500 * time.Millisecond)

	t.Cleanup(func() {
		for _, m := range mems {
			_ = rdb.Del(ctx, "mem:"+m.ID).Err()
		}
		rdb.SRem(ctx, "projects", searchTestProject)
		_ = emb.Close()
		_ = rdb.Close()
	})

	return store
}

func TestVectorSearch_SemanticMatch(t *testing.T) {
	store := newSearchTestStore(t)
	ctx := context.Background()
	filters := SearchFilters{Project: searchTestProject}

	// "in-memory data store default port" should find "Redis runs on port 6379" via semantic
	// similarity even though the exact words don't appear in the content
	results, err := store.VectorSearch(ctx, "in-memory data store default port", filters, 3)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("VectorSearch returned 0 results")
	}

	// Top result should be about Redis (most related to "in-memory data store")
	t.Logf("Query: 'in-memory data store default port'")
	for i, r := range results {
		t.Logf("  #%d [%.4f] %s", i+1, r.Score, r.Memory.Content)
	}

	foundRedis := false
	for _, r := range results {
		if r.Memory.ID == "test-s-001" {
			foundRedis = true
			break
		}
	}
	if !foundRedis {
		t.Error("VectorSearch for 'in-memory data store default port' should find Redis memory in top 3")
	}
}

func TestVectorSearch_ConceptualMatch(t *testing.T) {
	store := newSearchTestStore(t)
	ctx := context.Background()
	filters := SearchFilters{Project: searchTestProject}

	// "how to measure similarity between embeddings" should find cosine similarity concept
	results, err := store.VectorSearch(ctx, "how to measure similarity between embeddings", filters, 3)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("VectorSearch returned 0 results")
	}

	t.Logf("Query: 'how to measure similarity between embeddings'")
	for i, r := range results {
		t.Logf("  #%d [%.4f] %s", i+1, r.Score, r.Memory.Content)
	}

	foundCosine := false
	for _, r := range results {
		if r.Memory.ID == "test-s-008" {
			foundCosine = true
			break
		}
	}
	if !foundCosine {
		t.Error("VectorSearch for 'similarity between embeddings' should find cosine similarity memory in top 3")
	}
}

func TestFullTextSearch_ExactKeyword(t *testing.T) {
	store := newSearchTestStore(t)
	ctx := context.Background()
	filters := SearchFilters{Project: searchTestProject}

	// FTS should find exact keyword match for "goroutines"
	results, err := store.FullTextSearch(ctx, "goroutines", filters, 3)
	if err != nil {
		t.Fatalf("FullTextSearch: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("FullTextSearch returned 0 results")
	}

	t.Logf("Query: 'goroutines'")
	for i, r := range results {
		t.Logf("  #%d [%.4f] %s", i+1, r.Score, r.Memory.Content)
	}

	if results[0].Memory.ID != "test-s-003" {
		t.Errorf("FullTextSearch top result should be goroutines memory, got %s", results[0].Memory.ID)
	}
}

func TestFullTextSearch_MultiWord(t *testing.T) {
	store := newSearchTestStore(t)
	ctx := context.Background()
	filters := SearchFilters{Project: searchTestProject}

	// FTS for "vector search algorithm"
	results, err := store.FullTextSearch(ctx, "vector search algorithm", filters, 3)
	if err != nil {
		t.Fatalf("FullTextSearch: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("FullTextSearch returned 0 results")
	}

	t.Logf("Query: 'vector search algorithm'")
	for i, r := range results {
		t.Logf("  #%d [%.4f] %s", i+1, r.Score, r.Memory.Content)
	}

	// HNSW memory contains "vector search" and "algorithm"
	foundHNSW := false
	for _, r := range results {
		if r.Memory.ID == "test-s-005" {
			foundHNSW = true
			break
		}
	}
	if !foundHNSW {
		t.Error("FullTextSearch for 'vector search algorithm' should find HNSW memory")
	}
}

func TestHybridSearch_CombinesResults(t *testing.T) {
	store := newSearchTestStore(t)
	ctx := context.Background()
	filters := SearchFilters{Project: searchTestProject}

	// Hybrid should return results from both vector and FTS
	results, err := store.HybridSearch(ctx, "Redis connection problem", filters, 5)
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("HybridSearch returned 0 results")
	}

	t.Logf("Query: 'Redis connection problem'")
	for i, r := range results {
		t.Logf("  #%d [%.4f] %s", i+1, r.Score, r.Memory.Content)
	}

	// Both the Redis fact and the Redis debug narrative should appear
	foundFact := false
	foundNarrative := false
	for _, r := range results {
		if r.Memory.ID == "test-s-001" {
			foundFact = true
		}
		if r.Memory.ID == "test-s-006" {
			foundNarrative = true
		}
	}
	if !foundFact {
		t.Error("HybridSearch should find Redis port fact")
	}
	if !foundNarrative {
		t.Error("HybridSearch should find Redis debug narrative")
	}
}

func TestVectorSearch_TypeFilter(t *testing.T) {
	store := newSearchTestStore(t)
	ctx := context.Background()

	// Filter only concepts
	filters := SearchFilters{Project: searchTestProject, Type: "concept"}
	results, err := store.VectorSearch(ctx, "search algorithms", filters, 5)
	if err != nil {
		t.Fatalf("VectorSearch with type filter: %v", err)
	}

	t.Logf("Query: 'search algorithms' (type=concept only)")
	for i, r := range results {
		t.Logf("  #%d [%.4f] [%s] %s", i+1, r.Score, r.Memory.Type, r.Memory.Content)
	}

	for _, r := range results {
		if r.Memory.Type != Concept {
			t.Errorf("Expected only concepts, got type=%s: %s", r.Memory.Type, r.Memory.Content)
		}
	}
}
