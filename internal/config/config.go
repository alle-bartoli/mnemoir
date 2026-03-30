// Package config handles TOML configuration loading and defaults.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// RedisConfig holds Redis connection parameters.
type RedisConfig struct {
	Addr     string `toml:"addr"`
	Password string `toml:"password"`
	DB       int    `toml:"db"`
	TLS      bool   `toml:"tls"`
}

// CompressorClaudeConfig holds Anthropic API settings.
type CompressorClaudeConfig struct {
	Model string `toml:"model"`
}

// CompressorOllamaConfig holds Ollama compressor settings.
type CompressorOllamaConfig struct {
	URL   string `toml:"url"`
	Model string `toml:"model"`
}

// CompressorConfig holds compressor provider settings.
type CompressorConfig struct {
	Provider string                 `toml:"provider"`
	Claude   CompressorClaudeConfig `toml:"claude"`
	Ollama   CompressorOllamaConfig `toml:"ollama"`
}

// EmbeddingOpenAIConfig holds OpenAI embedding settings.
type EmbeddingOpenAIConfig struct {
	Model string `toml:"model"`
}

// EmbeddingOllamaConfig holds Ollama embedding settings.
type EmbeddingOllamaConfig struct {
	URL   string `toml:"url"`
	Model string `toml:"model"`
}

// EmbeddingLocalConfig holds local ONNX embedding settings.
type EmbeddingLocalConfig struct {
	Model    string `toml:"model"`
	ModelDir string `toml:"model_dir"`
}

// EmbeddingConfig holds embedding provider settings.
type EmbeddingConfig struct {
	Provider  string                `toml:"provider"`
	Dimension int                   `toml:"dimension"`
	OpenAI    EmbeddingOpenAIConfig `toml:"openai"`
	Ollama    EmbeddingOllamaConfig `toml:"ollama"`
	Local     EmbeddingLocalConfig  `toml:"local"`
}

// MemoryConfig holds memory behavior settings.
type MemoryConfig struct {
	DefaultImportance int     `toml:"default_importance"`
	AutoDecay         bool    `toml:"auto_decay"`
	DecayInterval     string  `toml:"decay_interval"`
	DecayFactor       float64 `toml:"decay_factor"`
	VectorWeight      float64 `toml:"vector_weight"`
	FTSWeight         float64 `toml:"fts_weight"`
	ImportanceWeight  float64 `toml:"importance_weight"`
	AccessBoostFactor float64 `toml:"access_boost_factor"`
	AccessBoostCap    float64 `toml:"access_boost_cap"`
}

// ParsedDecayInterval returns the decay interval as time.Duration.
func (m MemoryConfig) ParsedDecayInterval() (time.Duration, error) {
	return time.ParseDuration(m.DecayInterval)
}

// SessionConfig holds session behavior settings.
type SessionConfig struct {
	AutoSummarize  bool `toml:"auto_summarize"`
	MaxRecallItems int  `toml:"max_recall_items"`
}

// ServerConfig holds server-level settings.
type ServerConfig struct {
	HealthAddr string `toml:"health_addr"` // e.g. ":9090", empty to disable
}

// MaintenanceConfig holds automatic cleanup settings.
type MaintenanceConfig struct {
	Enabled              bool    `toml:"enabled"`
	ForgetThreshold      float64 `toml:"forget_threshold"`       // effective importance <= this
	ForgetInactiveDays   int     `toml:"forget_inactive_days"`   // days since last access
	MaxSessionsPerProject int    `toml:"max_sessions_per_project"`
	MinRunInterval       string  `toml:"min_run_interval"`       // e.g. "1h"
}

// ParsedMinRunInterval returns the minimum interval between maintenance runs.
func (m MaintenanceConfig) ParsedMinRunInterval() (time.Duration, error) {
	return time.ParseDuration(m.MinRunInterval)
}

// Config is the root configuration struct.
type Config struct {
	Redis       RedisConfig       `toml:"redis"`
	Compressor  CompressorConfig  `toml:"compressor"`
	Embedding   EmbeddingConfig   `toml:"embedding"`
	Memory      MemoryConfig      `toml:"memory"`
	Session     SessionConfig     `toml:"session"`
	Server      ServerConfig      `toml:"server"`
	Maintenance MaintenanceConfig `toml:"maintenance"`
}

// DefaultConfigPath returns ~/.mnemoir/config.toml.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(home, ".mnemoir", "config.toml")
}

// EnsureConfigDir creates ~/.mnemoir if it doesn't exist.
func EnsureConfigDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(home, ".mnemoir")
	return os.MkdirAll(dir, 0o700) // Security: restrict dir to owner only
}

