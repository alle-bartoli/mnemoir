package compressor_test

import (
	"context"
	"testing"

	"github.com/alle-bartoli/mnemoir/internal/compressor"
	"github.com/redis/go-redis/v9"
)

const tagFrequencyKey = "tags:frequency"

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	t.Cleanup(func() {
		rdb.Del(context.Background(), tagFrequencyKey)
		rdb.Close()
	})
	return rdb
}

func TestLocalCompressor(t *testing.T) {
	t.Run("SeedTagsOnFirstRun", func(t *testing.T) {
		rdb := newTestRedis(t)
		ctx := context.Background()

		// Ensure clean state
		rdb.Del(ctx, tagFrequencyKey)

		_, err := compressor.NewLocalCompressor(rdb)
		if err != nil {
			t.Fatalf("NewLocalCompressor: %v", err)
		}

		count, err := rdb.ZCard(ctx, tagFrequencyKey).Result()
		if err != nil {
			t.Fatalf("ZCard: %v", err)
		}
		if count == 0 {
			t.Fatal("expected seeded tags, got 0")
		}

		// Verify a known seed keyword exists with score 0
		score, err := rdb.ZScore(ctx, tagFrequencyKey, "redis").Result()
		if err != nil {
			t.Fatalf("ZScore redis: %v", err)
		}
		if score != 0 {
			t.Errorf("expected seed score 0, got %f", score)
		}
	})

	t.Run("SeedIsIdempotent", func(t *testing.T) {
		rdb := newTestRedis(t)
		ctx := context.Background()
		rdb.Del(ctx, tagFrequencyKey)

		// Create twice
		_, _ = compressor.NewLocalCompressor(rdb)
		// Manually bump a tag score
		rdb.ZIncrBy(ctx, tagFrequencyKey, 5, "redis")

		_, _ = compressor.NewLocalCompressor(rdb)

		// Score should not be reset to 0
		score, _ := rdb.ZScore(ctx, tagFrequencyKey, "redis").Result()
		if score != 5 {
			t.Errorf("expected score 5 after second seed, got %f", score)
		}
	})

	t.Run("IncrementTags", func(t *testing.T) {
		rdb := newTestRedis(t)
		ctx := context.Background()
		rdb.Del(ctx, tagFrequencyKey)

		compressor.IncrementTags(ctx, rdb, "redis,docker,newtech")

		score, _ := rdb.ZScore(ctx, tagFrequencyKey, "redis").Result()
		if score != 1 {
			t.Errorf("expected redis score 1, got %f", score)
		}
		score, _ = rdb.ZScore(ctx, tagFrequencyKey, "newtech").Result()
		if score != 1 {
			t.Errorf("expected newtech score 1, got %f", score)
		}

		// Increment again
		compressor.IncrementTags(ctx, rdb, "redis")
		score, _ = rdb.ZScore(ctx, tagFrequencyKey, "redis").Result()
		if score != 2 {
			t.Errorf("expected redis score 2 after second increment, got %f", score)
		}
	})

	t.Run("LearnedTagsUsedInCompress", func(t *testing.T) {
		rdb := newTestRedis(t)
		ctx := context.Background()
		rdb.Del(ctx, tagFrequencyKey)

		// Add a custom tag that wouldn't be in defaults
		rdb.ZAdd(ctx, tagFrequencyKey, redis.Z{Score: 10, Member: "mnemoir"})

		comp, err := compressor.NewLocalCompressor(rdb)
		if err != nil {
			t.Fatalf("NewLocalCompressor: %v", err)
		}

		result, err := comp.Compress(ctx, "The mnemoir server handles memory persistence for agents.")
		if err != nil {
			t.Fatalf("Compress: %v", err)
		}

		// Check that the learned tag "mnemoir" was extracted
		found := false
		all := append(result.Facts, append(result.Concepts, result.Narratives...)...)
		for _, m := range all {
			if containsTag(m.Tags, "mnemoir") {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected learned tag 'mnemoir' to be extracted from observations")
		}
	})

	t.Run("FallbackWithNilRedis", func(t *testing.T) {
		comp, err := compressor.NewLocalCompressor(nil)
		if err != nil {
			t.Fatalf("NewLocalCompressor with nil: %v", err)
		}

		result, err := comp.Compress(context.Background(), "Redis runs on port 6379 with Docker containers.")
		if err != nil {
			t.Fatalf("Compress: %v", err)
		}

		// Should still work using default keywords
		all := append(result.Facts, append(result.Concepts, result.Narratives...)...)
		if len(all) == 0 {
			t.Fatal("expected at least one extracted memory")
		}

		found := false
		for _, m := range all {
			if containsTag(m.Tags, "redis") || containsTag(m.Tags, "docker") {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected default tags to be extracted in fallback mode")
		}
	})
}

func containsTag(tags, target string) bool {
	for _, t := range splitTags(tags) {
		if t == target {
			return true
		}
	}
	return false
}

func splitTags(tags string) []string {
	var result []string
	for _, t := range splitComma(tags) {
		t = trimSpace(t)
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}

func splitComma(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && s[i] == ' ' {
		i++
	}
	for j > i && s[j-1] == ' ' {
		j--
	}
	return s[i:j]
}
