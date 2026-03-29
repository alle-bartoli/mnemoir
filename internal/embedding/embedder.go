// Package embedding provides vector embedding generation via multiple providers.
package embedding

import (
	"context"
	"fmt"

	"github.com/alle-bartoli/mnemoir/internal/config"
)

// @dev IEmbedder generates vector embeddings from text.
// Uses []float32 (not []float64) because:
//   - Redis VECTOR fields store FLOAT32
//   - Embedding models natively return float32
//   - Halves memory usage compared to float64 (4 bytes vs 8 per dimension)
//   - The extra precision of float64 is irrelevant for similarity search
//
// Close releases provider resources (e.g. ONNX session). Callers must defer Close().
type IEmbedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Dimension() int
	Close() error
}

// @dev NewEmbedder creates an embedder based on config provider (factory pattern).
// Supported providers: "openai" (1536d), "ollama" (768d), "local" (384d ONNX).
func NewEmbedder(cfg config.EmbeddingConfig) (IEmbedder, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAIEmbedder(cfg.OpenAI, cfg.Dimension)
	case "ollama":
		return NewOllamaEmbedder(cfg.Ollama, cfg.Dimension)
	case "local":
		return NewLocalEmbedder(cfg.Local, cfg.Dimension)
	default:
		return nil, fmt.Errorf("unknown embedding provider: %s", cfg.Provider)
	}
}
