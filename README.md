# Mnemoir

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
# Set Redis password
export MNEMOIR_REDIS_PASSWORD="your-secret"

# Full install: docker + build + config + MCP registration + SessionEnd hook
make setup
```

This runs everything: starts Redis, builds the binary, copies config to `~/.mnemoir/config.toml`, registers the MCP server globally with Claude Code, installs the `SessionEnd` hook for graceful session closure, and adds agent instructions to `~/.claude/CLAUDE.md`.

Edit `~/.mnemoir/config.toml` to customize providers and behavior.

Optional API keys (not needed with default `local` providers):

```bash
export ANTHROPIC_API_KEY="sk-ant-..."     # Only for Claude compressor
export OPENAI_API_KEY="sk-..."            # Only for OpenAI embeddings
```

## MCP Client Registration

Mnemoir works with any MCP-compatible coding agent via stdio transport.

### Claude Code

```bash
# Project-local (only available in current project)
make mcp

# Global (available in all projects)
make mcp-global
```

Or manually:

```bash
# Project-local (default)
claude mcp add mnemoir -s local -t stdio -e MNEMOIR_REDIS_PASSWORD="your-secret" -- /path/to/bin/mnemoir --config ~/.mnemoir/config.toml

# Global (user-wide)
claude mcp add mnemoir -s user -t stdio -e MNEMOIR_REDIS_PASSWORD="your-secret" -- /path/to/bin/mnemoir --config ~/.mnemoir/config.toml
```

### Cursor

Settings > MCP Servers > Add new server:

```json
{
  "mcpServers": {
    "mnemoir": {
      "command": "/path/to/bin/mnemoir",
      "args": ["--config", "~/.mnemoir/config.toml"],
      "env": {
        "MNEMOIR_REDIS_PASSWORD": "your-secret"
      }
    }
  }
}
```

### Windsurf

Settings > MCP > Add server:

```json
{
  "mcpServers": {
    "mnemoir": {
      "command": "/path/to/bin/mnemoir",
      "args": ["--config", "~/.mnemoir/config.toml"],
      "env": {
        "MNEMOIR_REDIS_PASSWORD": "your-secret"
      }
    }
  }
}
```

### Continue.dev

Add to `~/.continue/config.json`:

```json
{
  "mcpServers": [
    {
      "name": "mnemoir",
      "command": "/path/to/bin/mnemoir",
      "args": ["--config", "~/.mnemoir/config.toml"]
    }
  ]
}
```

### Cline (VS Code)

Settings > MCP Servers > Add:

```json
{
  "mcpServers": {
    "mnemoir": {
      "command": "/path/to/bin/mnemoir",
      "args": ["--config", "~/.mnemoir/config.toml"]
    }
  }
}
```

### Zed

Add to `~/.config/zed/settings.json`:

```json
{
  "language_models": {
    "mcp": {
      "mnemoir": {
        "command": "/path/to/bin/mnemoir",
        "args": ["--config", "~/.mnemoir/config.toml"]
      }
    }
  }
}
```

Replace `/path/to/bin/mnemoir` with the actual binary path (e.g. the output of `which mnemoir` after `make install`, or `$(pwd)/bin/mnemoir` for local builds).

## Configuration

Default configuration copied to `~/.mnemoir/config.toml` on first setup.

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
| `server.health_addr`         | `:9090` | Sideband HTTP address (`/healthz`, `/end-session`), empty to disable |

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
Client <--stdio/JSON-RPC--> mnemoir <--TCP--> Redis Stack
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
| Config     | TOML                        | `~/.mnemoir/config.toml`                                                                  |

### Directory structure

```
mnemoir/
├── cmd/mnemoir/        # Entry point, CLI flags
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
├── scripts/             # Hook scripts (Claude Code SessionEnd)
├── config/              # Default config template
└── docs/                # Architecture, tools reference, setup, configuration
```

## Development

```bash
make help                # Show all available commands
make setup               # Full install (docker + build + config + MCP + hook)
make build               # Build binary
make test                # Run tests
make docker-up           # Start Redis Stack
make docker-down         # Stop Redis Stack
make redis-ui            # Open RedisInsight web UI (http://localhost:8001)
make mcp                 # Register MCP server (project-local)
make mcp-global          # Register MCP server (all projects)
make hook                # Install Claude Code SessionEnd hook
make instructions        # Install agent instructions into ~/.claude/CLAUDE.md
make clean               # Clean build artifacts
make clean-data          # Stop Redis and wipe all stored memories (data/)
make install             # Install to $GOPATH/bin
make uninstall           # Remove binary, MCP registration, config, hook, and instructions
```

Redis data (RDB snapshots + AOF log) is persisted locally in `./data/`.
This directory is gitignored and stays small under normal usage (tens of MB for thousands of memories).
Redis is capped at 512MB via `maxmemory` with LRU eviction. Run `make clean-data` to reclaim disk space.

### RedisInsight

Redis Stack includes RedisInsight on port `8001` (`make redis-ui`).

- **Browser**: search `mem:*` to inspect memory hashes (content, type, project, tags, importance, timestamps)
- **Browser**: search `session:*` for session hashes, `project_sessions:*` for sorted sets
- **Profiler**: monitor queries from the MCP server in real-time
- **Workbench**: run queries directly

Useful Workbench queries:

```redis
-- All memories
FT.SEARCH idx:memories "*"

