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
- **Spaced repetition**: lazy temporal decay + recall boost on importance, integrated into hybrid search scoring
- **Typed memories**: `fact`, `concept`, `narrative` with automatic classification
- **Hybrid search**: vector (KNN/HNSW) + full-text (TF-IDF) via RediSearch
- **MCP-native**: 8 tools via Model Context Protocol over stdio
- **Triple embedding**: OpenAI (`text-embedding-3-small`, 1536d), Ollama (`nomic-embed-text`, 768d), local ONNX (`all-MiniLM-L6-v2`, 384d)
- **Flexible compression**: Claude API, Ollama, or local rule-based extraction
- **Session management**: start/end sessions with automatic summarization from extracted memories
- **Multi-project**: scoped memories per project with cross-project recall

## Prerequisites

- Go 1.25 or higher
- Docker and Docker Compose
- Redis Stack 7.2+ (provided via `docker-compose.yml`)
- Environment variables:
  - `AGENTMEM_REDIS_PASSWORD` (Redis auth, default: `changeme`)
  - `ANTHROPIC_API_KEY` (optional, for Claude compressor)
  - `OPENAI_API_KEY` (optional, for OpenAI embeddings)
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

Configure environment variables:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."     # Required for Claude compressor
export OPENAI_API_KEY="sk-..."            # Required for OpenAI embeddings
export AGENTMEM_REDIS_PASSWORD="secret"   # Redis auth (default: changeme)
```

Edit `~/.agentmem/config.toml` to customize providers and behavior.

## Configuration

Default configuration copied to `~/.agentmem/config.toml` on first setup.

Key settings:

- `compressor.provider`: `claude`, `ollama`, or `local`
- `embedding.provider`: `openai`, `ollama`, or `local`
- `memory.auto_decay`: Enable temporal decay (default: `true`)
- `memory.decay_factor`: Multiplier per interval, e.g. `0.9` = 10% loss (default: `0.9`)
- `memory.decay_interval`: Time between decay steps (default: `168h` = 1 week)
- `memory.vector_weight`: Semantic search weight in hybrid scoring (default: `0.60`)
- `memory.fts_weight`: Keyword search weight in hybrid scoring (default: `0.25`)
- `memory.importance_weight`: Memory importance weight in hybrid scoring (default: `0.15`)
- `memory.access_boost_factor`: Points gained per recall (default: `0.3`)
- `memory.access_boost_cap`: Max boost from recalls (default: `2.0`)
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

## Spaced Repetition

Memories decay over time and get boosted on recall, just like human memory.
This is computed lazily at query time (no background goroutine, no overhead).

### How it works

Every memory has a base `importance` (1-10).
When you `recall` memories, the search engine computes an **effective importance** that factors in:

1. **Temporal decay**: importance drops by `decay_factor` (default 10%) per `decay_interval` (default 1 week)
2. **Access boost**: each recall adds `access_boost_factor` points (default 0.3), capped at `access_boost_cap` (default 2.0)
3. **Clamping**: result is always between 1 and 10

```
effective = base * decay_factor^intervals + min(boost_cap, access_count * boost_factor)
```

### Decay examples

| Scenario             | Importance | Weeks idle | Recalls | Effective |
| -------------------- | ---------- | ---------- | ------- | --------- |
| Fresh memory         | 8          | 0          | 0       | 8.0       |
| One week idle        | 8          | 1          | 0       | 7.2       |
| Three weeks idle     | 8          | 3          | 0       | 5.8       |
| Frequently recalled  | 8          | 3          | 5       | 7.3       |
| Old but heavily used | 5          | 8          | 10      | 4.1       |
| Minimum floor        | 3          | 20         | 0       | 1.0       |

### How it affects search

Hybrid search (`recall` with `search_mode: hybrid`) combines three signals:

| Signal              | Default weight | What it captures                            |
| ------------------- | -------------- | ------------------------------------------- |
| Vector (semantic)   | 0.60           | Meaning similarity via embeddings           |
| Full-text (keyword) | 0.25           | Exact/stemmed keyword matches               |
| Importance (decay)  | 0.15           | How important and recently used a memory is |

Each signal is normalized to [0, 1] before weighting.
A memory with high effective importance gets a scoring nudge that can push it above a semantically similar but forgotten memory.

### When recall boosts happen

Every time a memory appears in search results (`recall`, `start_session` top memories), its `access_count` increments and `importance` is recalculated and persisted. This means:

- Memories you keep finding stay relevant
- Memories you never search for gradually fade
- The agent's context window naturally fills with what matters most

### Tuning

All parameters are configurable in `~/.agentmem/config.toml` under `[memory]`:

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

Directory structure:

```
agentmem/
├── cmd/agentmem/        # Entry point, CLI flags
├���─ internal/
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
