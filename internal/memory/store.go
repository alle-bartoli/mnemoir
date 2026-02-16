package memory

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/alle-bartoli/agentmem/internal/embedding"
	goredis "github.com/redis/go-redis/v9"
)

// Store handles memory CRUD operations backed by Redis.
type Store struct {
	rdb      *goredis.Client
	embedder embedding.IEmbedder
}

// NewStore creates a new memory store.
func NewStore(rdb *goredis.Client, embedder embedding.IEmbedder) *Store {
	return &Store{rdb: rdb, embedder: embedder}
}

// Save persists a memory to Redis with its embedding.
func (s *Store) Save(ctx context.Context, mem *Memory) error {
	emb, err := s.embedder.Embed(ctx, mem.Content)
	if err != nil {
		return fmt.Errorf("generate embedding: %w", err)
	}
	mem.Embedding = emb

	key := "mem:" + mem.ID
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
	pipe.SAdd(ctx, "projects", mem.Project)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("save memory %s: %w", mem.ID, err)
	}

	return nil
}

// Get retrieves a memory by ID.
func (s *Store) Get(ctx context.Context, id string) (*Memory, error) {
	key := "mem:" + id
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
	key := "mem:" + id
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
func (s *Store) DeleteByFilter(ctx context.Context, project string, olderThan time.Duration) (int, error) {
	query := buildFilterQuery(project, olderThan)
	args := []any{"FT.SEARCH", "idx:memories", query, "NOCONTENT", "LIMIT", 0, 10000}

	res, err := s.rdb.Do(ctx, args...).Result()
	if err != nil {
		return 0, fmt.Errorf("search for deletion: %w", err)
	}

	ids := extractIDsFromSearch(res)
	if len(ids) == 0 {
		return 0, nil
	}

	pipe := s.rdb.Pipeline()
	for _, id := range ids {
		pipe.Del(ctx, id)
	}
	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("bulk delete: %w", err)
	}

	count := 0
	for _, cmd := range cmds {
		if cmd.(*goredis.IntCmd).Val() > 0 {
			count++
		}
	}
	return count, nil
}

// UpdateAccess increments access_count and updates last_accessed.
func (s *Store) UpdateAccess(ctx context.Context, id string) error {
	key := "mem:" + id
	pipe := s.rdb.Pipeline()
	pipe.HIncrBy(ctx, key, "access_count", 1)
	pipe.HSet(ctx, key, "last_accessed", time.Now().Unix())
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("update access %s: %w", id, err)
	}
	return nil
}

// Update modifies specific fields of a memory. Recalculates embedding if content changes.
func (s *Store) Update(ctx context.Context, id string, fields map[string]any) error {
	key := "mem:" + id

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
	return s.rdb.SMembers(ctx, "projects").Result()
}

// CountByProject returns the number of memories for a project.
func (s *Store) CountByProject(ctx context.Context, project string) (int, error) {
	query := fmt.Sprintf("@project:{%s}", escapeTag(project))
	res, err := s.rdb.Do(ctx, "FT.SEARCH", "idx:memories", query, "NOCONTENT", "LIMIT", 0, 0).Result()
	if err != nil {
		return 0, fmt.Errorf("count by project: %w", err)
	}
	return extractTotalFromSearch(res), nil
}

// GetStats returns aggregate statistics, optionally filtered by project.
func (s *Store) GetStats(ctx context.Context, project string) (*MemoryStats, error) {
	query := "*"
	if project != "" {
		query = fmt.Sprintf("@project:{%s}", escapeTag(project))
	}

	res, err := s.rdb.Do(ctx,
		"FT.SEARCH", "idx:memories", query,
		"RETURN", "3", "type", "importance", "created_at",
		"LIMIT", 0, 10000,
	).Result()
	if err != nil {
		return nil, fmt.Errorf("stats query: %w", err)
	}

	return computeStats(res)
}

// SaveSession persists a session hash to Redis.
func (s *Store) SaveSession(ctx context.Context, sess *Session) error {
	key := "session:" + sess.ID
	fields := map[string]any{
		"project":      sess.Project,
		"started_at":   sess.StartedAt,
		"ended_at":     sess.EndedAt,
		"summary":      sess.Summary,
		"memory_count": sess.MemoryCount,
	}
	if err := s.rdb.HSet(ctx, key, fields).Err(); err != nil {
		return fmt.Errorf("save session %s: %w", sess.ID, err)
	}
	return nil
}

