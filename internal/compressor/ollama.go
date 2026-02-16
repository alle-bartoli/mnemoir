package compressor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/alle-bartoli/agentmem/internal/config"
)

// OllamaCompressor uses the Ollama generate API for compression.
type OllamaCompressor struct {
	url    string
	model  string
	client *http.Client
}

// NewOllamaCompressor creates an Ollama-backed compressor.
func NewOllamaCompressor(cfg config.CompressorOllamaConfig) (*OllamaCompressor, error) {
	url := cfg.URL
	if url == "" {
		url = "http://localhost:11434"
	}

	model := cfg.Model
	if model == "" {
		model = "llama3.2"
	}

	return &OllamaCompressor{
		url:    url,
		model:  model,
		client: &http.Client{},
	}, nil
}

type ollamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	System string `json:"system"`
	Stream bool   `json:"stream"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

// Compress sends observations to Ollama and parses structured memories.
func (c *OllamaCompressor) Compress(ctx context.Context, observations string) (*CompressResult, error) {
	body, err := json.Marshal(ollamaGenerateRequest{
		Model:  c.model,
		Prompt: observations,
		System: CompressPrompt,
		Stream: false,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := c.url + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result ollamaGenerateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", result.Error)
	}

	var compressed CompressResult
	if err := json.Unmarshal([]byte(result.Response), &compressed); err != nil {
		return nil, fmt.Errorf("parse compressed output: %w (raw: %s)", err, result.Response)
	}

	return &compressed, nil
}
