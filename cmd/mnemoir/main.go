package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/alle-bartoli/mnemoir/internal/compressor"
	"github.com/alle-bartoli/mnemoir/internal/config"
	"github.com/alle-bartoli/mnemoir/internal/embedding"
	mcpserver "github.com/alle-bartoli/mnemoir/internal/mcp"
	"github.com/alle-bartoli/mnemoir/internal/memory"
	redisclient "github.com/alle-bartoli/mnemoir/internal/redis"
	"github.com/mark3labs/mcp-go/server"
)

var version = "0.0.0"

func main() {
	configPath := flag.String("config", config.DefaultConfigPath(), "Path to config file")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("mnemoir", version)
		os.Exit(0)
	}

	// Set up structured logging to file (stderr is reserved for MCP stdio transport)
	logFile := initLogger()
	if logFile != nil {
		defer logFile.Close()
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Redis client
	rc, err := redisclient.NewClient(cfg.Redis)
	if err != nil {
		slog.Error("Failed to create Redis client", "error", err)
		os.Exit(1)
	}
	defer rc.Close()

	if pingErr := rc.Ping(ctx); pingErr != nil {
		slog.Error("Redis connection failed", "addr", cfg.Redis.Addr, "error", pingErr)
		os.Exit(1)
	}

	// Ensure RediSearch index
	if idxErr := redisclient.EnsureIndex(ctx, rc, cfg.Embedding.Dimension); idxErr != nil {
		slog.Error("Failed to ensure search index", "dimension", cfg.Embedding.Dimension, "error", idxErr)
		os.Exit(1)
	}

	// Initialize embedder
	emb, err := embedding.NewEmbedder(cfg.Embedding)
	if err != nil {
		slog.Error("Failed to create embedder", "provider", cfg.Embedding.Provider, "error", err)
		os.Exit(1)
	}
	defer emb.Close() // Release ONNX session and provider resources

	// Initialize compressor
	comp, err := compressor.NewCompressor(cfg.Compressor, rc.RDB())
	if err != nil {
		slog.Error("Failed to create compressor", "provider", cfg.Compressor.Provider, "error", err)
		os.Exit(1)
	}

	// Initialize memory store
	store := memory.NewStore(rc.RDB(), emb, cfg.Memory)

	// Start health endpoint (optional sideband HTTP)
	if ln, err := rc.StartHealthServer(cfg.Server.HealthAddr); err != nil {
		slog.Warn("Health endpoint disabled", "error", err)
	} else if ln != nil {
		defer ln.Close()
	}

	// Create and start MCP server
	s := mcpserver.NewServer(store, comp, cfg, rc.RDB())

	// Graceful shutdown: cancel context and let ServeStdio return naturally.
	// Avoids os.Exit which would bypass defers (embedder close, Redis close).
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("Server started",
		"embedding", cfg.Embedding.Provider,
		"compressor", cfg.Compressor.Provider,
		"redis", cfg.Redis.Addr,
	)

	go func() {
		<-sigCh
		slog.Info("Shutting down gracefully...")
		cancel()
	}()

	if err := server.ServeStdio(s); err != nil {
		slog.Error("MCP server error", "error", err)
		os.Exit(1)
	}
	slog.Info("Server stopped")
}

// initLogger sets up slog with JSON output to ~/.mnemoir/mnemoir.log.
// Falls back to stderr if the log file cannot be opened.
func initLogger() *os.File {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	logPath := filepath.Join(home, ".mnemoir", "mnemoir.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil
	}
	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(handler))
	// Redirect standard log package to slog for libraries using log.Printf
	log.SetOutput(&slogWriter{})
	log.SetFlags(0)
	return f
}

// slogWriter bridges standard log.Printf calls into slog.
type slogWriter struct{}

func (w *slogWriter) Write(p []byte) (n int, err error) {
	slog.Info(string(p))
	return len(p), nil
}
