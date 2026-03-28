package mcp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/alle-bartoli/agentmem/internal/compressor"
	"github.com/alle-bartoli/agentmem/internal/config"
	"github.com/alle-bartoli/agentmem/internal/memory"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oklog/ulid/v2"
)

// Security: input length limits prevent oversized payloads
const (
	maxContentLen = 50_000 // 50KB
	maxQueryLen   = 4_000  // 4KB
	maxProjectLen = 128
	maxTagsLen    = 1_000
)

func validateLength(name, value string, max int) error {
	if len(value) > max {
		return fmt.Errorf("%s exceeds max length (%d > %d)", name, len(value), max)
	}
	return nil
}

// Handlers implements all MCP tool handlers.
type Handlers struct {
	store      *memory.Store
	compressor compressor.ICompressor
	cfg        *config.Config

	mu            sync.Mutex // Security: protects activeSession from concurrent access
	activeSession *memory.Session
}

// StoreMemory handles the store_memory tool.
func (h *Handlers) StoreMemory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	content, err := req.RequireString("content")
	if err != nil {
		return mcp.NewToolResultError("content is required"), nil
	}
	if err := validateLength("content", content, maxContentLen); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	memType, err := req.RequireString("type")
	if err != nil {
		return mcp.NewToolResultError("type is required"), nil
	}

	if !memory.ValidMemoryType(memType) {
		return mcp.NewToolResultError("type must be one of: fact, concept, narrative"), nil
	}

	project, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError("project is required"), nil
	}
	if err := memory.ValidateTagValue(project); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := validateLength("project", project, maxProjectLen); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	tags := req.GetString("tags", "")
	importance := req.GetInt("importance", h.cfg.Memory.DefaultImportance)

	now := time.Now().Unix()
	id := newULID()

	h.mu.Lock()
	sessionID := ""
	if h.activeSession != nil {
		sessionID = h.activeSession.ID
	}
	h.mu.Unlock()

	mem := &memory.Memory{
		ID:           id,
		Content:      content,
		Type:         memory.MemoryType(memType),
		Project:      project,
		Tags:         tags,
		Importance:   importance,
		SessionID:    sessionID,
		CreatedAt:    now,
		LastAccessed: now,
		AccessCount:  0,
	}

	if err := h.store.Save(ctx, mem); err != nil {
		return nil, fmt.Errorf("save memory: %w", err)
	}

	result := map[string]any{
		"id":         id,
		"type":       memType,
		"project":    project,
		"created_at": now,
	}
	return jsonResult(result)
}

// Recall handles the recall tool.
func (h *Handlers) Recall(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil
	}
	if err := validateLength("query", query, maxQueryLen); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	project := req.GetString("project", "")
	memType := req.GetString("type", "")
	limit := req.GetInt("limit", 10)
	searchMode := req.GetString("search_mode", "hybrid")

	if project != "" {
		if err := memory.ValidateTagValue(project); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}
	if memType != "" {
		if err := memory.ValidateTagValue(memType); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	filters := memory.SearchFilters{
		Project: project,
		Type:    memType,
	}

	var results []memory.SearchResult

	switch memory.SearchMode(searchMode) {
	case memory.Vector:
		results, err = h.store.VectorSearch(ctx, query, filters, limit)
	case memory.FullText:
		results, err = h.store.FullTextSearch(ctx, query, filters, limit)
	case memory.Hybrid:
		results, err = h.store.HybridSearch(ctx, query, filters, limit)
	default:
		return mcp.NewToolResultError("search_mode must be one of: vector, fulltext, hybrid"), nil
	}

	if err != nil {
		return nil, fmt.Errorf("recall search: %w", err)
	}

	// Build response
	items := make([]map[string]any, 0, len(results))
	for _, r := range results {
		items = append(items, map[string]any{
			"id":         r.Memory.ID,
			"content":    r.Memory.Content,
			"type":       string(r.Memory.Type),
			"project":    r.Memory.Project,
			"tags":       r.Memory.Tags,
			"importance": r.Memory.Importance,
			"score":      r.Score,
			"created_at": r.Memory.CreatedAt,
		})
	}

	return jsonResult(items)
}

// Forget handles the forget tool.
func (h *Handlers) Forget(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := req.GetString("id", "")
	project := req.GetString("project", "")
	olderThan := req.GetString("older_than", "")

	if project != "" {
		if err := memory.ValidateTagValue(project); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	if id == "" && project == "" && olderThan == "" {
		return mcp.NewToolResultError("at least one of id, project, or older_than is required"), nil
	}

	deletedCount := 0

	// Delete by specific ID
	if id != "" {
		if err := h.store.Delete(ctx, id); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete failed: %v", err)), nil
		}
		deletedCount++
	}

	// Delete by filter (project and/or age)
	if project != "" || olderThan != "" {
		var duration time.Duration
		if olderThan != "" {
			d, err := parseDuration(olderThan)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid older_than format: %v", err)), nil
			}
			duration = d
		}

		count, err := h.store.DeleteByFilter(ctx, project, duration)
		if err != nil {
			return nil, fmt.Errorf("delete by filter: %w", err)
		}
		deletedCount += count
	}

	return jsonResult(map[string]any{"deleted_count": deletedCount})
}

