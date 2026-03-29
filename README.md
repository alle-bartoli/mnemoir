# Agentmem

MCP server that gives AI coding agents long-term memory.
Runs as a child process via stdio transport, backed by Redis Stack.
Fully offline-capable, no API keys required.

## Features

- **Offline-first**: local ONNX embeddings (`all-MiniLM-L6-v2`, 384d) + rule-based compression, no API keys required
- **Spaced repetition**: lazy temporal decay + recall boost on importance scores, computed at query time
- **Typed memories**: `fact`, `concept`, `narrative` with automatic classification
- **Hybrid search**: vector (KNN/HNSW) + full-text (TF-IDF) via RediSearch
- **MCP-native**: 8 tools via Model Context Protocol over stdio
- **Triple embedding**: OpenAI (`text-embedding-3-small`, 1536d), Ollama (`nomic-embed-text`, 768d), local ONNX (`all-MiniLM-L6-v2`, 384d)
- **Flexible compression**: Claude API, Ollama, or local rule-based extraction
- **Session management**: start/end sessions with automatic summarization from extracted memories
- **Multi-project**: scoped memories per project with cross-project recall

## Prerequisites

- Go 1.25+
- Docker and Docker Compose
- Redis Stack 7.2+ (provided via `docker-compose.yml`)
- An MCP-compatible client (Claude Code, Cursor, Windsurf)

## Quick Start

```bash
# Start Redis Stack and build binary
make setup

# Register MCP server with your client
make mcp-register

# Verify registration (in your MCP client)
/mcp
```

Configure environment variables:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."     # Required for Claude compressor
export OPENAI_API_KEY="sk-..."            # Required for OpenAI embeddings
export AGENTMEM_REDIS_PASSWORD="secret"   # Redis auth (default: changeme)
```

Edit `~/.agentmem/config.toml` to customize providers and behavior.

## Configuration

Default configuration copied to `~/.agentmem/config.toml` on first setup.

| Key                          | Default | Description                                   |
| ---------------------------- | ------- | --------------------------------------------- |
| `compressor.provider`        | `local` | `claude`, `ollama`, or `local`                |
| `embedding.provider`         | `local` | `openai`, `ollama`, or `local`                |
| `memory.auto_decay`          | `true`  | Enable temporal decay                         |
| `memory.decay_factor`        | `0.9`   | Multiplier per interval (0.9 = 10% loss/week) |
| `memory.decay_interval`      | `168h`  | Time between decay steps (1 week)             |
| `memory.vector_weight`       | `0.60`  | Semantic search weight in hybrid scoring      |
| `memory.fts_weight`          | `0.25`  | Keyword search weight in hybrid scoring       |
| `memory.importance_weight`   | `0.15`  | Importance weight in hybrid scoring           |
| `memory.access_boost_factor` | `0.3`   | Points gained per recall                      |
| `memory.access_boost_cap`    | `2.0`   | Max boost from recalls                        |
| `session.max_recall_items`   | `20`    | Limit recalled memories per session           |

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

## Spaced Repetition

Importance scores decay over time and boost on recall. Computed lazily at query time with no background goroutines.

### Formula

```
effective = base * decay_factor^intervals + min(boost_cap, access_count * boost_factor)
```

- `intervals`: number of `decay_interval` periods since last decay application
- Result is clamped to `[1, 10]`

### Decay examples

| Scenario             | Importance | Weeks idle | Recalls | Effective |
| -------------------- | ---------- | ---------- | ------- | --------- |
| Fresh memory         | 8          | 0          | 0       | 8.0       |
| One week idle        | 8          | 1          | 0       | 7.2       |
| Three weeks idle     | 8          | 3          | 0       | 5.8       |
| Frequently recalled  | 8          | 3          | 5       | 7.3       |
| Old but heavily used | 5          | 8          | 10      | 4.1       |
| Minimum floor        | 3          | 20         | 0       | 1.0       |

### Hybrid search weights

| Signal              | Default weight | What it captures                            |
| ------------------- | -------------- | ------------------------------------------- |
| Vector (semantic)   | 0.60           | Meaning similarity via embeddings           |
| Full-text (keyword) | 0.25           | Exact/stemmed keyword matches               |
| Importance (decay)  | 0.15           | How important and recently used a memory is |

Each signal is normalized to `[0, 1]` before weighting.

### Recall boost behavior

Every time a memory appears in search results (`recall`, `start_session` top memories), its `access_count` increments and `importance` is recalculated and persisted.

- Memories retrieved frequently stay relevant
- Memories never searched gradually fade
- Context window naturally fills with what matters most

### Tuning

```toml
[memory]
auto_decay = true          # enable/disable the decay system
decay_factor = 0.9         # 0.9 = 10% loss per interval
decay_interval = "168h"    # 1 week between decay steps
vector_weight = 0.60       # semantic search weight
fts_weight = 0.25          # keyword search weight
importance_weight = 0.15   # importance weight in scoring
access_boost_factor = 0.3  # points per recall
access_boost_cap = 2.0     # max recall boost
```

Set `importance_weight = 0` to disable importance scoring. Set `auto_decay = false` to keep static importance values.

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

### Directory structure

```
agentmem/
├── cmd/agentmem/        # Entry point, CLI flags
├── internal/
│   ├── compressor/      # Memory extraction (Claude API, Ollama, local rules)
│   ├── config/          # TOML configuration loading
│   ├── embedding/       # Vector generation (OpenAI, Ollama, local ONNX)
│   ├── mcp/             # MCP server + tool handlers
│   ├── memory/          # Core types, CRUD, search, spaced repetition
│   └── redis/           # Connection pool, index management
├── test/
│   ├── embedding/       # Black-box tests for embedding layer
│   └── memory/          # Black-box tests for store and search
├── config/              # Default config template
└── docs/                # Architecture, tools reference, setup, configuration
```

## Development

```bash
make help         # Show all available commands
make build        # Build binary
make test         # Run tests
make docker-up    # Start Redis Stack
make docker-down  # Stop Redis Stack
make redis-ui     # Open RedisInsight web UI (http://localhost:8001)
make clean        # Clean build artifacts
make install      # Install to $GOPATH/bin
```

### RedisInsight

Redis Stack includes RedisInsight on port `8001` (`make redis-ui`).

- Browse memories (`mem:{ulid}` hashes)
- Inspect search index (`idx:memories`)
- Monitor queries in real-time (Profiler)
- Run Redis commands (Workbench)

### Debugging with LazyVim/nvim-dap

1. Install Delve: `go install github.com/go-delve/delve/cmd/dlv@latest`
2. Ensure Redis is running: `make docker-up`
3. Open a Go file: `nvim cmd/agentmem/main.go`
4. Set breakpoint: `<leader>db`
5. Start debugger: `<leader>dc` then select "Debug Package"

## License

[MIT](LICENSE)
