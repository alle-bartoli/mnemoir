# Agentmem

Persistent memory system for AI coding agents, implemented in Go with Redis.

## Summary

`agentmem` is an MCP (Model Context Protocol) server that gives AI coding agents long-term memory. 
It runs as a child process launched via stdio transport, storing and retrieving structured memories backed by Redis Stack.

Unlike a simple key-value store, the system mimics human memory patterns:

- **Typed memories**: facts (concrete data), concepts (patterns/decisions), narratives (what happened and why)
- **Semantic search**: hybrid vector + full-text search via RediSearch HNSW index
- **Temporal decay**: importance scores degrade over time, boosted on recall (spaced repetition)
- **Session continuity**: sessions carry previous summaries and top memories across restarts
- **AI compression**: raw observations are automatically decomposed into structured memories via Claude API, Ollama, or local rule-based extraction

The stack: Go binary communicates with your MCP client over stdio (JSON-RPC), stores data in Redis Stack (Hashes + RediSearch), generates embeddings via OpenAI, Ollama, or local ONNX, and extracts structured memories via Claude API, Ollama, or local rule-based analysis.

## Features

- **MCP-native**: Exposes 8 tools via Model Context Protocol over stdio transport
- **Hybrid search**: Vector similarity + full-text search via RediSearch
- **Memory types**: `fact`, `concept`, `narrative` with automatic classification
- **Temporal decay**: Importance scores decay over time, boosted on recall (spaced repetition)
- **Triple embedding support**: OpenAI (`text-embedding-3-small`), Ollama (`nomic-embed-text`), or local ONNX (`all-MiniLM-L6-v2`)
- **Flexible compression**: Claude API, Ollama, or local rule-based extraction of structured memories
- **Zero API keys mode**: run fully offline with `local` providers for both embedding and compression
- **Session management**: Start/end sessions with automatic summarization
- **Multi-project**: Scoped memories per project with cross-project recall

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
# Start Redis Stack and build binary
make setup

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

See [docs/configuration.md](docs/configuration.md) for full reference.

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

See [docs/tools-reference.md](docs/tools-reference.md) for parameters and return values.

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

See [docs/architecture.md](docs/architecture.md) for detailed data flows.

## Development

```bash
# Build binary
make build

# Run tests
make test

# Start Redis Stack
make docker-up

# Stop Redis Stack
make docker-down

# Clean build artifacts
make clean

# Install to $GOPATH/bin
make install
```

## License

[MIT](LICENSE)
