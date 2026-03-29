package compressor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/alle-bartoli/mnemoir/internal/config"
)

// ClaudeCompressor uses the Anthropic Messages API for compression.
type ClaudeCompressor struct {
	apiKey string
	model  string
	client *http.Client
}

// NewClaudeCompressor creates an Anthropic-backed compressor.
func NewClaudeCompressor(cfg config.CompressorClaudeConfig) (*ClaudeCompressor, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable not set")
	}

	model := cfg.Model
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}

	return &ClaudeCompressor{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second}, // Security: prevent hanging connections
	}, nil
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	System    string           `json:"system"`
	Messages  []claudeMessage  `json:"messages"`
}

type claudeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Compress sends observations to Claude and parses structured memories.
func (c *ClaudeCompressor) Compress(ctx context.Context, observations string) (*CompressResult, error) {
	body, err := json.Marshal(claudeRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    CompressPrompt,
		Messages: []claudeMessage{
			{Role: "user", Content: observations},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()

	// Security: cap response body at 10MB to prevent memory exhaustion
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		slog.Error("anthropic API error", "status", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("compressor service error (status %d)", resp.StatusCode)     // Security: return generic error to caller
	}

	var result claudeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s", result.Error.Message)
	}

	if len(result.Content) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	var compressed CompressResult
	if err := json.Unmarshal([]byte(result.Content[0].Text), &compressed); err != nil {
		return nil, fmt.Errorf("parse compressed output: %w (raw: %s)", err, result.Content[0].Text)
	}

	return &compressed, nil
}
