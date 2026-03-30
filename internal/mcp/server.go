// Package mcp implements the MCP server and tool handlers.
package mcp

import (
	"github.com/alle-bartoli/mnemoir/internal/compressor"
	"github.com/alle-bartoli/mnemoir/internal/config"
	"github.com/alle-bartoli/mnemoir/internal/memory"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/redis/go-redis/v9"
)

// NewServer creates an MCP server with all tools registered.
// Returns both the MCP server and the Handlers for sideband HTTP endpoints.
func NewServer(store *memory.Store, comp compressor.ICompressor, cfg *config.Config, rdb *redis.Client) (*server.MCPServer, *Handlers) {
	s := server.NewMCPServer(
		"mnemoir",
		"0.0.0",
		server.WithToolCapabilities(true),
	)

	h := &Handlers{
		store:      store,
		compressor: comp,
		cfg:        cfg,
		rdb:        rdb,
	}

	registerTools(s, h)
	return s, h
}

func registerTools(s *server.MCPServer, h *Handlers) {
	// store_memory
	s.AddTool(mcp.NewTool("store_memory",
		mcp.WithDescription("Store a new memory (fact, concept, or narrative) for a project"),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("The content of the memory to store"),
		),
		mcp.WithString("type",
			mcp.Required(),
			mcp.Description("Memory type"),
			mcp.Enum(string(memory.Fact), string(memory.Concept), string(memory.Narrative)),
		),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name this memory belongs to"),
		),
		mcp.WithString("tags",
			mcp.Description("Comma-separated tags for categorization"),
		),
		mcp.WithNumber("importance",
			mcp.Description("Importance level 1-10 (default 5)"),
			mcp.Min(1),
			mcp.Max(10),
		),
	), h.StoreMemory)

	// recall
	s.AddTool(mcp.NewTool("recall",
		mcp.WithDescription("Search and recall memories using vector, fulltext, or hybrid search"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query to find relevant memories"),
		),
		mcp.WithString("project",
			mcp.Description("Filter by project name"),
		),
		mcp.WithString("type",
			mcp.Description("Filter by memory type"),
			mcp.Enum(string(memory.Fact), string(memory.Concept), string(memory.Narrative)),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results (default 10)"),
			mcp.Min(1),
			mcp.Max(100),
		),
		mcp.WithString("search_mode",
			mcp.Description("Search strategy to use"),
			mcp.Enum(string(memory.Vector), string(memory.FullText), string(memory.Hybrid)),
		),
	), h.Recall)

	// forget
	s.AddTool(mcp.NewTool("forget",
		mcp.WithDescription("Delete memories by ID, project, or age"),
		mcp.WithString("id",
			mcp.Description("Specific memory ID to delete"),
		),
		mcp.WithString("project",
			mcp.Description("Delete all memories for this project"),
		),
		mcp.WithString("older_than",
			mcp.Description("Delete memories older than this duration (e.g. '30d', '720h')"),
		),
	), h.Forget)

	// start_session
	s.AddTool(mcp.NewTool("start_session",
		mcp.WithDescription("Start a new working session for a project, loading previous context"),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name to start a session for"),
		),
	), h.StartSession)

	// end_session
	s.AddTool(mcp.NewTool("end_session",
		mcp.WithDescription("End the current session, optionally extracting memories from observations"),
		mcp.WithString("summary",
			mcp.Description("Brief summary of what was accomplished"),
		),
		mcp.WithString("observations",
			mcp.Description("Raw observations to extract and store as structured memories"),
		),
	), h.EndSession)

	// list_projects
	s.AddTool(mcp.NewTool("list_projects",
		mcp.WithDescription("List all projects with memory counts"),
	), h.ListProjects)

	// memory_stats
	s.AddTool(mcp.NewTool("memory_stats",
		mcp.WithDescription("Get aggregate statistics about stored memories"),
		mcp.WithString("project",
			mcp.Description("Filter stats by project name"),
		),
	), h.MemoryStats)

	// update_memory
	s.AddTool(mcp.NewTool("update_memory",
		mcp.WithDescription("Update fields of an existing memory"),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Memory ID to update"),
		),
		mcp.WithNumber("importance",
			mcp.Description("New importance level 1-10"),
			mcp.Min(1),
			mcp.Max(10),
		),
		mcp.WithString("tags",
			mcp.Description("New comma-separated tags"),
		),
		mcp.WithString("content",
			mcp.Description("New content (triggers embedding recalculation)"),
		),
	), h.UpdateMemory)
}
