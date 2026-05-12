package mcp

import (
	"context"

	"github.com/alle-bartoli/mnemoir/internal/memory"
	"github.com/mark3labs/mcp-go/mcp"
)

// Tool names.
const (
	toolStoreMemory   = "store_memory"
	toolRecall        = "recall"
	toolForget        = "forget"
	toolStartSession  = "start_session"
	toolEndSession    = "end_session"
	toolRenameProject = "rename_project"
	toolListProjects  = "list_projects"
	toolMemoryStats   = "memory_stats"
	toolUpdateMemory  = "update_memory"
)

// Parameter and response field names.
const (
	paramContent      = "content"
	paramType         = "type"
	paramProject      = "project"
	paramTags         = "tags"
	paramImportance   = "importance"
	paramQuery        = "query"
	paramLimit        = "limit"
	paramSearchMode   = "search_mode"
	paramID           = "id"
	paramOlderThan    = "older_than"
	paramSummary      = "summary"
	paramObservations = "observations"
	paramOldName      = "old_name"
	paramNewName      = "new_name"
	paramCreatedAt    = "created_at"
	paramScore        = "score"
	paramSessionID    = "session_id"
	paramMemoryCount  = "memory_count"
)

type toolDef struct {
	name    string
	opts    []mcp.ToolOption
	handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

func toolDefs(h *Handlers) []toolDef {
	memTypes := []string{string(memory.Fact), string(memory.Concept), string(memory.Narrative)}

	return []toolDef{
		{
			name: toolStoreMemory,
			opts: []mcp.ToolOption{
				mcp.WithDescription("Store a new memory (fact, concept, or narrative) for a project"),
				mcp.WithString(paramContent,
					mcp.Required(),
					mcp.Description("The content of the memory to store"),
				),
				mcp.WithString(paramType,
					mcp.Required(),
					mcp.Description("Memory type"),
					mcp.Enum(memTypes...),
				),
				mcp.WithString(paramProject,
					mcp.Required(),
					mcp.Description("Project name this memory belongs to"),
				),
				mcp.WithString(paramTags,
					mcp.Description("Comma-separated tags for categorization"),
				),
				mcp.WithNumber(paramImportance,
					mcp.Description("Importance level 1-10 (default 5)"),
					mcp.Min(1),
					mcp.Max(10),
				),
			},
			handler: h.StoreMemory,
		},
		{
			name: toolRecall,
			opts: []mcp.ToolOption{
				mcp.WithDescription("Search and recall memories using vector, fulltext, or hybrid search"),
				mcp.WithString(paramQuery,
					mcp.Required(),
					mcp.Description("Search query to find relevant memories"),
				),
				mcp.WithString(paramProject,
					mcp.Description("Filter by project name"),
				),
				mcp.WithString(paramType,
					mcp.Description("Filter by memory type"),
					mcp.Enum(memTypes...),
				),
				mcp.WithNumber(paramLimit,
					mcp.Description("Maximum number of results (default 10)"),
					mcp.Min(1),
					mcp.Max(100),
				),
				mcp.WithString(paramSearchMode,
					mcp.Description("Search strategy to use"),
					mcp.Enum(string(memory.Vector), string(memory.FullText), string(memory.Hybrid)),
				),
			},
			handler: h.Recall,
		},
		{
			name: toolForget,
			opts: []mcp.ToolOption{
				mcp.WithDescription("Delete memories by ID, project, or age"),
				mcp.WithString(paramID,
					mcp.Description("Specific memory ID to delete"),
				),
				mcp.WithString(paramProject,
					mcp.Description("Delete all memories for this project"),
				),
				mcp.WithString(paramOlderThan,
					mcp.Description("Delete memories older than this duration (e.g. '30d', '720h')"),
				),
			},
			handler: h.Forget,
		},
		{
			name: toolStartSession,
			opts: []mcp.ToolOption{
				mcp.WithDescription("Start a new working session for a project, loading previous context"),
				mcp.WithString(paramProject,
					mcp.Required(),
					mcp.Description("Project name to start a session for"),
				),
			},
			handler: h.StartSession,
		},
		{
			name: toolEndSession,
			opts: []mcp.ToolOption{
				mcp.WithDescription("End the current session, optionally extracting memories from observations"),
				mcp.WithString(paramSummary,
					mcp.Description("Brief summary of what was accomplished"),
				),
				mcp.WithString(paramObservations,
					mcp.Description("Raw observations to extract and store as structured memories"),
				),
			},
			handler: h.EndSession,
		},
		{
			name: toolRenameProject,
			opts: []mcp.ToolOption{
				mcp.WithDescription("Rename a project, migrating all memories and sessions to the new name"),
				mcp.WithString(paramOldName,
					mcp.Required(),
					mcp.Description("Current project name"),
				),
				mcp.WithString(paramNewName,
					mcp.Required(),
					mcp.Description("New project name"),
				),
			},
			handler: h.RenameProject,
		},
		{
			name: toolListProjects,
			opts: []mcp.ToolOption{
				mcp.WithDescription("List all projects with memory counts"),
			},
			handler: h.ListProjects,
		},
		{
			name: toolMemoryStats,
			opts: []mcp.ToolOption{
				mcp.WithDescription("Get aggregate statistics about stored memories"),
				mcp.WithString(paramProject,
					mcp.Description("Filter stats by project name"),
				),
			},
			handler: h.MemoryStats,
		},
		{
			name: toolUpdateMemory,
			opts: []mcp.ToolOption{
				mcp.WithDescription("Update fields of an existing memory"),
				mcp.WithString(paramID,
					mcp.Required(),
					mcp.Description("Memory ID to update"),
				),
				mcp.WithNumber(paramImportance,
					mcp.Description("New importance level 1-10"),
					mcp.Min(1),
					mcp.Max(10),
				),
				mcp.WithString(paramTags,
					mcp.Description("New comma-separated tags"),
				),
				mcp.WithString(paramContent,
					mcp.Description("New content (triggers embedding recalculation)"),
				),
			},
			handler: h.UpdateMemory,
		},
	}
}
