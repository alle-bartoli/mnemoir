package memory

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alle-bartoli/mnemoir/internal/config"
	"github.com/alle-bartoli/mnemoir/internal/embedding"
	"github.com/alle-bartoli/mnemoir/internal/redis"
	goredis "github.com/redis/go-redis/v9"
)

// Security: allowlist regex prevents RediSearch TAG injection
var validTagValue = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)

// ValidateTagValue rejects values containing RediSearch special characters.
// Called at the handler boundary before any query is built.
func ValidateTagValue(s string) error {
	if !validTagValue.MatchString(s) {
		return fmt.Errorf("invalid value %q: only alphanumeric, underscore, hyphen, dot allowed", s)
	}
	return nil
}

// Store handles memory CRUD operations backed by Redis.
type Store struct {
	rdb      *goredis.Client
	embedder embedding.IEmbedder
	memCfg   config.MemoryConfig
}

// NewStore creates a new memory store.
func NewStore(rdb *goredis.Client, embedder embedding.IEmbedder, memCfg config.MemoryConfig) *Store {
	return &Store{rdb: rdb, embedder: embedder, memCfg: memCfg}
}

// Save persists a memory to Redis with its embedding.
func (s *Store) Save(ctx context.Context, mem *Memory) error {
	emb, err := s.embedder.Embed(ctx, mem.Content)
	if err != nil {
		return fmt.Errorf("generate embedding: %w", err)
	}
	mem.Embedding = emb

	key := redis.KeyPrefixMemory + mem.ID
	fields := map[string]any{
		"content":       mem.Content,
		"type":          string(mem.Type),
		"project":       mem.Project,
		"tags":          mem.Tags,
		"importance":    mem.Importance,
		"session_id":    mem.SessionID,
		"created_at":    mem.CreatedAt,
		"last_accessed": mem.LastAccessed,
		"access_count":  mem.AccessCount,
		"embedding":     float32ToBytes(emb),
	}

	pipe := s.rdb.Pipeline()
	pipe.HSet(ctx, key, fields)
	pipe.SAdd(ctx, redis.KeyProjects, mem.Project)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("save memory %s: %w", mem.ID, err)
	}

	return nil
}

// Get retrieves a memory by ID.
func (s *Store) Get(ctx context.Context, id string) (*Memory, error) {
	key := redis.KeyPrefixMemory + id
	vals, err := s.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("get memory %s: %w", id, err)
	}
	if len(vals) == 0 {
		return nil, fmt.Errorf("memory %s not found", id)
	}

	return hashToMemory(id, vals)
}

// Delete removes a single memory by ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	key := redis.KeyPrefixMemory + id
	deleted, err := s.rdb.Del(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("delete memory %s: %w", id, err)
	}
	if deleted == 0 {
		return fmt.Errorf("memory %s not found", id)
	}
	return nil
}

// DeleteByFilter removes memories matching project and/or age criteria.
// Processes in batches of 1000 to avoid the 10k FT.SEARCH cap.
func (s *Store) DeleteByFilter(ctx context.Context, project string, olderThan time.Duration) (int, error) {
	query := buildFilterQuery(project, olderThan)
	totalDeleted := 0

	const maxIterations = 100 // Safety: prevent infinite loop if index is stale
	for iteration := 0; iteration < maxIterations; iteration++ {
		args := []any{"FT.SEARCH", redis.IndexName, query, "NOCONTENT", "LIMIT", 0, 1000}
		res, err := s.rdb.Do(ctx, args...).Result()
		if err != nil {
			return totalDeleted, fmt.Errorf("search for deletion: %w", err)
		}

		ids := extractIDsFromSearch(res)
		if len(ids) == 0 {
			break
		}

		pipe := s.rdb.Pipeline()
		for _, id := range ids {
			pipe.Del(ctx, id)
		}
		cmds, err := pipe.Exec(ctx)
		if err != nil {
			return totalDeleted, fmt.Errorf("bulk delete: %w", err)
		}

		deletedThisBatch := 0
		for _, cmd := range cmds {
			if cmd.(*goredis.IntCmd).Val() > 0 {
				deletedThisBatch++
			}
		}
		totalDeleted += deletedThisBatch

		// Safety: if no keys were actually deleted, the index is stale - break to avoid spinning
		if deletedThisBatch == 0 {
			break
		}
	}

	return totalDeleted, nil
}

