// Package dbops implements database backup and restore operations for mnemoir.
package dbops

// snapshotVersion is incremented when the JSON format changes in a breaking way.
const snapshotVersion = 1

// @dev Snapshot is the portable JSON representation of a complete backup.
// All Redis data types are flattened: hashes become structs, sorted sets become
// []ScoredEntry, and the projects SET becomes []string. The format is intentionally
// human-readable so snapshots can be inspected and manually edited.
// EmbeddingDim is captured at dump time so Restore can reject a dimension mismatch
// before writing any data.
type Snapshot struct {
	Version         int                      `json:"version"`
	CreatedAt       int64                    `json:"created_at"`
	EmbeddingDim    int                      `json:"embedding_dim"`
	Memories        []MemoryRecord           `json:"memories"`
	Sessions        []SessionRecord          `json:"sessions"`
	Projects        []string                 `json:"projects"`
	ProjectSessions map[string][]ScoredEntry `json:"project_sessions"`
	TagFrequencies  []ScoredEntry            `json:"tag_frequencies"`
}

// SnapshotHeader contains only the metadata fields of a snapshot.
// Used to peek at version and embedding_dim without decoding the full payload.
type SnapshotHeader struct {
	Version      int `json:"version"`
	EmbeddingDim int `json:"embedding_dim"`
}

// @dev MemoryRecord mirrors the Redis hash stored at mem:{ULID}.
// All numeric fields are kept as strings to preserve exact Redis representation
// without lossy float64 conversion. Embedding holds the raw float32 LE blob
// base64-encoded; restoring it via HSet bypasses the embedder entirely.
type MemoryRecord struct {
	ID           string `json:"id"`
	Content      string `json:"content"`
	Type         string `json:"type"`
	Project      string `json:"project"`
	Tags         string `json:"tags"`
	Importance   string `json:"importance"`
	SessionID    string `json:"session_id"`
	CreatedAt    string `json:"created_at"`
	LastAccessed string `json:"last_accessed"`
	AccessCount  string `json:"access_count"`
	Embedding    string `json:"embedding,omitempty"`
}

// @dev SessionRecord mirrors the Redis hash stored at session:{ULID}.
type SessionRecord struct {
	ID          string `json:"id"`
	Project     string `json:"project"`
	StartedAt   string `json:"started_at"`
	EndedAt     string `json:"ended_at"`
	Summary     string `json:"summary"`
	MemoryCount string `json:"memory_count"`
}

// @dev ScoredEntry represents one element of a Redis sorted set (ZSET).
// Used for both project_sessions:{project} and tags:frequency.
type ScoredEntry struct {
	Member string  `json:"member"`
	Score  float64 `json:"score"`
}

// @dev RestoreResult reports how many entities were written and whether RediSearch
// finished re-indexing within the timeout window.
type RestoreResult struct {
	Memories         int
	Sessions         int
	Projects         int
	TagScores        int
	IndexingComplete bool
}
