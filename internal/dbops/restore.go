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

const reindexTimeout = 2 * time.Minute
const reindexPollInterval = 500 * time.Millisecond

// @dev Restore decodes a JSON snapshot from r and writes all entities back into Redis.
// Validates snapshot version and embedding dimension before writing any data, so
// mismatches fail early with no partial writes.
//
// Prerequisites (caller's responsibility):
//   - RediSearch index must already exist with the correct dimension.
//     Call redis.EnsureIndex(ctx, rc, snap.EmbeddingDim) before calling Restore.
//     Use dbops.PeekHeader to read snap.EmbeddingDim without consuming r.
//   - For a clean restore (no stale data), flush the DB and recreate the index first.
//
// Restore blocks until RediSearch finishes background indexing (up to 2 minutes).
// RestoreResult.IndexingComplete is false if the timeout was reached.
func Restore(ctx context.Context, rdb *goredis.Client, r io.Reader) (*RestoreResult, error) {
	var snap Snapshot
	if err := json.NewDecoder(r).Decode(&snap); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	if snap.Version != snapshotVersion {
		return nil, fmt.Errorf("unsupported snapshot version %d (expected %d)", snap.Version, snapshotVersion)
	}

	// Validate embedding dimension before writing any data.
	if snap.EmbeddingDim > 0 {
		currentDim, err := redis.GetIndexDimension(ctx, rdb)
		if err != nil {
			return nil, fmt.Errorf("read index dimension: %w", err)
		}
		if currentDim != snap.EmbeddingDim {
			return nil, fmt.Errorf(
				"embedding dimension mismatch: index has %d dims, snapshot has %d dims; "+
					"recreate the index with the correct dimension before restoring",
				currentDim, snap.EmbeddingDim,
			)
		}
	}

	res := &RestoreResult{}

	if err := restoreMemories(ctx, rdb, snap.Memories, res); err != nil {
		return res, fmt.Errorf("restore memories: %w", err)
	}
	if err := restoreSessions(ctx, rdb, snap.Sessions, res); err != nil {
		return res, fmt.Errorf("restore sessions: %w", err)
	}
	if err := restoreProjects(ctx, rdb, snap.Projects, res); err != nil {
		return res, fmt.Errorf("restore projects: %w", err)
	}
	if err := restoreProjectSessions(ctx, rdb, snap.ProjectSessions, res); err != nil {
		return res, fmt.Errorf("restore project sessions: %w", err)
	}
	if err := restoreTagFrequencies(ctx, rdb, snap.TagFrequencies, res); err != nil {
		return res, fmt.Errorf("restore tag frequencies: %w", err)
	}

	res.IndexingComplete = waitForReindex(ctx, rdb, res.Memories)

	return res, nil
}

// PRIVATE

// @dev restoreMemories writes all memory hashes in a single pipeline. Embedding is
// decoded from base64 back to raw bytes and passed directly to HSet, bypassing the
// embedder. This preserves the original vector without re-inference.
func restoreMemories(ctx context.Context, rdb *goredis.Client, records []MemoryRecord, res *RestoreResult) error {
	if len(records) == 0 {
		return nil
	}
	pipe := rdb.Pipeline()
	for _, rec := range records {
		embBytes, err := base64.StdEncoding.DecodeString(rec.Embedding)
		if err != nil {
			return fmt.Errorf("decode embedding for %s: %w", rec.ID, err)
		}
		key := redis.KeyPrefixMemory + rec.ID
		fields := map[string]any{
			memory.FieldContent:      rec.Content,
			memory.FieldType:         rec.Type,
			memory.FieldProject:      rec.Project,
			memory.FieldTags:         rec.Tags,
			memory.FieldImportance:   rec.Importance,
			memory.FieldSessionID:    rec.SessionID,
			memory.FieldCreatedAt:    rec.CreatedAt,
			memory.FieldLastAccessed: rec.LastAccessed,
			memory.FieldAccessCount:  rec.AccessCount,
			memory.FieldEmbedding:    embBytes,
		}
		pipe.HSet(ctx, key, fields)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("pipeline HSet memories: %w", err)
	}
	res.Memories = len(records)
	return nil
}

