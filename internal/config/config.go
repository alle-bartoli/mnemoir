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

// Config is the root configuration struct.
type Config struct {
	Redis      RedisConfig      `toml:"redis"`
	Compressor CompressorConfig `toml:"compressor"`
	Embedding  EmbeddingConfig  `toml:"embedding"`
	Memory     MemoryConfig     `toml:"memory"`
	Session    SessionConfig    `toml:"session"`
}

// DefaultConfigPath returns ~/.agentmem/config.toml.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(home, ".agentmem", "config.toml")
}

// EnsureConfigDir creates ~/.agentmem if it doesn't exist.
func EnsureConfigDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(home, ".agentmem")
	return os.MkdirAll(dir, 0o755)
}

// Load reads and parses a TOML config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	// Expand environment variables in the TOML content
	expanded := os.ExpandEnv(string(data))

	cfg := &Config{
		Redis: RedisConfig{
			Addr: "localhost:6379",
			DB:   0,
		},
		Compressor: CompressorConfig{
			Provider: "claude",
			Claude:   CompressorClaudeConfig{Model: "claude-haiku-4-5-20251001"},
			Ollama:   CompressorOllamaConfig{URL: "http://localhost:11434", Model: "llama3.2"},
		},
		Embedding: EmbeddingConfig{
			Provider:  "openai",
			Dimension: 1536,
			OpenAI:    EmbeddingOpenAIConfig{Model: "text-embedding-3-small"},
			Ollama:    EmbeddingOllamaConfig{URL: "http://localhost:11434", Model: "nomic-embed-text"},
			Local:     EmbeddingLocalConfig{Model: "sentence-transformers/all-MiniLM-L6-v2", ModelDir: "~/.agentmem/models"},
		},
		Memory: MemoryConfig{
			DefaultImportance: 5,
			AutoDecay:         true,
			DecayInterval:     "168h",
			DecayFactor:       0.9,
		},
		Session: SessionConfig{
			AutoSummarize:  true,
			MaxRecallItems: 20,
		},
	}

	if err := toml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}
