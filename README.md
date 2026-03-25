# Agentmem

Personal memory layer for AI coding agents. Runs offline, keeps data local.

## Why I built this

Claude Code's built-in memory (CLAUDE.md + auto-memory) is file-based and flat. No semantic search, no decay, no scoring. I wanted something that works more like actual memory: things you use often stay sharp, things you forget fade away.

The key ideas behind agentmem:

- **Offline by default.** The entire stack runs without API keys. Local ONNX embeddings and rule-based compression keep everything on my machine. Cloud providers (OpenAI, Claude API, Ollama) are supported but optional.
- **Spaced repetition.** Importance scores decay over time (`importance * 0.9^weeks`). Each recall boosts the score back up. This means the agent's context naturally prioritizes what matters.
- **Typed memories.** Not everything is a flat string. Facts (concrete data), concepts (patterns/decisions), and narratives (what happened and why) are stored and searched differently.

## Summary

`agentmem` is an MCP server that gives AI coding agents long-term memory. It runs as a child process via stdio transport, backed by Redis Stack.

The stack: Go binary over stdio (JSON-RPC), Redis Stack (Hashes + RediSearch), embeddings via OpenAI/Ollama/local ONNX, memory extraction via Claude API/Ollama/local rules.

## Features

- **Offline-first**: local ONNX embeddings + rule-based compression, no API keys required
- **Spaced repetition**: importance decay over time, boost on recall
- **Typed memories**: `fact`, `concept`, `narrative` with automatic classification
- **Hybrid search**: vector (KNN/HNSW) + full-text (TF-IDF) via RediSearch
- **MCP-native**: 8 tools via Model Context Protocol over stdio
- **Triple embedding**: OpenAI (`text-embedding-3-small`, 1536d), Ollama (`nomic-embed-text`, 768d), local ONNX (`all-MiniLM-L6-v2`, 384d)
- **Flexible compression**: Claude API, Ollama, or local rule-based extraction
- **Session management**: start/end sessions with automatic summarization
- **Multi-project**: scoped memories per project with cross-project recall

## Prerequisites

- Go 1.21 or higher
- Docker and Docker Compose
- Redis Stack 7.2+ (provided via `docker-compose.yml`)
- API keys (optional when using `local` providers):
  - `ANTHROPIC_API_KEY` (for compressor if using Claude)
  - `OPENAI_API_KEY` (for embeddings if using OpenAI)
- An MCP-compatible client (e.g. Claude Code, Cursor, Windsurf)

## Quick Start

```bash
# Show all available commands
make help

# Start Redis Stack and build binary
make setup

# Open RedisInsight web UI (optional)
make redis-ui

# Register MCP server with your client
make mcp-register

# Verify registration (in your MCP client)
/mcp
```

Configure API keys in your environment:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
export OPENAI_API_KEY="sk-..."
```

Edit `~/.agentmem/config.toml` to customize providers and behavior.

## Configuration

Default configuration copied to `~/.agentmem/config.toml` on first setup.

Key settings:

- `compressor.provider`: `claude`, `ollama`, or `local`
- `embedding.provider`: `openai`, `ollama`, or `local`
- `memory.auto_decay`: Enable temporal decay (default: `true`)
- `session.max_recall_items`: Limit recalled memories (default: `20`)


## MCP Tools

| Tool            | Description                                                  |
| --------------- | ------------------------------------------------------------ |
| `store_memory`  | Store new memory (auto-classifies type, generates embedding) |
| `recall`        | Retrieve memories by query (vector + text search)            |
| `forget`        | Delete memories by ID, project, or age                       |
| `update_memory` | Modify existing memory content or metadata                   |
| `start_session` | Begin tracked session (optionally scoped to project)         |
| `end_session`   | Close session with automatic summarization                   |
| `list_projects` | List all projects with memory counts                         |
| `memory_stats`  | Get statistics (total memories, types, avg importance)       |


## Architecture

```
Client <--stdio/JSON-RPC--> agentmem <--TCP--> Redis Stack
                                      |
                                      +--> OpenAI API (embeddings)
                                      +--> Anthropic API (compression)
                                      +--> Ollama (local alternative)
                                      +--> Local ONNX / rules (zero API keys)
```

| Layer      | Technology                  | Role                                                                                      |
| ---------- | --------------------------- | ----------------------------------------------------------------------------------------- |
| Transport  | stdio (MCP)                 | `mark3labs/mcp-go` JSON-RPC over stdin/stdout                                             |
| Storage    | Redis Stack                 | Hash storage + RediSearch (vector + FTS)                                                  |
| Embeddings | OpenAI / Ollama / Local     | `text-embedding-3-small` (1536d), `nomic-embed-text` (768d), or `all-MiniLM-L6-v2` (384d) |
| Compressor | Claude API / Ollama / Local | Structured memory extraction from raw observations                                        |
| IDs        | ULID                        | Chronologically sortable unique identifiers                                               |
| Config     | TOML                        | `~/.agentmem/config.toml`                                                                 |

Directory structure:

```
agentmem/
├── cmd/agentmem/        # Entry point, CLI flags
├── internal/
│   ├── compressor/      # AI-powered memory extraction
│   ├── config/          # TOML configuration loading
│   ├── embedding/       # Vector generation (OpenAI/Ollama)
│   ├── mcp/             # MCP server + tool handlers
│   ├── memory/          # Core types, CRUD, search
│   └── redis/           # Connection pool, index management
├── config/              # Default config template
└── docs/                # Architecture, tools reference, setup
```


## Development

```bash
# Show all available commands
make help

# Build binary
make build

# Run tests
make test

# Start Redis Stack
make docker-up

# Open RedisInsight web UI (http://localhost:8001)
make redis-ui

# Stop Redis Stack
make docker-down

# Clean build artifacts
make clean

# Install to $GOPATH/bin
make install
```

### RedisInsight Web UI

Redis Stack includes **RedisInsight** on port `8001`. Open it with:

```bash
make redis-ui
```

Use it to:

- Browse memories (`mem:{ulid}` hashes)
- Inspect search index (`idx:memories`)
- Monitor queries in real-time (Profiler)
- Run Redis commands (Workbench)

### Debugging with LazyVim/nvim-dap

For Go debugging with Delve:

1. Install Delve: `go install github.com/go-delve/delve/cmd/dlv@latest`
2. Ensure Redis is running: `make docker-up`
3. Open a Go file: `nvim cmd/agentmem/main.go`
4. Set breakpoint: `<leader>db`
5. Start debugger: `<leader>dc` → select "Debug Package"

## License

[MIT](LICENSE)
