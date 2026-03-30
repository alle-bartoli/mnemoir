package config

// Embedding provider identifiers.
const (
	EmbeddingProviderOpenAI = "openai"
	EmbeddingProviderOllama = "ollama"
	EmbeddingProviderLocal  = "local"
)

// Compressor provider identifiers.
const (
	CompressorProviderClaude = "claude"
	CompressorProviderOllama = "ollama"
	CompressorProviderLocal  = "local"
)

// Environment variable names used in configuration and provider initialization.
const (
	EnvAnthropicAPIKey = "ANTHROPIC_API_KEY"
	EnvOpenAIAPIKey    = "OPENAI_API_KEY"
	EnvHome            = "HOME"
	EnvRedisPassword   = "MNEMOIR_REDIS_PASSWORD"
)

// Default configuration values shared between config defaults and provider constructors.
const (
	DefaultRedisAddr             = "localhost:6379"
	DefaultOllamaURL             = "http://localhost:11434"
	DefaultClaudeModel           = "claude-haiku-4-5-20251001"
	DefaultOllamaCompressorModel = "llama3.2"
	DefaultOpenAIEmbeddingModel  = "text-embedding-3-small"
	DefaultOllamaEmbeddingModel  = "nomic-embed-text"
	DefaultLocalEmbeddingModel   = "sentence-transformers/all-MiniLM-L6-v2"
	DefaultModelDir              = "~/.mnemoir/models"
	DefaultDecayInterval         = "168h"
	DefaultMinRunInterval        = "1h"
)