// updateAccessScript atomically reads, computes, and writes access state in a single
// Redis round-trip. Eliminates the read-modify-write race in the previous pipeline approach.
//
// KEYS[1] = mem:{id}
// ARGV[1] = decay_factor, ARGV[2] = decay_interval_seconds, ARGV[3] = boost_factor,
// ARGV[4] = boost_cap, ARGV[5] = now_unix
var updateAccessScript = goredis.NewScript(`
local key = KEYS[1]
local decay_factor = tonumber(ARGV[1])
local decay_interval = tonumber(ARGV[2])
local boost_factor = tonumber(ARGV[3])
local boost_cap = tonumber(ARGV[4])
local now = tonumber(ARGV[5])

local importance = tonumber(redis.call('HGET', key, 'importance') or '5')
local access_count = tonumber(redis.call('HGET', key, 'access_count') or '0')
local last_accessed = tonumber(redis.call('HGET', key, 'last_accessed') or tostring(now))

access_count = access_count + 1

-- Spaced repetition: effective = base * decay^intervals + min(cap, count * factor)
local intervals = 0
if decay_interval > 0 then
    intervals = (now - last_accessed) / decay_interval
end
local decayed = importance * math.pow(decay_factor, intervals)
local boost = math.min(boost_cap, access_count * boost_factor)
local effective = decayed + boost
-- Clamp to [1, 10]
if effective < 1 then effective = 1 end
if effective > 10 then effective = 10 end
local new_importance = math.floor(effective + 0.5)

redis.call('HSET', key, 'access_count', access_count, 'last_accessed', now, 'importance', new_importance)
return new_importance
`)

// UpdateAccess atomically increments access_count, updates last_accessed, and
// recalculates importance using spaced repetition. Uses a Lua script to prevent
// read-modify-write races between concurrent search requests.
func (s *Store) UpdateAccess(ctx context.Context, id string) error {
	key := redis.KeyPrefixMemory + id
	decayInterval, _ := s.memCfg.ParsedDecayInterval()

	_, err := updateAccessScript.Run(ctx, s.rdb, []string{key},
		s.memCfg.DecayFactor,
		decayInterval.Seconds(),
		s.memCfg.AccessBoostFactor,
		s.memCfg.AccessBoostCap,
		time.Now().Unix(),
	).Result()
	if err != nil {
		return fmt.Errorf("update access %s: %w", id, err)
	}
	return nil
}

// Update modifies specific fields of a memory. Recalculates embedding if content changes.
func (s *Store) Update(ctx context.Context, id string, fields map[string]any) error {
	key := redis.KeyPrefixMemory + id

	exists, err := s.rdb.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("check memory %s: %w", id, err)
	}
	if exists == 0 {
		return fmt.Errorf("memory %s not found", id)
	}

	if content, ok := fields["content"].(string); ok && content != "" {
		emb, err := s.embedder.Embed(ctx, content)
		if err != nil {
			return fmt.Errorf("regenerate embedding: %w", err)
		}
		fields["embedding"] = float32ToBytes(emb)
	}

	if err := s.rdb.HSet(ctx, key, fields).Err(); err != nil {
		return fmt.Errorf("update memory %s: %w", id, err)
	}
	return nil
}

// ListProjects returns all known project names.
func (s *Store) ListProjects(ctx context.Context) ([]string, error) {
	return s.rdb.SMembers(ctx, redis.KeyProjects).Result()
}

// CountByProject returns the number of memories for a project.
func (s *Store) CountByProject(ctx context.Context, project string) (int, error) {
	query := fmt.Sprintf("@project:{%s}", escapeTag(project))
	res, err := s.rdb.Do(ctx, "FT.SEARCH", redis.IndexName, query, "NOCONTENT", "LIMIT", 0, 0).Result()
	if err != nil {
		return 0, fmt.Errorf("count by project: %w", err)
	}
	return extractTotalFromSearch(res), nil
}

// CountAllByProject returns memory counts grouped by project in a single FT.AGGREGATE call.
// Replaces the N+1 pattern of calling CountByProject per project (1 round-trip vs N).
//
// The generated query:
//
//	FT.AGGREGATE idx:memories * GROUPBY 1 @project REDUCE COUNT 0 AS count
func (s *Store) CountAllByProject(ctx context.Context) (map[string]int, error) {
	res, err := s.rdb.Do(ctx,
		"FT.AGGREGATE", redis.IndexName, "*",
		"GROUPBY", "1", "@project",
		"REDUCE", "COUNT", "0", "AS", "count",
	).Result()
	if err != nil {
		return nil, fmt.Errorf("count all by project: %w", err)
	}

	counts := make(map[string]int)
	entries := getResultEntries(res)
	for _, entry := range entries {
		attrs := getExtraAttributes(entry)
		project := attrs["project"]
		count, _ := strconv.Atoi(attrs["count"])
		if project != "" {
			counts[project] = count
		}
	}
	return counts, nil
}

