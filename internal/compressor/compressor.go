// Package compressor extracts structured memories from raw observations.
package compressor

import (
	"context"
	"fmt"

	"github.com/alle-bartoli/mnemoir/internal/config"
)

// ExtractedMemory is a single memory extracted by the compressor.
type ExtractedMemory struct {
	Content    string `json:"content"`
	Tags       string `json:"tags"`
	Importance int    `json:"importance"`
}

// CompressResult holds all memories extracted from observations.
type CompressResult struct {
	Facts      []ExtractedMemory `json:"facts"`
	Concepts   []ExtractedMemory `json:"concepts"`
	Narratives []ExtractedMemory `json:"narratives"`
}

// ICompressor extracts structured memories from raw observations.
type ICompressor interface {
	Compress(ctx context.Context, observations string) (*CompressResult, error)
}

// NewCompressor creates a compressor based on config provider.
func NewCompressor(cfg config.CompressorConfig) (ICompressor, error) {
	switch cfg.Provider {
	case "claude":
		return NewClaudeCompressor(cfg.Claude)
	case "ollama":
		return NewOllamaCompressor(cfg.Ollama)
	case "local":
		return NewLocalCompressor()
	default:
		return nil, fmt.Errorf("unknown compressor provider: %s", cfg.Provider)
	}
}

// CompressPrompt is the system prompt used to extract structured memories.
const CompressPrompt = `Extract structured memories from these observations. Return ONLY valid JSON with no additional text:
{
  "facts": [{"content": "...", "tags": "...", "importance": N}],
  "concepts": [{"content": "...", "tags": "...", "importance": N}],
  "narratives": [{"content": "...", "tags": "...", "importance": N}]
}

Rules:
- Facts: concrete data points (ports, versions, file paths, configs)
- Concepts: patterns, architectures, design decisions
- Narratives: what was done, why, and the outcome
- Importance 1-3: trivial, 4-6: useful, 7-9: critical, 10: must never forget
- Tags: comma-separated, lowercase, relevant keywords
- If no memories can be extracted for a category, use an empty array`
