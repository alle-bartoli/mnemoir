package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/alle-bartoli/agentmem/internal/memory"
)

func TestVectorSearch(t *testing.T) {
	t.Run("SemanticMatch", func(t *testing.T) {
		store, _ := newSearchTestStore(t)
		ctx := context.Background()
		filters := memory.SearchFilters{Project: searchTestProject}

		results, err := store.VectorSearch(ctx, "in-memory data store default port", filters, 3)
		if err != nil {
			t.Fatalf("VectorSearch: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("VectorSearch returned 0 results")
		}

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
			t.Error("should find Redis memory in top 3")
		}
	})

	t.Run("ConceptualMatch", func(t *testing.T) {
		store, _ := newSearchTestStore(t)
		ctx := context.Background()
		filters := memory.SearchFilters{Project: searchTestProject}

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
			t.Error("should find cosine similarity memory in top 3")
		}
	})

	t.Run("TypeFilter", func(t *testing.T) {
		store, _ := newSearchTestStore(t)
		ctx := context.Background()
		filters := memory.SearchFilters{Project: searchTestProject, Type: "concept"}

		results, err := store.VectorSearch(ctx, "search algorithms", filters, 5)
		if err != nil {
			t.Fatalf("VectorSearch with type filter: %v", err)
		}

		t.Logf("Query: 'search algorithms' (type=concept only)")
		for i, r := range results {
			t.Logf("  #%d [%.4f] [%s] %s", i+1, r.Score, r.Memory.Type, r.Memory.Content)
		}

		for _, r := range results {
			if r.Memory.Type != memory.Concept {
				t.Errorf("Expected only concepts, got type=%s: %s", r.Memory.Type, r.Memory.Content)
			}
		}
	})
}

func TestFullTextSearch(t *testing.T) {
	t.Run("ExactKeyword", func(t *testing.T) {
		store, _ := newSearchTestStore(t)
		ctx := context.Background()
		filters := memory.SearchFilters{Project: searchTestProject}

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
			t.Errorf("top result should be goroutines memory, got %s", results[0].Memory.ID)
		}
	})

	t.Run("MultiWord", func(t *testing.T) {
		store, _ := newSearchTestStore(t)
		ctx := context.Background()
		filters := memory.SearchFilters{Project: searchTestProject}

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

		foundHNSW := false
		for _, r := range results {
			if r.Memory.ID == "test-s-005" {
				foundHNSW = true
				break
			}
		}
		if !foundHNSW {
			t.Error("should find HNSW memory")
		}
	})
}

func TestHybridSearch(t *testing.T) {
	t.Run("CombinesResults", func(t *testing.T) {
		store, _ := newSearchTestStore(t)
		ctx := context.Background()
		filters := memory.SearchFilters{Project: searchTestProject}

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

		foundFact, foundNarrative := false, false
		for _, r := range results {
			if r.Memory.ID == "test-s-001" {
				foundFact = true
			}
			if r.Memory.ID == "test-s-006" {
				foundNarrative = true
			}
		}
		if !foundFact {
			t.Error("should find Redis port fact")
		}
		if !foundNarrative {
			t.Error("should find Redis debug narrative")
		}
	})

	t.Run("ImportanceAffectsRanking", func(t *testing.T) {
		store, rdb := newSearchTestStore(t)
		ctx := context.Background()

		now := time.Now().Unix()
		memHigh := &memory.Memory{
			ID: "test-s-rank-high", Content: "Configuration settings for the application server",
			Type: memory.Fact, Project: searchTestProject, Tags: "config", Importance: 9,
			CreatedAt: now, LastAccessed: now, AccessCount: 5,
		}
		memLow := &memory.Memory{
			ID: "test-s-rank-low", Content: "Configuration settings for the application server",
			Type: memory.Fact, Project: searchTestProject, Tags: "config", Importance: 2,
			CreatedAt: now, LastAccessed: now, AccessCount: 0,
		}

		if err := store.Save(ctx, memHigh); err != nil {
			t.Fatalf("Save high: %v", err)
		}
		if err := store.Save(ctx, memLow); err != nil {
			t.Fatalf("Save low: %v", err)
		}
		time.Sleep(500 * time.Millisecond)

		t.Cleanup(func() {
			rdb.Del(ctx, "mem:test-s-rank-high", "mem:test-s-rank-low")
		})

		filters := memory.SearchFilters{Project: searchTestProject}
		results, err := store.HybridSearch(ctx, "application server configuration", filters, 10)
		if err != nil {
			t.Fatalf("HybridSearch: %v", err)
		}

		t.Logf("Query: 'application server configuration'")
		for i, r := range results {
			t.Logf("  #%d [%.4f] imp=%d acc=%d %s", i+1, r.Score, r.Memory.Importance, r.Memory.AccessCount, r.Memory.Content)
		}

		highPos, lowPos := -1, -1
		for i, r := range results {
			if r.Memory.ID == "test-s-rank-high" {
				highPos = i
			}
			if r.Memory.ID == "test-s-rank-low" {
				lowPos = i
			}
		}

		if highPos == -1 || lowPos == -1 {
			t.Fatalf("Both ranking memories should appear: high=%d, low=%d", highPos, lowPos)
		}
		if highPos >= lowPos {
			t.Errorf("High importance (pos=%d) should rank above low importance (pos=%d)", highPos, lowPos)
		}
	})
}

func TestSpacedRepetition(t *testing.T) {
	t.Run("UpdateAccessPersistsImportance", func(t *testing.T) {
		store, _ := newSearchTestStore(t)
		ctx := context.Background()

		before, err := store.Get(ctx, "test-s-001")
		if err != nil {
			t.Fatalf("Get before: %v", err)
		}
		initialImportance := before.Importance

		for i := 0; i < 5; i++ {
			if err := store.UpdateAccess(ctx, "test-s-001"); err != nil {
				t.Fatalf("UpdateAccess #%d: %v", i+1, err)
			}
		}

		after, err := store.Get(ctx, "test-s-001")
		if err != nil {
			t.Fatalf("Get after: %v", err)
		}

		t.Logf("Importance: before=%d, after=%d, accessCount=%d", initialImportance, after.Importance, after.AccessCount)

		if after.AccessCount != before.AccessCount+5 {
			t.Errorf("AccessCount = %d, want %d", after.AccessCount, before.AccessCount+5)
		}
		if after.Importance < initialImportance {
			t.Errorf("Importance should not decrease for freshly recalled memory: got %d, initial was %d", after.Importance, initialImportance)
		}
	})
}