// StartSession handles the start_session tool.
func (h *Handlers) StartSession(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError("project is required"), nil
	}
	if err := memory.ValidateTagValue(project); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	sessionID := newULID()
	now := time.Now().Unix()

	sess := &memory.Session{
		ID:        sessionID,
		Project:   project,
		StartedAt: now,
	}

	if saveErr := h.store.SaveSession(ctx, sess); saveErr != nil {
		return nil, fmt.Errorf("save session: %w", saveErr)
	}

	h.mu.Lock()
	h.activeSession = sess
	h.mu.Unlock()

	// Retrieve previous session summary
	var previousSummary string
	lastSess, err := h.store.GetLastSession(ctx, project)
	if err == nil && lastSess != nil && lastSess.ID != sessionID {
		previousSummary = lastSess.Summary
	}

	// Retrieve top memories by importance
	topMemories, _ := h.store.GetTopMemories(ctx, project, 10)
	keyMemories := make([]map[string]any, 0, len(topMemories))
	for _, m := range topMemories {
		keyMemories = append(keyMemories, map[string]any{
			"id":         m.ID,
			"content":    m.Content,
			"type":       string(m.Type),
			"importance": m.Importance,
			"tags":       m.Tags,
		})
	}

	result := map[string]any{
		"session_id":       sessionID,
		"previous_summary": previousSummary,
		"key_memories":     keyMemories,
	}
	return jsonResult(result)
}

// EndSession handles the end_session tool.
func (h *Handlers) EndSession(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.mu.Lock()
	sess := h.activeSession
	h.activeSession = nil
	h.mu.Unlock()

	if sess == nil {
		return mcp.NewToolResultError("no active session to end"), nil
	}

	summary := req.GetString("summary", "")
	observations := req.GetString("observations", "")
	now := time.Now().Unix()

	memoriesCreated := 0

	// Extract structured memories from observations
	if observations != "" && h.compressor != nil {
		compressed, err := h.compressor.Compress(ctx, observations)
		if err != nil {
			log.Printf("compression failed: %v", err) // Security: log details internally
			summary += "\n[compression failed]"           // Security: generic message in user-facing summary
		} else {
			memoriesCreated, _ = h.saveExtracted(ctx, compressed, sess.Project)
		}
	}

	// Update session
	sess.EndedAt = now
	sess.Summary = summary
	sess.MemoryCount = memoriesCreated

	if err := h.store.SaveSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("update session: %w", err)
	}

	duration := now - sess.StartedAt

	result := map[string]any{
		"memories_created": memoriesCreated,
		"session_duration": duration,
	}

	return jsonResult(result)
}

// ListProjects handles the list_projects tool.
func (h *Handlers) ListProjects(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projects, err := h.store.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	items := make([]map[string]any, 0, len(projects))
	for _, p := range projects {
		count, _ := h.store.CountByProject(ctx, p)
		items = append(items, map[string]any{
			"project":      p,
			"memory_count": count,
		})
	}

	return jsonResult(items)
}

// MemoryStats handles the memory_stats tool.
func (h *Handlers) MemoryStats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project := req.GetString("project", "")
	if project != "" {
		if err := memory.ValidateTagValue(project); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	stats, err := h.store.GetStats(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("memory stats: %w", err)
	}

	return jsonResult(stats)
}

// UpdateMemory handles the update_memory tool.
func (h *Handlers) UpdateMemory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}

	fields := make(map[string]any)
	updatedFields := []string{}

	if content := req.GetString("content", ""); content != "" {
		fields["content"] = content
		updatedFields = append(updatedFields, "content")
	}
	if tags := req.GetString("tags", ""); tags != "" {
		fields["tags"] = tags
		updatedFields = append(updatedFields, "tags")
	}
	if importance := req.GetInt("importance", 0); importance > 0 {
		fields["importance"] = importance
		updatedFields = append(updatedFields, "importance")
	}

	if len(fields) == 0 {
		return mcp.NewToolResultError("no fields to update"), nil
	}

	if err := h.store.Update(ctx, id, fields); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("update failed: %v", err)), nil
	}

	result := map[string]any{
		"id":             id,
		"updated_fields": updatedFields,
	}
	return jsonResult(result)
}

// PRIVATE

func (h *Handlers) saveExtracted(ctx context.Context, cr *compressor.CompressResult, project string) (int, error) {
	count := 0
	now := time.Now().Unix()

	h.mu.Lock()
	sessionID := ""
	if h.activeSession != nil {
		sessionID = h.activeSession.ID
	}
	h.mu.Unlock()

	save := func(items []compressor.ExtractedMemory, memType memory.MemoryType) {
		for _, item := range items {
			mem := &memory.Memory{
				ID:           newULID(),
				Content:      item.Content,
				Type:         memType,
				Project:      project,
				Tags:         item.Tags,
				Importance:   item.Importance,
				SessionID:    sessionID,
				CreatedAt:    now,
				LastAccessed: now,
				AccessCount:  0,
			}
			if err := h.store.Save(ctx, mem); err == nil {
				count++
			}
		}
	}

	save(cr.Facts, memory.Fact)
	save(cr.Concepts, memory.Concept)
	save(cr.Narratives, memory.Narrative)

	return count, nil
}

// Security: use crypto/rand instead of math/rand for unpredictable IDs
func newULID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
}

func parseDuration(s string) (time.Duration, error) {
	// Support "30d" format in addition to Go's standard duration
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err != nil {
			return 0, fmt.Errorf("invalid day format: %s", s)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func jsonResult(data any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return mcp.NewToolResultText(string(b)), nil
}
