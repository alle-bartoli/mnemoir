// Package redis provides Redis client and RediSearch index management.
package redis

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/alle-bartoli/agentmem/internal/config"
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
