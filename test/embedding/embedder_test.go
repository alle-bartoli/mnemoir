package embedding_test

import (
	"testing"

	"github.com/alle-bartoli/mnemoir/internal/config"
	"github.com/alle-bartoli/mnemoir/internal/embedding"
)

func TestPrewarmLocalModelSkipsNonLocalProviders(t *testing.T) {
	for _, provider := range []string{config.EmbeddingProviderOpenAI, config.EmbeddingProviderOllama} {
		t.Run(provider, func(t *testing.T) {
			if err := embedding.PrewarmLocalModel(config.EmbeddingConfig{Provider: provider}); err != nil {
				t.Fatalf("PrewarmLocalModel() error = %v", err)
			}
		})
	}
}
