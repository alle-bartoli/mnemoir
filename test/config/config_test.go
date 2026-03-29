package config_test

import (
	"testing"

	"github.com/alle-bartoli/mnemoir/internal/config"
)

func validConfig() *config.Config {
	return &config.Config{
		Redis: config.RedisConfig{Addr: "localhost:6379"},
		Compressor: config.CompressorConfig{
			Provider: "local",
		},
		Embedding: config.EmbeddingConfig{
			Provider:  "local",
			Dimension: 384,
		},
		Memory: config.MemoryConfig{
			DefaultImportance: 5,
			AutoDecay:         true,
			DecayInterval:     "168h",
			DecayFactor:       0.9,
			VectorWeight:      0.60,
			FTSWeight:         0.25,
			ImportanceWeight:  0.15,
			AccessBoostFactor: 0.3,
			AccessBoostCap:    2.0,
		},
		Session: config.SessionConfig{
			AutoSummarize:  true,
			MaxRecallItems: 20,
		},
	}
}

func TestValidate(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		cfg := validConfig()
		if err := cfg.Validate(); err != nil {
			t.Errorf("valid config should pass: %v", err)
		}
	})

	t.Run("InvalidEmbeddingProvider", func(t *testing.T) {
		cfg := validConfig()
		cfg.Embedding.Provider = "invalid"
		if err := cfg.Validate(); err == nil {
			t.Error("should reject unknown embedding provider")
		}
	})

	t.Run("InvalidCompressorProvider", func(t *testing.T) {
		cfg := validConfig()
		cfg.Compressor.Provider = "gpt4"
		if err := cfg.Validate(); err == nil {
			t.Error("should reject unknown compressor provider")
		}
	})

	t.Run("ZeroDimension", func(t *testing.T) {
		cfg := validConfig()
		cfg.Embedding.Dimension = 0
		if err := cfg.Validate(); err == nil {
			t.Error("should reject dimension <= 0")
		}
	})

	t.Run("NegativeDimension", func(t *testing.T) {
		cfg := validConfig()
		cfg.Embedding.Dimension = -1
		if err := cfg.Validate(); err == nil {
			t.Error("should reject negative dimension")
		}
	})

	t.Run("DecayFactorTooHigh", func(t *testing.T) {
		cfg := validConfig()
		cfg.Memory.DecayFactor = 1.5
		if err := cfg.Validate(); err == nil {
			t.Error("should reject decay_factor >= 1")
		}
	})

	t.Run("DecayFactorZero", func(t *testing.T) {
		cfg := validConfig()
		cfg.Memory.DecayFactor = 0
		if err := cfg.Validate(); err == nil {
			t.Error("should reject decay_factor <= 0")
		}
	})

	t.Run("InvalidDecayInterval", func(t *testing.T) {
		cfg := validConfig()
		cfg.Memory.DecayInterval = "not-a-duration"
		if err := cfg.Validate(); err == nil {
			t.Error("should reject unparseable decay_interval")
		}
	})

	t.Run("WeightsSumTooLow", func(t *testing.T) {
		cfg := validConfig()
		cfg.Memory.VectorWeight = 0.1
		cfg.Memory.FTSWeight = 0.1
		cfg.Memory.ImportanceWeight = 0.1
		if err := cfg.Validate(); err == nil {
			t.Error("should reject weights summing to 0.3")
		}
	})

	t.Run("ImportanceOutOfRange", func(t *testing.T) {
		cfg := validConfig()
		cfg.Memory.DefaultImportance = 11
		if err := cfg.Validate(); err == nil {
			t.Error("should reject default_importance > 10")
		}
	})

	t.Run("MaxRecallItemsZero", func(t *testing.T) {
		cfg := validConfig()
		cfg.Session.MaxRecallItems = 0
		if err := cfg.Validate(); err == nil {
			t.Error("should reject max_recall_items <= 0")
		}
	})

	t.Run("NegativeBoostFactor", func(t *testing.T) {
		cfg := validConfig()
		cfg.Memory.AccessBoostFactor = -1.0
		if err := cfg.Validate(); err == nil {
			t.Error("should reject negative access_boost_factor")
		}
	})

	t.Run("AllProvidersValid", func(t *testing.T) {
		for _, ep := range []string{"openai", "ollama", "local"} {
			for _, cp := range []string{"claude", "ollama", "local"} {
				cfg := validConfig()
				cfg.Embedding.Provider = ep
				cfg.Compressor.Provider = cp
				if err := cfg.Validate(); err != nil {
					t.Errorf("embedding=%s compressor=%s should be valid: %v", ep, cp, err)
				}
			}
		}
	})
}
