package compressor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/alle-bartoli/mnemoir/internal/config"
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
		url = config.DefaultOllamaURL
	}

	model := cfg.Model
	if model == "" {
		model = config.DefaultOllamaCompressorModel
	}

	// Security: prevent SSRF by restricting to localhost
	if err := validateOllamaURL(url); err != nil {
		return nil, err
	}

	return &OllamaCompressor{
		url:    url,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second}, // Security: prevent hanging connections
	}, nil
}

func validateOllamaURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid ollama URL: %w", err)
	}
	host := u.Hostname()
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("ollama URL must point to localhost, got %q", host)
	}
	return nil
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

	// Security: cap response body at 10MB to prevent memory exhaustion
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		slog.Error("ollama compressor API error", "status", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("compressor service error (status %d)", resp.StatusCode)               // Security: return generic error to caller
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
