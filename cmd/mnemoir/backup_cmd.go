package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/alle-bartoli/mnemoir/internal/config"
	"github.com/alle-bartoli/mnemoir/internal/dbops"
	redisclient "github.com/alle-bartoli/mnemoir/internal/redis"
)

func runBackup(args []string) {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	configPath := fs.String("config", config.DefaultConfigPath(), "Path to config file")
	output := fs.String("output", "", "Output file path (default: stdout)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	_, rc := mustConnectRedis(*configPath)
	defer rc.Close()

	fmt.Fprintln(os.Stderr, "WARNING: JSON backup is not atomic. Concurrent writes may produce an inconsistent snapshot.")
	fmt.Fprintln(os.Stderr, "         For a consistent backup use 'make backup' (BGSAVE + data dir copy).")

	w := os.Stdout
	if *output != "" {
		f, err := os.OpenFile(*output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open output file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		w = f
	}

	if err := dbops.Dump(context.Background(), rc.RDB(), w); err != nil {
		fmt.Fprintf(os.Stderr, "backup failed: %v\n", err)
		os.Exit(1)
	}

	if *output != "" {
		fmt.Fprintf(os.Stderr, "backup written to %s\n", *output)
	}
}

func runRestore(args []string) {
	fs := flag.NewFlagSet("restore", flag.ExitOnError)
	configPath := fs.String("config", config.DefaultConfigPath(), "Path to config file")
	input := fs.String("input", "", "Input file path (default: stdin)")
	flush := fs.Bool("flush", false, "Flush the entire Redis database before restoring (destructive)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cfg, rc := mustConnectRedis(*configPath)
	defer rc.Close()

	// Read the full input into memory so we can peek the header before consuming it.
	var raw []byte
	if *input != "" {
		f, err := os.Open(*input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open input file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		raw, err = io.ReadAll(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read input file: %v\n", err)
			os.Exit(1)
		}
	} else {
		var err error
		raw, err = io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
			os.Exit(1)
		}
	}

	// Peek at snapshot header to get the embedding dimension before touching Redis.
	var header dbops.SnapshotHeader
	if err := json.Unmarshal(raw, &header); err != nil {
		fmt.Fprintf(os.Stderr, "decode snapshot header: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	if *flush {
		fmt.Fprintln(os.Stderr, "WARNING: flushing entire Redis database before restore...")
		if err := rc.RDB().FlushDB(ctx).Err(); err != nil {
			fmt.Fprintf(os.Stderr, "flushdb: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "database flushed.")
	}

	// Use snapshot dimension if available, fall back to config.
	dim := header.EmbeddingDim
	if dim == 0 {
		dim = cfg.Embedding.Dimension
	}
	if err := redisclient.EnsureIndex(ctx, rc, dim); err != nil {
		fmt.Fprintf(os.Stderr, "ensure index (dim=%d): %v\n", dim, err)
		os.Exit(1)
	}

	res, err := dbops.Restore(ctx, rc.RDB(), bytes.NewReader(raw))
	if err != nil {
		fmt.Fprintf(os.Stderr, "restore failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "restored: %d memories, %d sessions, %d projects, %d tag scores\n",
		res.Memories, res.Sessions, res.Projects, res.TagScores)

	if !res.IndexingComplete {
		fmt.Fprintln(os.Stderr, "WARNING: RediSearch indexing timed out. Vector search may return incomplete results until indexing finishes.")
	} else {
		fmt.Fprintln(os.Stderr, "RediSearch indexing complete.")
	}
}

// mustConnectRedis loads config and opens a Redis connection, exiting on error.
func mustConnectRedis(configPath string) (*config.Config, *redisclient.Client) {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	rc, err := redisclient.NewClient(cfg.Redis)
	if err != nil {
		fmt.Fprintf(os.Stderr, "redis client: %v\n", err)
		os.Exit(1)
	}

	if err := rc.Ping(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "redis ping: %v\n", err)
		os.Exit(1)
	}

	return cfg, rc
}