// GetStats returns aggregate statistics, optionally filtered by project.
// Uses FT.AGGREGATE for server-side computation (no 10k result cap).
func (s *Store) GetStats(ctx context.Context, project string) (*MemoryStats, error) {
	query := "*"
	if project != "" {
		query = fmt.Sprintf("@project:{%s}", escapeTag(project))
	}

	// Use FT.AGGREGATE for server-side stats: total, avg importance, min/max created_at per type
	res, err := s.rdb.Do(ctx,
		"FT.AGGREGATE", redis.IndexName, query,
		"GROUPBY", "1", "@type",
		"REDUCE", "COUNT", "0", "AS", "count",
		"REDUCE", "AVG", "1", "@importance", "AS", "avg_imp",
		"REDUCE", "MIN", "1", "@created_at", "AS", "oldest",
		"REDUCE", "MAX", "1", "@created_at", "AS", "newest",
	).Result()
	if err != nil {
		// Fallback to basic search count if FT.AGGREGATE is not available
		return s.getStatsLegacy(ctx, query)
	}

	return parseAggregateStats(res)
}

// getStatsLegacy is a fallback using FT.SEARCH with a capped result set.
func (s *Store) getStatsLegacy(ctx context.Context, query string) (*MemoryStats, error) {
	res, err := s.rdb.Do(ctx,
		"FT.SEARCH", redis.IndexName, query,
		"RETURN", "3", "type", "importance", "created_at",
		"LIMIT", 0, 10000,
	).Result()
	if err != nil {
		return nil, fmt.Errorf("stats query: %w", err)
	}
	return computeStats(res)
}

// parseAggregateStats extracts stats from FT.AGGREGATE GROUPBY response.
func parseAggregateStats(res any) (*MemoryStats, error) {
	stats := &MemoryStats{
		ByType: map[string]int{string(Fact): 0, string(Concept): 0, string(Narrative): 0},
	}

	entries := getResultEntries(res)
	var totalImportanceSum float64
	var totalCount int

	for _, entry := range entries {
		attrs := getExtraAttributes(entry)
		typeName := attrs["type"]
		count, _ := strconv.Atoi(attrs["count"])
		avgImp, _ := strconv.ParseFloat(attrs["avg_imp"], 64)
		oldest, _ := strconv.ParseInt(attrs["oldest"], 10, 64)
		newest, _ := strconv.ParseInt(attrs["newest"], 10, 64)

		if typeName != "" {
			stats.ByType[typeName] = count
		}
		totalCount += count
		totalImportanceSum += avgImp * float64(count)

		if oldest > 0 && (stats.OldestMemoryAt == 0 || oldest < stats.OldestMemoryAt) {
			stats.OldestMemoryAt = oldest
		}
		if newest > stats.NewestMemoryAt {
			stats.NewestMemoryAt = newest
		}
	}

	stats.Total = totalCount
	if totalCount > 0 {
		stats.AvgImportance = totalImportanceSum / float64(totalCount)
	}

	return stats, nil
}

// SaveSession persists a session hash to Redis and indexes it in a sorted set
// keyed by project for O(log N) latest-session lookups.
func (s *Store) SaveSession(ctx context.Context, sess *Session) error {
	key := redis.KeyPrefixSession + sess.ID
	fields := map[string]any{
		"project":      sess.Project,
		"started_at":   sess.StartedAt,
		"ended_at":     sess.EndedAt,
		"summary":      sess.Summary,
		"memory_count": sess.MemoryCount,
	}

	pipe := s.rdb.Pipeline()
	pipe.HSet(ctx, key, fields)
	// Index session by project with started_at as score for fast latest-session lookup
	pipe.ZAdd(ctx, redis.KeyPrefixProjectSessions+sess.Project, goredis.Z{
		Score:  float64(sess.StartedAt),
		Member: sess.ID,
	})
	// Register project so list_projects sees it even without stored memories
	pipe.SAdd(ctx, redis.KeyProjects, sess.Project)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("save session %s: %w", sess.ID, err)
	}
	return nil
}

