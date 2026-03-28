package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/alle-bartoli/agentmem/internal/compressor"
	"github.com/alle-bartoli/agentmem/internal/config"
	"github.com/alle-bartoli/agentmem/internal/embedding"
	mcpserver "github.com/alle-bartoli/agentmem/internal/mcp"
	"github.com/alle-bartoli/agentmem/internal/memory"
	redisclient "github.com/alle-bartoli/agentmem/internal/redis"
	"github.com/mark3labs/mcp-go/server"
)

var version = "0.0.0"

func main() {
	configPath := flag.String("config", config.DefaultConfigPath(), "Path to config file")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("agentmem", version)
		os.Exit(0)
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Redis client
	rc, err := redisclient.NewClient(cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to create Redis client: %v", err)
	}
	defer rc.Close()

	if pingErr := rc.Ping(ctx); pingErr != nil {
		log.Fatalf("Redis connection failed: %v", pingErr)
	}

	// Ensure RediSearch index
	if idxErr := redisclient.EnsureIndex(ctx, rc, cfg.Embedding.Dimension); idxErr != nil {
		log.Fatalf("Failed to ensure search index: %v", idxErr)
	}

	// Initialize embedder
	emb, err := embedding.NewEmbedder(cfg.Embedding)
	if err != nil {
		log.Fatalf("Failed to create embedder: %v", err)
	}

	// Initialize compressor
	comp, err := compressor.NewCompressor(cfg.Compressor)
	if err != nil {
		log.Fatalf("Failed to create compressor: %v", err)
	}

	// Initialize memory store
	store := memory.NewStore(rc.RDB(), emb, cfg.Memory)

	// Create and start MCP server
	s := mcpserver.NewServer(store, comp, cfg)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		cancel()
		rc.Close()
		os.Exit(0)
	}()

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}
