// Package embedding provides vector embedding generation via multiple providers.
package embedding

import (
	"context"
	"fmt"

	"github.com/alle-bartoli/agentmem/internal/config"
)

// IEmbedder generates vector embeddings from text.
type IEmbedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Dimension() int
}

// NewEmbedder creates an embedder based on config provider.
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