// GetLastSession retrieves the most recent session for a project.
func (s *Store) GetLastSession(ctx context.Context, project string) (*Session, error) {
	// Scan session keys and find the latest for this project
	var cursor uint64
	var latestKey string
	var latestStarted int64

	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, "session:*", 100).Result()
		if err != nil {
			return nil, fmt.Errorf("scan sessions: %w", err)
		}

		for _, key := range keys {
			proj, err := s.rdb.HGet(ctx, key, "project").Result()
			if err != nil || proj != project {
				continue
			}
			startedStr, err := s.rdb.HGet(ctx, key, "started_at").Result()
			if err != nil {
				continue
			}
			started, _ := strconv.ParseInt(startedStr, 10, 64)
			if started > latestStarted {
				latestStarted = started
				latestKey = key
			}
		}

		cursor = next
		if cursor == 0 {
			break
		}
	}

	if latestKey == "" {
		return nil, nil // No previous session
	}

	vals, err := s.rdb.HGetAll(ctx, latestKey).Result()
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	return hashToSession(latestKey, vals), nil
}

// GetTopMemories retrieves top memories by importance for a project.
func (s *Store) GetTopMemories(ctx context.Context, project string, limit int) ([]Memory, error) {
	query := fmt.Sprintf("@project:{%s}", escapeTag(project))
	res, err := s.rdb.Do(ctx,
		"FT.SEARCH", "idx:memories", query,
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
	id := key
	if len(key) > 8 {
		id = key[8:] // Remove "session:" prefix
	}

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

func escapeTag(s string) string {
	// RediSearch TAG fields need escaping for special chars
	replacer := []string{
		"-", "\\-",
		".", "\\.",
		"@", "\\@",
		" ", "\\ ",
	}
	result := s
	for i := 0; i < len(replacer); i += 2 {
		result = replaceAll(result, replacer[i], replacer[i+1])
	}
	return result
}

func replaceAll(s, old, new string) string {
	result := ""
	for i := 0; i < len(s); i++ {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			result += new
			i += len(old) - 1
		} else {
			result += string(s[i])
		}
	}
	return result
}

func buildFilterQuery(project string, olderThan time.Duration) string {
	parts := []string{}
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
	query := ""
	for i, p := range parts {
		if i > 0 {
			query += " "
		}
		query += p
	}
	return query
}

func extractIDsFromSearch(res any) []string {
	arr, ok := res.([]any)
	if !ok || len(arr) < 1 {
		return nil
	}

	var ids []string
	// arr[0] is the total count, then alternating key, fields
	for i := 1; i < len(arr); i++ {
		if key, ok := arr[i].(string); ok {
			ids = append(ids, key)
		}
	}
	return ids
}

func extractTotalFromSearch(res any) int {
	arr, ok := res.([]any)
	if !ok || len(arr) < 1 {
		return 0
	}
	if total, ok := arr[0].(int64); ok {
		return int(total)
	}
	return 0
}

func extractMemoriesFromSearch(res any) ([]Memory, error) {
	arr, ok := res.([]any)
	if !ok || len(arr) < 1 {
		return nil, nil
	}

	var memories []Memory
	// Format: [total, key1, [field1, val1, ...], key2, [field2, val2, ...], ...]
	for i := 1; i+1 < len(arr); i += 2 {
		key, ok := arr[i].(string)
		if !ok {
			continue
		}

		fieldArr, ok := arr[i+1].([]any)
		if !ok {
			continue
		}

		vals := arrayToMap(fieldArr)
		// Extract ID from key "mem:ULID"
		id := key
		if len(key) > 4 {
			id = key[4:]
		}

		mem, err := hashToMemory(id, vals)
		if err != nil {
			continue
		}
		memories = append(memories, *mem)
	}

	return memories, nil
}

func computeStats(res any) (*MemoryStats, error) {
	arr, ok := res.([]any)
	if !ok || len(arr) < 1 {
		return &MemoryStats{ByType: map[string]int{}}, nil
	}

	total := 0
	if t, ok := arr[0].(int64); ok {
		total = int(t)
	}

	stats := &MemoryStats{
		Total:  total,
		ByType: map[string]int{"fact": 0, "concept": 0, "narrative": 0},
	}

	var sumImportance int
	var oldest, newest int64

	for i := 1; i+1 < len(arr); i += 2 {
		fieldArr, ok := arr[i+1].([]any)
		if !ok {
			continue
		}
		vals := arrayToMap(fieldArr)

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

	if total > 0 {
		stats.AvgImportance = float64(sumImportance) / float64(total)
	}
	stats.OldestMemoryAt = oldest
	stats.NewestMemoryAt = newest

	return stats, nil
}

func arrayToMap(arr []any) map[string]string {
	m := make(map[string]string)
	for i := 0; i+1 < len(arr); i += 2 {
		key, ok1 := arr[i].(string)
		val, ok2 := arr[i+1].(string)
		if ok1 && ok2 {
			m[key] = val
		}
	}
	return m
}
