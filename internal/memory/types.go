// Package memory implements persistent memory storage, CRUD, and search.
package memory

import "time"

// @dev MemoryType classifies the kind of memory stored.
// String alias (not iota/int) because:
//   - Serializes directly to JSON/Redis without a conversion layer
//   - Readable in logs and debug output
//   - Tradeoff: no compile-time type safety, but values are validated at
//     the boundary (MCP handler) via ValidMemoryType()
type MemoryType string

const (
	Fact      MemoryType = "fact"
	Concept   MemoryType = "concept"
	Narrative MemoryType = "narrative"
)

// ValidMemoryType checks if the given string is a valid memory type.
func ValidMemoryType(s string) bool {
	switch MemoryType(s) {
	case Fact, Concept, Narrative:
		return true
	}
	return false
}

// SearchMode defines how recall queries are executed.
type SearchMode string

const (
	Vector   SearchMode = "vector"
	FullText SearchMode = "fulltext"
	Hybrid   SearchMode = "hybrid"
)

// Memory represents a single stored memory.
type Memory struct {
	ID           string     `json:"id"`
	Content      string     `json:"content"`
	Type         MemoryType `json:"type"`
	Project      string     `json:"project"`
	Tags         string     `json:"tags"`
	Importance   int        `json:"importance"`
	SessionID    string     `json:"session_id"`
	CreatedAt    int64      `json:"created_at"`
	LastAccessed int64      `json:"last_accessed"`
	AccessCount  int        `json:"access_count"`
	Embedding    []float32  `json:"-"`
}

// CreatedAtTime returns CreatedAt as time.Time.
func (m *Memory) CreatedAtTime() time.Time {
	return time.Unix(m.CreatedAt, 0)
}

// Session tracks a working session with a project.
type Session struct {
	ID          string `json:"id"`
	Project     string `json:"project"`
	StartedAt   int64  `json:"started_at"`
	EndedAt     int64  `json:"ended_at"`
	Summary     string `json:"summary"`
	MemoryCount int    `json:"memory_count"`
}

// SearchResult wraps a Memory with its relevance score.
type SearchResult struct {
	Memory Memory  `json:"memory"`
	Score  float64 `json:"score"`
}

// SearchFilters constrains search results.
type SearchFilters struct {
	Project string
	Type    string
}

// ProjectInfo holds project-level aggregate data.
type ProjectInfo struct {
	Project      string `json:"project"`
	MemoryCount  int    `json:"memory_count"`
	LastActivity int64  `json:"last_activity"`
}

// MemoryStats holds aggregate statistics.
type MemoryStats struct {
	Total          int            `json:"total"`
	ByType         map[string]int `json:"by_type"`
	AvgImportance  float64        `json:"avg_importance"`
	OldestMemoryAt int64          `json:"oldest"`
	NewestMemoryAt int64          `json:"newest"`
}
