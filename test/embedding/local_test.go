package embedding_test

import (
	"context"
	"math"
	"testing"

	"github.com/alle-bartoli/agentmem/internal/config"
	"github.com/alle-bartoli/agentmem/internal/embedding"
)

func newTestLocalEmbedder(t *testing.T) *embedding.LocalEmbedder {
	t.Helper()
	cfg := config.EmbeddingLocalConfig{
		Model:    "sentence-transformers/all-MiniLM-L6-v2",
		ModelDir: "~/.agentmem/models",
	}
	emb, err := embedding.NewLocalEmbedder(cfg, 384)
	if err != nil {
		t.Fatalf("NewLocalEmbedder: %v", err)
	}
	t.Cleanup(func() { _ = emb.Close() })
	return emb
}

func TestLocalEmbedder(t *testing.T) {
	t.Run("Dimension", func(t *testing.T) {
		emb := newTestLocalEmbedder(t)
		if emb.Dimension() != 384 {
			t.Errorf("Dimension() = %d, want 384", emb.Dimension())
		}
	})

	t.Run("Embed", func(t *testing.T) {
		emb := newTestLocalEmbedder(t)
		ctx := context.Background()

		vec, err := emb.Embed(ctx, "hello world")
		if err != nil {
			t.Fatalf("Embed: %v", err)
		}
		if len(vec) != 384 {
			t.Fatalf("len(vec) = %d, want 384", len(vec))
		}

		allZero := true
		for _, v := range vec {
			if v != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			t.Error("embedding is all zeros")
		}
	})

	t.Run("DifferentTextsDifferentVectors", func(t *testing.T) {
		emb := newTestLocalEmbedder(t)
		ctx := context.Background()

		vecA, err := emb.Embed(ctx, "Redis is a fast in-memory database")
		if err != nil {
			t.Fatalf("Embed A: %v", err)
		}
		vecB, err := emb.Embed(ctx, "The weather is sunny today")
		if err != nil {
			t.Fatalf("Embed B: %v", err)
		}

		sim := cosineSimilarity(vecA, vecB)
		if sim > 0.7 {
			t.Errorf("unrelated texts have similarity %.4f, expected < 0.7", sim)
		}
	})

	t.Run("SimilarTextsHighSimilarity", func(t *testing.T) {
		emb := newTestLocalEmbedder(t)
		ctx := context.Background()

		vecA, err := emb.Embed(ctx, "Redis runs on port 6379")
		if err != nil {
			t.Fatalf("Embed A: %v", err)
		}
		vecB, err := emb.Embed(ctx, "The Redis server listens on port 6379")
		if err != nil {
			t.Fatalf("Embed B: %v", err)
		}

		sim := cosineSimilarity(vecA, vecB)
		if sim < 0.7 {
			t.Errorf("similar texts have similarity %.4f, expected > 0.7", sim)
		}
	})

	t.Run("EmptyText", func(t *testing.T) {
		emb := newTestLocalEmbedder(t)
		ctx := context.Background()

		vec, err := emb.Embed(ctx, "")
		if err != nil {
			t.Fatalf("Embed empty: %v", err)
		}
		if len(vec) != 384 {
			t.Fatalf("len(vec) = %d, want 384", len(vec))
		}
	})
}

// cosineSimilarity computes the cosine similarity between two vectors.
// Returns a value in [-1, 1] where 1.0 = identical, 0.0 = orthogonal.
func cosineSimilarity(a, b []float32) float64 {
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
