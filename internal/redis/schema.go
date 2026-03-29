package redis

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const IndexName = "idx:memories"

// EnsureIndex creates the RediSearch index if it doesn't exist.
// If the index exists with a different vector dimension, it drops and recreates it.
func EnsureIndex(ctx context.Context, c *Client, dimension int) error {
	currentDim, err := CheckIndexDimension(ctx, c)
	if err == nil {
		if currentDim == dimension {
			return nil // Index exists with correct dimension
		}
		// Dimension mismatch, drop and recreate
		if err := DropIndex(ctx, c); err != nil {
			return fmt.Errorf("drop index for recreation: %w", err)
		}
	}

	return createIndex(ctx, c, dimension)
}

func createIndex(ctx context.Context, c *Client, dimension int) error {
	args := []any{
		IndexName, "ON", "HASH", "PREFIX", "1", "mem:",
		"SCHEMA",
		"content", "TEXT", "WEIGHT", "1.0",
		"type", "TAG",
		"project", "TAG",
		"tags", "TAG", "SEPARATOR", ",",
		"importance", "NUMERIC", "SORTABLE",
		"created_at", "NUMERIC", "SORTABLE",
		"last_accessed", "NUMERIC", "SORTABLE",
		"access_count", "NUMERIC", "SORTABLE",
		"embedding", "VECTOR", "HNSW", "6",
		"TYPE", "FLOAT32",
		"DIM", dimension,
		"DISTANCE_METRIC", "COSINE",
	}

	if err := c.rdb.Do(ctx, append([]any{"FT.CREATE"}, args...)...).Err(); err != nil {
		return fmt.Errorf("FT.CREATE: %w", err)
	}
	return nil
}

// DropIndex removes the RediSearch index without deleting the underlying data.
func DropIndex(ctx context.Context, c *Client) error {
	if err := c.rdb.Do(ctx, "FT.DROPINDEX", IndexName).Err(); err != nil {
		return fmt.Errorf("FT.DROPINDEX: %w", err)
	}
	return nil
}

// CheckIndexDimension reads the current vector dimension from the index info.
// Returns an error if the index does not exist.
func CheckIndexDimension(ctx context.Context, c *Client) (int, error) {
	res, err := c.rdb.Do(ctx, "FT.INFO", IndexName).Result()
	if err != nil {
		return 0, fmt.Errorf("FT.INFO: %w", err)
	}

	return parseDimensionFromInfo(res)
}

// parseDimensionFromInfo extracts DIM from FT.INFO result by walking the typed structure.
func parseDimensionFromInfo(info any) (int, error) {
	return parseDimensionNested(info)
}

// parseDimensionNested handles the nested structure from go-redis FT.INFO.
// go-redis may return map[interface{}]interface{} or []any depending on version.
func parseDimensionNested(v any) (int, error) {
	switch val := v.(type) {
	case map[any]any:
		for k, mv := range val {
			if str, ok := k.(string); ok && strings.EqualFold(str, "dim") {
				return toInt(mv)
			}
			if dim, err := parseDimensionNested(mv); err == nil {
				return dim, nil
			}
		}
	case map[string]any:
		for k, mv := range val {
			if strings.EqualFold(k, "dim") {
				return toInt(mv)
			}
			if dim, err := parseDimensionNested(mv); err == nil {
				return dim, nil
			}
		}
	case []any:
		for i, item := range val {
			if str, ok := item.(string); ok && strings.EqualFold(str, "DIM") {
				if i+1 < len(val) {
					return toInt(val[i+1])
				}
			}
			if dim, err := parseDimensionNested(item); err == nil {
				return dim, nil
			}
		}
	}
	return 0, fmt.Errorf("DIM not found in index info")
}

func toInt(v any) (int, error) {
	switch val := v.(type) {
	case int64:
		return int(val), nil
	case string:
		return strconv.Atoi(val)
	default:
		return 0, fmt.Errorf("unexpected type %T for dimension", v)
	}
}
