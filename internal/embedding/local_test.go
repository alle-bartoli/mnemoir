package embedding

import (
	"context"
	"math"
	"testing"

	"github.com/alle-bartoli/agentmem/internal/config"
)

// newTestLocalEmbedder creates a LocalEmbedder using the shared model cache.
// First run downloads ~80MB from HuggingFace.
func newTestLocalEmbedder(t *testing.T) *LocalEmbedder {
	t.Helper()
	cfg := config.EmbeddingLocalConfig{
		Model:    "sentence-transformers/all-MiniLM-L6-v2",
		ModelDir: "~/.agentmem/models",
	}
	emb, err := NewLocalEmbedder(cfg, localDefaultDimension)
	if err != nil {
		t.Fatalf("NewLocalEmbedder: %v", err)
	}
	t.Cleanup(func() { _ = emb.Close() })
	return emb
}

func TestLocalEmbedder_Dimension(t *testing.T) {
	emb := newTestLocalEmbedder(t)

	if emb.Dimension() != 384 {
		t.Errorf("Dimension() = %d, want 384", emb.Dimension())
	}
}

func TestLocalEmbedder_Embed(t *testing.T) {
	emb := newTestLocalEmbedder(t)
	ctx := context.Background()

	vec, err := emb.Embed(ctx, "hello world")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if len(vec) != 384 {
		t.Fatalf("len(vec) = %d, want 384", len(vec))
	}

	// Sanity check: vector should not be all zeros
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
}

func TestLocalEmbedder_DifferentTextsDifferentVectors(t *testing.T) {
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
	// Unrelated sentences should have low similarity (< 0.7)
	if sim > 0.7 {
		t.Errorf("unrelated texts have similarity %.4f, expected < 0.7", sim)
	}
}

func TestLocalEmbedder_SimilarTextsHighSimilarity(t *testing.T) {
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
	// Similar sentences should have high similarity (> 0.7)
	if sim < 0.7 {
		t.Errorf("similar texts have similarity %.4f, expected > 0.7", sim)
	}
}

func TestLocalEmbedder_EmptyText(t *testing.T) {
	emb := newTestLocalEmbedder(t)
	ctx := context.Background()

	vec, err := emb.Embed(ctx, "")
	if err != nil {
		t.Fatalf("Embed empty: %v", err)
	}

	if len(vec) != 384 {
		t.Fatalf("len(vec) = %d, want 384", len(vec))
	}
}

// PRIVATE

// cosineSimilarity computes the cosine similarity between two vectors.
// Returns a value in [-1, 1] where:
//   - 1.0 = identical direction (semantically identical)
//   - 0.0 = orthogonal (no semantic relation)
//   - -1.0 = opposite direction (semantically opposite)
//
// Formula: cos(θ) = (A · B) / (||A|| * ||B||)
// where ||A|| is the norm (length) of vector A: sqrt(a1² + a2² + ... + an²)
//
// Example with 3d vectors:
//   A = [1, 2, 3], B = [4, 5, 6]
//   A · B  = (1*4) + (2*5) + (3*6) = 32
//   ||A||  = sqrt(1+4+9)   = sqrt(14) ≈ 3.74
//   ||B||  = sqrt(16+25+36) = sqrt(77) ≈ 8.77
//   cos(θ) = 32 / (3.74 * 8.77) ≈ 0.975 (very similar)
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
