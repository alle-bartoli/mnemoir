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
	for _, t := range toolDefs(h) {
		s.AddTool(mcp.NewTool(t.name, t.opts...), t.handler)
	}
}
