package dbops

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/alle-bartoli/mnemoir/internal/memory"
	"github.com/alle-bartoli/mnemoir/internal/redis"
	goredis "github.com/redis/go-redis/v9"
)

// @dev Dump builds a Snapshot from live Redis data and encodes it as indented JSON to w.
// Order of operations: memories -> sessions -> projects -> project_sessions -> tag_frequencies.
// project_sessions depends on projects being populated first (iterates snap.Projects).
// EmbeddingDim is read from the live RediSearch index so Restore can validate it.
// WARNING: Dump is not atomic. Concurrent writes during Dump may produce an inconsistent
// snapshot. For a consistent backup use 'make backup' (BGSAVE + data dir copy).
func Dump(ctx context.Context, rdb *goredis.Client, w io.Writer) error {
	dim, _ := redis.GetIndexDimension(ctx, rdb)
	snap := &Snapshot{
		Version:         snapshotVersion,
		CreatedAt:       time.Now().Unix(),
		EmbeddingDim:    dim,
		ProjectSessions: make(map[string][]ScoredEntry),
	}

	if err := dumpMemories(ctx, rdb, snap); err != nil {
		return fmt.Errorf("dump memories: %w", err)
	}
	if err := dumpSessions(ctx, rdb, snap); err != nil {
		return fmt.Errorf("dump sessions: %w", err)
	}
	if err := dumpProjects(ctx, rdb, snap); err != nil {
		return fmt.Errorf("dump projects: %w", err)
	}
	if err := dumpProjectSessions(ctx, rdb, snap); err != nil {
		return fmt.Errorf("dump project sessions: %w", err)
	}
	if err := dumpTagFrequencies(ctx, rdb, snap); err != nil {
		return fmt.Errorf("dump tag frequencies: %w", err)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(snap)
}

// PRIVATE

// @dev dumpMemories scans all mem:{ULID} keys and fetches their hashes in a single
// pipeline round-trip. Embedding bytes are base64-encoded for JSON transport.
func dumpMemories(ctx context.Context, rdb *goredis.Client, snap *Snapshot) error {
	keys, err := scanKeys(ctx, rdb, redis.KeyPrefixMemory+"*")
	if err != nil {
		return err
	}

	pipe := rdb.Pipeline()
	cmds := make([]*goredis.MapStringStringCmd, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.HGetAll(ctx, key)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("pipeline HGetAll: %w", err)
	}

	snap.Memories = make([]MemoryRecord, 0, len(keys))
	for i, cmd := range cmds {
		vals, err := cmd.Result()
		if err != nil || len(vals) == 0 {
			continue
		}
		id := stripPrefix(keys[i], redis.KeyPrefixMemory)
		rec := MemoryRecord{
			ID:           id,
			Content:      vals[memory.FieldContent],
			Type:         vals[memory.FieldType],
			Project:      vals[memory.FieldProject],
			Tags:         vals[memory.FieldTags],
			Importance:   vals[memory.FieldImportance],
			SessionID:    vals[memory.FieldSessionID],
			CreatedAt:    vals[memory.FieldCreatedAt],
			LastAccessed: vals[memory.FieldLastAccessed],
			AccessCount:  vals[memory.FieldAccessCount],
			Embedding:    base64.StdEncoding.EncodeToString([]byte(vals[memory.FieldEmbedding])),
		}
		snap.Memories = append(snap.Memories, rec)
	}
	return nil
}

// @dev dumpSessions scans all session:{ULID} keys and fetches their hashes in a single
// pipeline round-trip.
func dumpSessions(ctx context.Context, rdb *goredis.Client, snap *Snapshot) error {
	keys, err := scanKeys(ctx, rdb, redis.KeyPrefixSession+"*")
	if err != nil {
		return err
	}

	pipe := rdb.Pipeline()
	cmds := make([]*goredis.MapStringStringCmd, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.HGetAll(ctx, key)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("pipeline HGetAll: %w", err)
	}

	snap.Sessions = make([]SessionRecord, 0, len(keys))
	for i, cmd := range cmds {
		vals, err := cmd.Result()
		if err != nil || len(vals) == 0 {
			continue
		}
		id := stripPrefix(keys[i], redis.KeyPrefixSession)
		rec := SessionRecord{
			ID:          id,
			Project:     vals[memory.FieldProject],
			StartedAt:   vals[memory.FieldStartedAt],
			EndedAt:     vals[memory.FieldEndedAt],
			Summary:     vals[memory.FieldSummary],
			MemoryCount: vals[memory.FieldMemoryCount],
		}
		snap.Sessions = append(snap.Sessions, rec)
	}
	return nil
}

func dumpProjects(ctx context.Context, rdb *goredis.Client, snap *Snapshot) error {
	members, err := rdb.SMembers(ctx, redis.KeyProjects).Result()
	if err != nil {
		return fmt.Errorf("smembers %s: %w", redis.KeyProjects, err)
	}
	snap.Projects = members
	return nil
}

// @dev dumpProjectSessions iterates snap.Projects (already populated) to fetch
// each project_sessions:{project} ZSET in individual calls. No pipeline here because
// the number of projects is typically small; a pipeline would add complexity without
// measurable gain.
func dumpProjectSessions(ctx context.Context, rdb *goredis.Client, snap *Snapshot) error {
	for _, project := range snap.Projects {
		key := redis.KeyPrefixProjectSessions + project
		entries, err := rdb.ZRangeWithScores(ctx, key, 0, -1).Result()
		if err != nil {
			return fmt.Errorf("zrange %s: %w", key, err)
		}
		scored := make([]ScoredEntry, 0, len(entries))
		for _, e := range entries {
			scored = append(scored, ScoredEntry{Member: e.Member.(string), Score: e.Score})
		}
		snap.ProjectSessions[project] = scored
	}
	return nil
}

func dumpTagFrequencies(ctx context.Context, rdb *goredis.Client, snap *Snapshot) error {
	entries, err := rdb.ZRangeWithScores(ctx, redis.KeyTagFrequency, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("zrange %s: %w", redis.KeyTagFrequency, err)
	}
	snap.TagFrequencies = make([]ScoredEntry, 0, len(entries))
	for _, e := range entries {
		snap.TagFrequencies = append(snap.TagFrequencies, ScoredEntry{Member: e.Member.(string), Score: e.Score})
	}
	return nil
}

// @dev scanKeys collects all Redis keys matching pattern using cursor-based SCAN.
// Avoids the 10k result cap of KEYS and is safe on large keyspaces.
// Batch size 200 balances round-trips against memory pressure.
func scanKeys(ctx context.Context, rdb *goredis.Client, pattern string) ([]string, error) {
	var keys []string
	var cursor uint64
	for {
		batch, next, err := rdb.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", pattern, err)
		}
		keys = append(keys, batch...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return keys, nil
}

func stripPrefix(key, prefix string) string {
	if len(key) > len(prefix) {
		return key[len(prefix):]
	}
	return key
}
