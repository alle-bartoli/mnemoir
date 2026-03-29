// Package redis provides Redis client and RediSearch index management.
package redis

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/alle-bartoli/mnemoir/internal/config"
	"github.com/redis/go-redis/v9"
)

// Client wraps a Redis connection with health check support.
type Client struct {
	rdb *redis.Client
}

// NewClient creates a new Redis client from config.
func NewClient(cfg config.RedisConfig) (*Client, error) {
	opts := &redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: 10,
	}
	// Security: optional TLS with minimum TLS 1.2 for encrypted connections
	if cfg.TLS {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	rdb := redis.NewClient(opts)

	return &Client{rdb: rdb}, nil
}

// Ping verifies the Redis connection is alive.
func (c *Client) Ping(ctx context.Context) error {
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}

// Close shuts down the Redis connection.
func (c *Client) Close() error {
	return c.rdb.Close()
}

// RDB exposes the underlying redis.Client for direct commands.
func (c *Client) RDB() *redis.Client {
	return c.rdb
}

// StartHealthServer starts a sideband HTTP server on the given address (e.g. ":9090")
// exposing /healthz for readiness probes. Returns the listener for shutdown.
func (c *Client) StartHealthServer(addr string) (net.Listener, error) {
	if addr == "" {
		return nil, nil // disabled
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		status := "ok"
		code := http.StatusOK
		if err := c.rdb.Ping(ctx).Err(); err != nil {
			status = "redis unreachable"
			code = http.StatusServiceUnavailable
			slog.Warn("health check failed", "error", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"status": status})
	})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("health server listen: %w", err)
	}

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("health server error", "error", err)
		}
	}()

	slog.Info("Health endpoint started", "addr", ln.Addr().String())
	return ln, nil
}
