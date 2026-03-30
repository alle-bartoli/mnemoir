package redis

// Redis key prefixes and well-known key names.
const (
	KeyPrefixMemory          = "mem:"
	KeyPrefixSession         = "session:"
	KeyPrefixProjectSessions = "project_sessions:"
	KeyProjects              = "projects"
	KeyPrefixMaintLastRun    = "maint:last_run:"
	KeyTagFrequency          = "tags:frequency"
)
