package memory

// Memory hash field names. Values must match the RediSearch schema in internal/redis/schema.go.
const (
	FieldContent      = "content"
	FieldType         = "type"
	FieldProject      = "project"
	FieldTags         = "tags"
	FieldImportance   = "importance"
	FieldSessionID    = "session_id"
	FieldCreatedAt    = "created_at"
	FieldLastAccessed = "last_accessed"
	FieldAccessCount  = "access_count"
	FieldEmbedding    = "embedding"
)

// Session hash field names.
const (
	FieldStartedAt   = "started_at"
	FieldEndedAt     = "ended_at"
	FieldSummary     = "summary"
	FieldMemoryCount = "memory_count"
)