-- Filter by project
FT.SEARCH idx:memories "@project:{my-project}"

-- Filter by type
FT.SEARCH idx:memories "@type:{fact}"

-- Combined filters
FT.SEARCH idx:memories "@project:{my-project} @type:{concept}"

-- List all projects
SMEMBERS projects

-- Sessions for a project (most recent first)
ZREVRANGE project_sessions:my-project 0 -1 WITHSCORES

-- Memory count per project
FT.AGGREGATE idx:memories "*" GROUPBY 1 @project REDUCE COUNT 0 AS count
```

### Debugging with LazyVim/nvim-dap

1. Install Delve: `go install github.com/go-delve/delve/cmd/dlv@latest`
2. Ensure Redis is running: `make docker-up`
3. Open a Go file: `nvim cmd/mnemoir/main.go`
4. Set breakpoint: `<leader>db`
5. Start debugger: `<leader>dc` then select "Debug Package"

## Session Hook (Claude Code)

When a Claude Code session ends (including `ctrl+c`), the agent has no chance to call `end_session`. The included hook script solves this by calling the `/end-session` HTTP endpoint automatically.

### Setup

The hook is installed automatically by `make setup`. To install it separately:

```bash
make hook
```

This merges the `SessionEnd` hook into `~/.claude/settings.json` without overwriting existing settings. Idempotent (safe to run multiple times). Requires `jq`.

The hook sends a `POST` to `http://localhost:9090/end-session` with the exit reason as observation. Override the port with `MNEMOIR_HEALTH_PORT` env var if needed. Ensure `server.health_addr` is set in `~/.mnemoir/config.toml` (default: `":9090"`).

### HTTP Sideband Endpoints

When `server.health_addr` is configured, mnemoir exposes:

| Endpoint            | Method | Description                                    |
| ------------------- | ------ | ---------------------------------------------- |
| `/healthz`          | GET    | Redis connectivity check (200/503)             |
| `/end-session`      | POST   | Gracefully close active session via JSON body  |

`/end-session` accepts:

```json
{
  "observations": "what happened in the session",
  "summary": "optional override"
}
```

Returns 200 on success, 404 if no active session, 400 on invalid input.

## Agent Instructions

`make setup` automatically installs agent instructions into `~/.claude/CLAUDE.md`. To install or update them separately:

```bash
make instructions
```

See [docs/agent-instructions.md](docs/agent-instructions.md) for the full prompt block. You can also copy it manually into a project-level `CLAUDE.md` or equivalent system prompt for other agents.

## TODO

- [x] **Multi-session support**: each Claude Code instance spawns its own `mnemoir` process via stdio, so concurrent sessions across projects (or the same project) work naturally with no shared in-memory state
- [ ] **Cross-project recall**: search memories across all projects in a single query
- [ ] **Memory export/import**: dump and restore memories as JSON for backup or migration
- [x] **Auto-maintenance**: automatic cleanup on `start_session` (auto-forget stale low-importance memories, session pruning, orphan cleanup)

## License

[MIT](LICENSE)