// Load reads and parses a TOML config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	// Security: expand only allowed env vars, not arbitrary ones
	expanded := expandAllowedEnv(string(data))

	cfg := &Config{
		Redis: RedisConfig{
			Addr: DefaultRedisAddr,
			DB:   0,
		},
		Compressor: CompressorConfig{
			Provider: CompressorProviderClaude,
			Claude:   CompressorClaudeConfig{Model: DefaultClaudeModel},
			Ollama:   CompressorOllamaConfig{URL: DefaultOllamaURL, Model: DefaultOllamaCompressorModel},
		},
		Embedding: EmbeddingConfig{
			Provider:  EmbeddingProviderOpenAI,
			Dimension: 1536,
			OpenAI:    EmbeddingOpenAIConfig{Model: DefaultOpenAIEmbeddingModel},
			Ollama:    EmbeddingOllamaConfig{URL: DefaultOllamaURL, Model: DefaultOllamaEmbeddingModel},
			Local:     EmbeddingLocalConfig{Model: DefaultLocalEmbeddingModel, ModelDir: DefaultModelDir},
		},
		Memory: MemoryConfig{
			DefaultImportance: 5,
			AutoDecay:         true,
			DecayInterval:     DefaultDecayInterval,
			DecayFactor:       0.9,
			VectorWeight:      0.60,
			FTSWeight:         0.25,
			ImportanceWeight:  0.15,
			AccessBoostFactor: 0.3,
			AccessBoostCap:    2.0,
		},
		Session: SessionConfig{
			AutoSummarize:  true,
			MaxRecallItems: 20,
		},
		Maintenance: MaintenanceConfig{
			Enabled:              true,
			ForgetThreshold:      2.0,
			ForgetInactiveDays:   90,
			MaxSessionsPerProject: 50,
			MinRunInterval:       DefaultMinRunInterval,
		},
	}

	if err := toml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// Validate checks all config invariants and returns the first error found.
func (c *Config) Validate() error {
	// Embedding
	validEmbeddingProviders := map[string]bool{
		EmbeddingProviderOpenAI: true,
		EmbeddingProviderOllama: true,
		EmbeddingProviderLocal:  true,
	}
	if !validEmbeddingProviders[c.Embedding.Provider] {
		return fmt.Errorf("embedding.provider must be one of: %s, %s, %s (got %q)",
			EmbeddingProviderOpenAI, EmbeddingProviderOllama, EmbeddingProviderLocal, c.Embedding.Provider)
	}
	if c.Embedding.Dimension <= 0 {
		return fmt.Errorf("embedding.dimension must be > 0 (got %d)", c.Embedding.Dimension)
	}

	// Compressor
	validCompressorProviders := map[string]bool{
		CompressorProviderClaude: true,
		CompressorProviderOllama: true,
		CompressorProviderLocal:  true,
	}
	if !validCompressorProviders[c.Compressor.Provider] {
		return fmt.Errorf("compressor.provider must be one of: %s, %s, %s (got %q)",
			CompressorProviderClaude, CompressorProviderOllama, CompressorProviderLocal, c.Compressor.Provider)
	}

	// Memory
	if c.Memory.DefaultImportance < 1 || c.Memory.DefaultImportance > 10 {
		return fmt.Errorf("memory.default_importance must be 1-10 (got %d)", c.Memory.DefaultImportance)
	}
	if c.Memory.DecayFactor <= 0 || c.Memory.DecayFactor >= 1 {
		return fmt.Errorf("memory.decay_factor must be in (0, 1) (got %f)", c.Memory.DecayFactor)
	}
	if _, err := time.ParseDuration(c.Memory.DecayInterval); err != nil {
		return fmt.Errorf("memory.decay_interval is not a valid duration: %w", err)
	}
	// Weights should approximately sum to 1.0 (allow small tolerance)
	weightSum := c.Memory.VectorWeight + c.Memory.FTSWeight + c.Memory.ImportanceWeight
	if weightSum < 0.9 || weightSum > 1.1 {
		return fmt.Errorf("memory weights (vector+fts+importance) should sum to ~1.0 (got %.2f)", weightSum)
	}
	if c.Memory.AccessBoostFactor < 0 {
		return fmt.Errorf("memory.access_boost_factor must be >= 0 (got %f)", c.Memory.AccessBoostFactor)
	}
	if c.Memory.AccessBoostCap < 0 {
		return fmt.Errorf("memory.access_boost_cap must be >= 0 (got %f)", c.Memory.AccessBoostCap)
	}

	// Session
	if c.Session.MaxRecallItems <= 0 {
		return fmt.Errorf("session.max_recall_items must be > 0 (got %d)", c.Session.MaxRecallItems)
	}

	// Maintenance
	if c.Maintenance.Enabled {
		if c.Maintenance.ForgetThreshold < 0 || c.Maintenance.ForgetThreshold > 10 {
			return fmt.Errorf("maintenance.forget_threshold must be in [0, 10] (got %.1f)", c.Maintenance.ForgetThreshold)
		}
		if c.Maintenance.ForgetInactiveDays <= 0 {
			return fmt.Errorf("maintenance.forget_inactive_days must be > 0 (got %d)", c.Maintenance.ForgetInactiveDays)
		}
		if c.Maintenance.MaxSessionsPerProject <= 0 {
			return fmt.Errorf("maintenance.max_sessions_per_project must be > 0 (got %d)", c.Maintenance.MaxSessionsPerProject)
		}
		if _, err := time.ParseDuration(c.Maintenance.MinRunInterval); err != nil {
			return fmt.Errorf("maintenance.min_run_interval is not a valid duration: %w", err)
		}
	}

	return nil
}

// Security: only these env vars are expanded in config TOML
var allowedEnvVars = map[string]bool{
	EnvAnthropicAPIKey: true,
	EnvOpenAIAPIKey:    true,
	EnvHome:            true,
	EnvRedisPassword:   true,
}

func expandAllowedEnv(s string) string {
	return os.Expand(s, func(key string) string {
		if allowedEnvVars[key] {
			return os.Getenv(key)
		}
		return "$" + key
	})
}
