package embedding

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alle-bartoli/mnemoir/internal/config"
	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/pipelines"
)

const localDefaultDimension = 384

// LocalEmbedder runs ONNX models locally via hugot (pure Go, no CGO).
// The hugot pipeline is NOT goroutine-safe (data race in gomlx backend),
// so all calls are serialized via mu. This is required because HybridSearch
// runs vector and FTS searches concurrently, both of which may call Embed.
type LocalEmbedder struct {
	mu        sync.Mutex
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

	// hugot's HF hub client derives its download cache from XDG_CACHE_HOME, or
	// $HOME/.cache as fallback. On Windows $HOME is empty, so the cache resolves
	// to a cwd-relative ".cache" dir: the model re-downloads on every spawn and
	// concurrent spawns race on lock files, so it never completes and blocks
	// startup. Pin the cache to an absolute, owner-only dir beside the model dir.
	if os.Getenv("XDG_CACHE_HOME") == "" {
		cacheDir := filepath.Join(filepath.Dir(modelDir), "cache")
		if err := os.MkdirAll(cacheDir, 0o700); err == nil {
			_ = os.Setenv("XDG_CACHE_HOME", cacheDir)
		}
	}

	session, err := hugot.NewGoSession()
	if err != nil {
		return nil, fmt.Errorf("create hugot session: %w", err)
	}

	model := cfg.Model
	if model == "" {
		model = config.DefaultLocalEmbeddingModel
	}

	// hugot.DownloadModel populates the HF hub cache and symlinks blobs into a
	// snapshot dir. On Windows os.Symlink needs the create-symlink privilege
	// (Developer Mode / admin) and is flaky even when granted, so the download
	// intermittently fails with ERROR_PRIVILEGE_NOT_HELD. modelPath only needs a
	// flat set of files (model.onnx, tokenizer.json, config.json, ...), so fall
	// back to a direct, symlink-free download straight into it.
	modelPath := filepath.Join(modelDir, strings.ReplaceAll(model, "/", "_"))
	if !localModelComplete(modelPath) {
		dlOpts := hugot.NewDownloadOptions()
		dlOpts.OnnxFilePath = "onnx/model.onnx"
		if _, err = hugot.DownloadModel(model, modelDir, dlOpts); err != nil {
			if derr := downloadModelDirect(model, modelPath, dlOpts.Branch); derr != nil {
				_ = session.Destroy()
				return nil, fmt.Errorf("download model %s: hub: %v; direct: %w", model, err, derr)
			}
		}
	}

	feConfig := hugot.FeatureExtractionConfig{
		ModelPath:    modelPath,
		OnnxFilename: "model.onnx",
		Name:         "embedding",
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

// PRIVATE

// requiredModelFiles must exist in the model dir for hugot to load the model.
var requiredModelFiles = []string{"model.onnx", "tokenizer.json"}

// optionalModelFiles improve loading (config, special tokens) but are not fatal.
var optionalModelFiles = []string{"config.json", "tokenizer_config.json", "special_tokens_map.json", "vocab.txt"}

// @dev Reports whether modelPath already holds the files hugot needs, so we can
// skip downloading entirely.
func localModelComplete(modelPath string) bool {
	for _, name := range requiredModelFiles {
		info, err := os.Stat(filepath.Join(modelPath, name))
		if err != nil || info.Size() == 0 {
			return false
		}
	}
	return true
}

// @dev Downloads a model's flat file set directly from the HuggingFace resolve
// endpoint into destDir, bypassing the hub cache and its symlink step (which is
// unreliable on Windows). The .onnx file lives under onnx/ on the hub but is
// stored flat as model.onnx, matching hugot's FeatureExtractionConfig.
func downloadModelDirect(model, destDir, branch string) error {
	if branch == "" {
		branch = "main"
	}
	// Security: restrict model dir to owner only
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return fmt.Errorf("create model dir: %w", err)
	}

	base := fmt.Sprintf("https://huggingface.co/%s/resolve/%s/", model, branch)
	// remote path -> local filename
	files := map[string]string{
		"onnx/model.onnx":         "model.onnx",
		"tokenizer.json":          "tokenizer.json",
		"config.json":             "config.json",
		"tokenizer_config.json":   "tokenizer_config.json",
		"special_tokens_map.json": "special_tokens_map.json",
		"vocab.txt":               "vocab.txt",
	}
	required := map[string]bool{"model.onnx": true, "tokenizer.json": true}

	for remote, local := range files {
		err := downloadFile(base+remote, filepath.Join(destDir, local))
		if err != nil && required[local] {
			return fmt.Errorf("download %s: %w", remote, err)
		}
	}
	return nil
}

// @dev Downloads url to dst atomically (temp file then rename). Returns an error
// on any non-200 response so optional files can be skipped by the caller.
func downloadFile(url, dst string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: status %d", url, resp.StatusCode)
	}

	tmp := dst + ".downloading"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}

	if _, err = io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err = f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close %s: %w", tmp, err)
	}

	if err = os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s to %s: %w", tmp, dst, err)
	}
	return nil
}

// Embed generates an embedding vector for the given text.
// Serialized via mutex because hugot's gomlx backend is not goroutine-safe.
func (e *LocalEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	e.mu.Lock()
	result, err := e.pipeline.RunPipeline([]string{text})
	e.mu.Unlock()
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
