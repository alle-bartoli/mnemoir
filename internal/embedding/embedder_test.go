package embedding

import (
	"testing"

	"github.com/alle-bartoli/mnemoir/internal/config"
)

func TestPrewarmLocalModelSkipsNonLocalProviders(t *testing.T) {
	for _, provider := range []string{config.EmbeddingProviderOpenAI, config.EmbeddingProviderOllama} {
		t.Run(provider, func(t *testing.T) {
			if err := PrewarmLocalModel(config.EmbeddingConfig{Provider: provider}); err != nil {
				t.Fatalf("PrewarmLocalModel() error = %v", err)
			}
		})
	}
}
