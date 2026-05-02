# Mnemoir

MCP server that gives AI coding agents long-term memory.
Runs as a child process via stdio transport, backed by Redis Stack.
Fully offline-capable, no API keys required.

## Features

- **Offline-first**: local ONNX embeddings + rule-based compression, zero API keys
- **Hybrid search**: vector (KNN/HNSW) + full-text (TF-IDF) + importance scoring
- **Spaced repetition**: lazy temporal decay + recall boost, computed at query time
- **Typed memories**: `fact`, `concept`, `narrative` with automatic classification
- **Session management**: start/end sessions with automatic summarization
- **Multi-project**: scoped memories per project
- **8 MCP tools**: `store_memory`, `recall`, `forget`, `update_memory`, `start_session`, `end_session`, `list_projects`, `memory_stats`

## Prerequisites

- Go 1.25+
- Docker and Docker Compose
- `jq` (for hook installation)

> **Note**: tested only on macOS Tahoe 26.4 / Apple M1 Pro (arm64). Linux and other architectures may work but are untested.

## Quick Start

```bash
# Set Redis password
export MNEMOIR_REDIS_PASSWORD="your-secret"

# Full install: docker + build + config + MCP + hook + agent specs
make setup
```

This starts Redis, builds the binary, copies config to `~/.mnemoir/config.toml`, registers the MCP server globally with Claude Code, installs the `SessionEnd` hook into `~/.claude/settings.json`, and installs agent specs to `~/.claude/memory/reference_mnemoir.md` with a minimal pointer in `~/.claude/CLAUDE.md`.

**Note**:`make setup` registers mnemoir for the CLI only. The Desktop app reads its own config file. Add mnemoir to `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "mnemoir": {
      "command": "/path/to/bin/mnemoir",
      "args": ["--config", "~/.mnemoir/config.toml"],
      "env": { "MNEMOIR_REDIS_PASSWORD": "your-secret" }
    }
  }
}
```

Optional API keys (not needed with default `local` providers):

```bash
export ANTHROPIC_API_KEY="sk-ant-..."     # Only for Claude compressor
export OPENAI_API_KEY="sk-..."            # Only for OpenAI embeddings
```

Replace `/path/to/bin/mnemoir` with the actual binary path (`which mnemoir` after `make install`, or `$(pwd)/bin/mnemoir` for local builds).

## Other MCP Clients

For clients other than Claude Code, add the same JSON block above to your MCP config file.

Works with: Cursor, Windsurf, Continue.dev, Cline, Zed.

## Configuration

Edit `~/.mnemoir/config.toml`. Key settings:

| Key                        | Default | Description                                                  |
| -------------------------- | ------- | ------------------------------------------------------------ |
| `compressor.provider`      | `local` | `claude`, `ollama`, or `local`                               |
| `embedding.provider`       | `local` | `openai`, `ollama`, or `local`                               |
| `memory.vector_weight`     | `0.60`  | Semantic search weight in hybrid                             |
| `memory.fts_weight`        | `0.25`  | Keyword search weight in hybrid                              |
| `memory.importance_weight` | `0.15`  | Importance weight in hybrid                                  |
| `memory.decay_factor`      | `0.9`   | Decay per interval (0.9 = 10%/week)                          |
| `server.health_addr`       | `:9090` | Sideband HTTP (`/healthz`, `/end-session`), empty to disable |

See `~/.mnemoir/config.toml` for all options with inline comments.

## How Scoring Works

`recall` uses **hybrid search** by default: vector (semantic) and full-text (keyword) run in parallel, then results merge into a single ranked list.

### Hybrid merge formula

Each signal is normalized to [0, 1] then weighted:

```
final_score = (vec_score / max_vec)  * vector_weight       # default 0.60
            + (fts_score / max_fts)  * fts_weight          # default 0.25
            + (eff_importance / 10)  * importance_weight   # default 0.15
```

- **Vector score**: cosine similarity (`1.0 - cosine_distance`), via HNSW index.
- **FTS score**: RediSearch TF-IDF, normalized against the max score in the result set.
- **Effective importance**: see below.

If a memory appears in both vector and FTS results, its weighted scores are summed (deduplication by ID).

### Effective importance

Importance decays over time and gets boosted by recall frequency:

```
decayed   = importance * decay_factor ^ (time_since_access / decay_interval)
boost     = min(access_boost_cap, access_count * access_boost_factor)
effective = clamp(1.0, 10.0, decayed + boost)
```

Defaults: `decay_factor=0.9`, `decay_interval=7d`, `access_boost_factor=0.3`, `access_boost_cap=2.0`.

Example: importance 8, not accessed for 30 days, accessed 3 times:

- `decayed = 8 * 0.9^4.3 ≈ 4.2`
- `boost = min(2.0, 3 * 0.3) = 0.9`
- `effective = 5.1`

### Auto-forget

Maintenance runs periodically (default: once per hour per project).
Memories with `effective_importance <= 2.0` AND not accessed in 90+ days are automatically deleted.
Both thresholds are configurable via `maintenance.forget_threshold` and `maintenance.forget_inactive_days`.

## Development

```bash
make help           # Show all targets
make setup          # Full install (docker + build + config + MCP + hook + specs)
make build          # Build binary
make test           # Run tests
make docker-up      # Start Redis Stack
make docker-down    # Stop Redis Stack
make redis-ui       # Open RedisInsight (http://localhost:8001)
make mcp            # Register MCP (project-local)
make mcp-global     # Register MCP (all projects)
make hook           # Install SessionEnd hook
make specs          # Install agent specs into ~/.claude/memory/
make clean          # Remove build artifacts
make clean-data     # Stop Redis + wipe data/
make install        # Install to $GOPATH/bin
make uninstall      # Remove everything (binary, MCP, config, hook, specs)
```

Redis data persists in `./data/` (gitignored, capped at 512MB). Run `make clean-data` to reclaim disk space.

## Agent Specs

See [docs/agent-specs.md](docs/agent-specs.md) for the ready-to-copy prompt block that teaches agents how to use mnemoir. Installed automatically by `make setup` into `~/.claude/memory/reference_mnemoir.md` (Claude Code auto-memory), with a minimal behavioral pointer in `~/.claude/CLAUDE.md`.

## TODO

- [ ] Cross-project recall
- [ ] Memory export/import (JSON backup/restore)

## License

[MIT](LICENSE)
