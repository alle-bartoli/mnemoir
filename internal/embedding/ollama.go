package embedding

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

	"github.com/alle-bartoli/agentmem/internal/config"
)

// OllamaEmbedder calls the Ollama embeddings API.
type OllamaEmbedder struct {
	url       string
	model     string
	dimension int
	client    *http.Client
}

// NewOllamaEmbedder creates an Ollama-backed embedder.
func NewOllamaEmbedder(cfg config.EmbeddingOllamaConfig, dimension int) (*OllamaEmbedder, error) {
	url := cfg.URL
	if url == "" {
		url = "http://localhost:11434"
	}

	model := cfg.Model
	if model == "" {
		model = "nomic-embed-text"
	}

	// Security: prevent SSRF by restricting to localhost
	if err := validateOllamaURL(url); err != nil {
		return nil, err
	}

	return &OllamaEmbedder{
		url:       url,
		model:     model,
		dimension: dimension,
		client:    &http.Client{Timeout: 30 * time.Second}, // Security: prevent hanging connections
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

type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
	Error     string    `json:"error,omitempty"`
}

// Embed generates an embedding vector using Ollama.
func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(ollamaEmbedRequest{
		Model:  e.model,
		Prompt: text,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := e.url + "/api/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
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
		slog.Error("ollama embedding API error", "status", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("embedding service error (status %d)", resp.StatusCode)              // Security: return generic error to caller
	}

	var result ollamaEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", result.Error)
	}

	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	return result.Embedding, nil
}

// Dimension returns the configured vector dimension.
func (e *OllamaEmbedder) Dimension() int {
	return e.dimension
}

// Close is a no-op for HTTP-based embedders.
func (e *OllamaEmbedder) Close() error { return nil }