// @dev restoreSessions writes all session hashes in a single pipeline.
func restoreSessions(ctx context.Context, rdb *goredis.Client, records []SessionRecord, res *RestoreResult) error {
	if len(records) == 0 {
		return nil
	}
	pipe := rdb.Pipeline()
	for _, rec := range records {
		key := redis.KeyPrefixSession + rec.ID
		fields := map[string]any{
			memory.FieldProject:     rec.Project,
			memory.FieldStartedAt:   rec.StartedAt,
			memory.FieldEndedAt:     rec.EndedAt,
			memory.FieldSummary:     rec.Summary,
			memory.FieldMemoryCount: rec.MemoryCount,
		}
		pipe.HSet(ctx, key, fields)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("pipeline HSet sessions: %w", err)
	}
	res.Sessions = len(records)
	return nil
}

func restoreProjects(ctx context.Context, rdb *goredis.Client, projects []string, res *RestoreResult) error {
	if len(projects) == 0 {
		return nil
	}
	members := make([]any, len(projects))
	for i, p := range projects {
		members[i] = p
	}
	if err := rdb.SAdd(ctx, redis.KeyProjects, members...).Err(); err != nil {
		return fmt.Errorf("sadd %s: %w", redis.KeyProjects, err)
	}
	res.Projects = len(projects)
	return nil
}

// @dev restoreProjectSessions rebuilds all project_sessions:{project} ZSETs in a
// single pipeline. Scores are Unix timestamps (set at session creation), so ordering
// is preserved exactly as it was at backup time.
func restoreProjectSessions(ctx context.Context, rdb *goredis.Client, projectSessions map[string][]ScoredEntry, res *RestoreResult) error {
	if len(projectSessions) == 0 {
		return nil
	}
	pipe := rdb.Pipeline()
	for project, entries := range projectSessions {
		if len(entries) == 0 {
			continue
		}
		key := redis.KeyPrefixProjectSessions + project
		members := make([]goredis.Z, len(entries))
		for i, e := range entries {
			members[i] = goredis.Z{Score: e.Score, Member: e.Member}
		}
		pipe.ZAdd(ctx, key, members...)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("pipeline ZAdd project sessions: %w", err)
	}
	return nil
}

func restoreTagFrequencies(ctx context.Context, rdb *goredis.Client, entries []ScoredEntry, res *RestoreResult) error {
	if len(entries) == 0 {
		return nil
	}
	members := make([]goredis.Z, len(entries))
	for i, e := range entries {
		members[i] = goredis.Z{Score: e.Score, Member: e.Member}
	}
	if err := rdb.ZAdd(ctx, redis.KeyTagFrequency, members...).Err(); err != nil {
		return fmt.Errorf("zadd %s: %w", redis.KeyTagFrequency, err)
	}
	res.TagScores = len(entries)
	return nil
}

// @dev waitForReindex polls FT.INFO until RediSearch reports indexing==0.
// The initial 300ms sleep is required because RediSearch detects new hashes via
// keyspace notifications (async): without the delay the first poll fires before
// the indexing pipeline starts, producing a false-positive indexing==0.
// Returns true if indexing completed within the timeout, false on timeout.
func waitForReindex(ctx context.Context, rdb *goredis.Client, minDocs int) bool {
	if minDocs == 0 {
		return true
	}
	// Allow RediSearch time to detect the newly written hashes before polling.
	time.Sleep(300 * time.Millisecond)

	deadline := time.Now().Add(reindexTimeout)
	for time.Now().Before(deadline) {
		done, err := redis.IsIndexingComplete(ctx, rdb)
		if err == nil && done {
			return true
		}
		time.Sleep(reindexPollInterval)
	}
	return false
}