// GetLastSession retrieves the most recent session for a project using the
// sorted set index (O(log N) instead of the previous O(N) SCAN approach).
func (s *Store) GetLastSession(ctx context.Context, project string) (*Session, error) {
	// Get the highest-scored (most recent) session ID from the sorted set
	ids, err := s.rdb.ZRevRange(ctx, redis.KeyPrefixProjectSessions+project, 0, 0).Result()
	if err != nil {
		return nil, fmt.Errorf("get last session: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil // No previous session
	}

	key := redis.KeyPrefixSession + ids[0]
	vals, err := s.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if len(vals) == 0 {
		return nil, nil // Session hash was deleted
	}

	return hashToSession(key, vals), nil
}

// GetTopMemories retrieves top memories by importance for a project.
func (s *Store) GetTopMemories(ctx context.Context, project string, limit int) ([]Memory, error) {
	query := fmt.Sprintf("@project:{%s}", escapeTag(project))
	res, err := s.rdb.Do(ctx,
		"FT.SEARCH", redis.IndexName, query,
		"SORTBY", "importance", "DESC",
		"LIMIT", 0, limit,
	).Result()
	if err != nil {
		return nil, fmt.Errorf("top memories: %w", err)
	}

	return extractMemoriesFromSearch(res)
}

// PRIVATE

// @dev float32ToBytes converts a float32 vector into a little-endian byte blob
// for Redis VECTOR fields. Each float32 (4 bytes, IEEE 754) is written sequentially,
// producing a buffer of len(v)*4 bytes that RediSearch uses directly for KNN search.
//
// The slice expression buf[i*4:] offsets into the buffer so each float32 lands
// in its own 4-byte slot:
//
// v = [0.5, 1.0, 0.75] -> buf = make([]byte, 3*4) = 12 bytes
//
// i=0: buf[0:]  -> writes bytes at positions 0,1,2,3
// i=1: buf[4:]  -> writes bytes at positions 4,5,6,7
// i=2: buf[8:]  -> writes bytes at positions 8,9,10,11
func float32ToBytes(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func hashToMemory(id string, vals map[string]string) (*Memory, error) {
	importance, _ := strconv.Atoi(vals["importance"])
	createdAt, _ := strconv.ParseInt(vals["created_at"], 10, 64)
	lastAccessed, _ := strconv.ParseInt(vals["last_accessed"], 10, 64)
	accessCount, _ := strconv.Atoi(vals["access_count"])

	return &Memory{
		ID:           id,
		Content:      vals["content"],
		Type:         MemoryType(vals["type"]),
		Project:      vals["project"],
		Tags:         vals["tags"],
		Importance:   importance,
		SessionID:    vals["session_id"],
		CreatedAt:    createdAt,
		LastAccessed: lastAccessed,
		AccessCount:  accessCount,
	}, nil
}

func hashToSession(key string, vals map[string]string) *Session {
	// Extract ID from key "session:ULID"
	id := strings.TrimPrefix(key, redis.KeyPrefixSession)

	startedAt, _ := strconv.ParseInt(vals["started_at"], 10, 64)
	endedAt, _ := strconv.ParseInt(vals["ended_at"], 10, 64)
	memCount, _ := strconv.Atoi(vals["memory_count"])

	return &Session{
		ID:          id,
		Project:     vals["project"],
		StartedAt:   startedAt,
		EndedAt:     endedAt,
		Summary:     vals["summary"],
		MemoryCount: memCount,
	}
}

// tagEscaper is a package-level replacer for RediSearch TAG field special characters.
// Created once and reused across all queries, avoiding per-call string allocations.
// The old replaceAll() built strings via byte-by-byte concatenation (O(n*m) allocations).
var tagEscaper = strings.NewReplacer(
	"-", "\\-",
	".", "\\.",
	"@", "\\@",
	" ", "\\ ",
)

func escapeTag(s string) string {
	return tagEscaper.Replace(s)
}

func buildFilterQuery(project string, olderThan time.Duration) string {
	parts := make([]string, 0, 2) // at most project + age filter
	if project != "" {
		parts = append(parts, fmt.Sprintf("@project:{%s}", escapeTag(project)))
	}
	if olderThan > 0 {
		cutoff := time.Now().Add(-olderThan).Unix()
		parts = append(parts, fmt.Sprintf("@created_at:[-inf %d]", cutoff))
	}
	if len(parts) == 0 {
		return "*"
	}
	return strings.Join(parts, " ")
}

// @dev extractIDsFromSearch extracts memory keys from a NOCONTENT FT.SEARCH response.
func extractIDsFromSearch(res any) []string {
	entries := getResultEntries(res)
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if id := getMapString(entry, "id"); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// @dev extractTotalFromSearch returns total_results from FT.SEARCH response.
func extractTotalFromSearch(res any) int {
	m, ok := res.(map[any]any)
	if !ok {
		return 0
	}
	if total, ok := m["total_results"].(int64); ok {
		return int(total)
	}
	return 0
}

// @dev extractMemoriesFromSearch parses FT.SEARCH results into Memory structs.
func extractMemoriesFromSearch(res any) ([]Memory, error) {
	entries := getResultEntries(res)
	if len(entries) == 0 {
		return nil, nil
	}

	memories := make([]Memory, 0, len(entries))
	for _, entry := range entries {
		id := stripMemPrefix(getMapString(entry, "id"))
		vals := getExtraAttributes(entry)

		mem, err := hashToMemory(id, vals)
		if err != nil {
			continue
		}
		memories = append(memories, *mem)
	}

	return memories, nil
}

// @dev computeStats aggregates statistics from FT.SEARCH results.
func computeStats(res any) (*MemoryStats, error) {
	m, ok := res.(map[any]any)
	if !ok {
		return &MemoryStats{ByType: map[string]int{}}, nil
	}

	total := 0
	if t, ok := m["total_results"].(int64); ok {
		total = int(t)
	}

	stats := &MemoryStats{
		Total:  total,
		ByType: map[string]int{string(Fact): 0, string(Concept): 0, string(Narrative): 0},
	}

	entries := getResultEntries(res)
	var sumImportance int
	var oldest, newest int64

	for _, entry := range entries {
		vals := getExtraAttributes(entry)

		if t := vals["type"]; t != "" {
			stats.ByType[t]++
		}
		if imp, err := strconv.Atoi(vals["importance"]); err == nil {
			sumImportance += imp
		}
		if ca, err := strconv.ParseInt(vals["created_at"], 10, 64); err == nil {
			if oldest == 0 || ca < oldest {
				oldest = ca
			}
			if ca > newest {
				newest = ca
			}
		}
	}

	entryCount := len(entries)
	if entryCount > 0 {
		stats.AvgImportance = float64(sumImportance) / float64(entryCount)
	}
	stats.OldestMemoryAt = oldest
	stats.NewestMemoryAt = newest

	return stats, nil
}

// @dev getResultEntries extracts the "results" array from a RESP3 FT.SEARCH response.
// Each entry is a map[any]any with "id", "score", and "extra_attributes" keys.
func getResultEntries(res any) []map[any]any {
	m, ok := res.(map[any]any)
	if !ok {
		return nil
	}
	results, ok := m["results"].([]any)
	if !ok {
		return nil
	}
	entries := make([]map[any]any, 0, len(results))
	for _, item := range results {
		if entry, ok := item.(map[any]any); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

// @dev getExtraAttributes extracts field values from a result entry as map[string]string.
// In RESP3, fields are in "extra_attributes" as map[any]any (not a flat array).
// Fast-path: string values are type-asserted directly (zero-alloc), avoiding fmt.Sprintf
// reflection overhead. Sprintf is only used as fallback for non-string types (rare).
func getExtraAttributes(entry map[any]any) map[string]string {
	attrs, ok := entry["extra_attributes"].(map[any]any)
	if !ok {
		return make(map[string]string)
	}
	m := make(map[string]string, len(attrs))
	for k, v := range attrs {
		key, ok := k.(string)
		if !ok {
			continue
		}
		// Fast path: most Redis hash values are strings, skip fmt reflection
		if s, ok := v.(string); ok {
			m[key] = s
		} else {
			m[key] = fmt.Sprintf("%v", v)
		}
	}
	return m
}

// @dev getMapString extracts a string value from a map[any]any.
func getMapString(m map[any]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// @dev getMapFloat extracts a float64 value from a map[any]any.
func getMapFloat(m map[any]any, key string) float64 {
	if v, ok := m[key]; ok {
		switch f := v.(type) {
		case float64:
			return f
		case string:
			if n, err := strconv.ParseFloat(f, 64); err == nil {
				return n
			}
		}
	}
	return 0
}

// @dev stripMemPrefix removes the "mem:" key prefix to get the plain ULID.
func stripMemPrefix(key string) string {
	return strings.TrimPrefix(key, redis.KeyPrefixMemory)
}
