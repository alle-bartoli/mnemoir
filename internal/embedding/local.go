package embedding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alle-bartoli/agentmem/internal/config"
	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/pipelines"
)

const localDefaultDimension = 384

// LocalEmbedder runs ONNX models locally via hugot (pure Go, no CGO).
type LocalEmbedder struct {
	session   *hugot.Session
	pipeline  *pipelines.FeatureExtractionPipeline
	dimension int
}

// NewLocalEmbedder creates a local embedder backed by an ONNX model.
// Downloads the model from HuggingFace on first use.
func NewLocalEmbedder(cfg config.EmbeddingLocalConfig, dimension int) (*LocalEmbedder, error) {
	modelDir := expandHome(cfg.ModelDir)
	// Security: restrict model dir to owner only
	if err := os.MkdirAll(modelDir, 0o700); err != nil {
		return nil, fmt.Errorf("create model dir: %w", err)
	}

	session, err := hugot.NewGoSession()
	if err != nil {
		return nil, fmt.Errorf("create hugot session: %w", err)
	}

	model := cfg.Model
	if model == "" {
		model = "sentence-transformers/all-MiniLM-L6-v2"
	}

	dlOpts := hugot.NewDownloadOptions()
	dlOpts.OnnxFilePath = "onnx/model.onnx"
	modelPath, err := hugot.DownloadModel(model, modelDir, dlOpts)
	if err != nil {
		_ = session.Destroy()
		return nil, fmt.Errorf("download model %s: %w", model, err)
	}

	feConfig := hugot.FeatureExtractionConfig{
		ModelPath: modelPath,
		Name:      "embedding",
	}
	pipeline, err := hugot.NewPipeline(session, feConfig)
	if err != nil {
		_ = session.Destroy()
		return nil, fmt.Errorf("create pipeline: %w", err)
	}

	if dimension == 0 {
		dimension = localDefaultDimension
	}

	return &LocalEmbedder{
		session:   session,
		pipeline:  pipeline,
		dimension: dimension,
	}, nil
}

// Embed generates an embedding vector for the given text.
func (e *LocalEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	result, err := e.pipeline.RunPipeline([]string{text})
	if err != nil {
		return nil, fmt.Errorf("local embed: %w", err)
	}

	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	return result.Embeddings[0], nil
}

// Dimension returns the configured vector dimension.
func (e *LocalEmbedder) Dimension() int {
	return e.dimension
}

// Close releases hugot session resources.
func (e *LocalEmbedder) Close() error {
	if e.session != nil {
		return e.session.Destroy()
	}
	return nil
}

// PRIVATE

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
