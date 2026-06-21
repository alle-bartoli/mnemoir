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
- **9 MCP tools**: `store_memory`, `recall`, `forget`, `update_memory`, `start_session`, `end_session`, `list_projects`, `memory_stats`, `rename_project`

## Prerequisites

- Go 1.25+
- Docker and Docker Compose
- `jq` (for hook installation)

> **Note**: tested only on macOS Tahoe 26.4 / Apple M1 Pro (arm64). Linux and other architectures may work but are untested.

## Quick Start

```bash
# Set Redis password
export MNEMOIR_REDIS_PASSWORD="your-secret"

# claude: full install (docker + build + config + MCP + hook + agent specs)
make setup

# OpenAI Codex CLI: full install
make setup-codex
```

### claude

`make setup` starts Redis, builds the binary, copies config to `~/.mnemoir/config.toml`, registers the MCP server globally with claude, installs the `SessionEnd` hook into `~/.claude/settings.json`, and installs agent specs to `~/.claude/memory/reference_mnemoir.md` with a minimal pointer in `~/.claude/CLAUDE.md`.

**Note**: `make setup` registers mnemoir for the CLI only. The Desktop app reads its own config file. Add mnemoir to `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

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

### OpenAI Codex CLI

`make setup-codex` runs the same initial steps then:

- Registers the MCP server via `codex mcp add` (writes to `~/.codex/config.toml`)
- Installs a `Stop` hook (fires at the end of each turn) to close mnemoir sessions
- Installs agent specs to `~/.codex/memory/reference_mnemoir.md` with a minimal pointer in `~/.codex/AGENTS.md`

**Note**: Codex CLI uses `Stop` (per-turn) rather than a true session-end event. The hook is a no-op when no mnemoir session is active (returns 404, hook exits 0).

### Other MCP Clients

For clients other than claude and Codex CLI, add the server block to your MCP config file manually.

Works with: Cursor, Windsurf, Continue.dev, Cline, Zed.

Optional API keys (not needed with default `local` providers):

```bash
export ANTHROPIC_API_KEY="sk-ant-..."     # Only for Claude compressor
export OPENAI_API_KEY="sk-..."            # Only for OpenAI embeddings
```

Replace `/path/to/bin/mnemoir` with the actual binary path (`which mnemoir` after `make install`, or `$(pwd)/bin/mnemoir` for local builds).

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

## Backup & Restore

Two complementary strategies are available. Use **native** for routine backups and disaster recovery; use **JSON** for portability, cross-host migrations, or human-readable snapshots.

### Native backup (recommended)

Captures the full Redis dataset (RDB + AOF) with a single atomic BGSAVE. Redis keeps running throughout.

```bash
# Backup — triggers BGSAVE, waits for completion, copies data/
# Default output: ~/.mnemoir/backups/YYYYMMDD
make backup

# Override output location
make backup OUTPUT=~/my-backups/$(date +%Y%m%d)

# Restore — stops Redis, replaces data/, restarts Redis
make restore INPUT=~/.mnemoir/backups/20260512
```

**When to use**: daily snapshots, disaster recovery, same-machine restores.

**Guarantees**: atomic snapshot (BGSAVE point-in-time), exact Redis state preserved including AOF, no data loss.

**Restore caveats**: Redis must be managed by the local Docker Compose. The `restore` target stops and restarts the container.

### JSON backup

Exports all memories, sessions, projects, and tag frequencies to a portable JSON file. Embedding vectors are stored as base64 of the raw `float32` bytes, so no re-embedding is needed at restore time.

```bash
# Backup — writes indented JSON
# Default output: ~/.mnemoir/backups/YYYYMMDD.json
make backup-json

# Override output location
make backup-json OUTPUT=~/my-backups/snapshot.json

# Restore (additive — merges with existing data)
make restore-json INPUT=~/.mnemoir/backups/20260512.json

# Restore (clean — wipes DB first, then restores)
bin/mnemoir restore --flush --input ~/.mnemoir/backups/20260512.json \
  --config ~/.mnemoir/config.toml
```

**When to use**: cross-host migrations, human-readable snapshots, selective restores, environments where the Docker volume is not accessible.

**Guarantees**: embedding vectors are bit-exact (no re-inference). Restore validates the embedding dimension against the live RediSearch index before writing any data — a dimension mismatch returns a clear error with no partial writes. After restore, the command blocks until RediSearch finishes re-indexing (up to 2 minutes) and prints a warning if the timeout is reached.

**Restore caveats**: JSON backup is **not atomic** — concurrent writes during `backup-json` may produce an inconsistent snapshot. Stop the MCP server or use `make backup` if consistency is required. Without `--flush`, restore is additive (existing keys are overwritten on conflict but unrelated keys are left intact).

### Choosing between the two

|                             | Native (`make backup`) | JSON (`make backup-json`) |
| --------------------------- | ---------------------- | ------------------------- |
| Atomic snapshot             | Yes (BGSAVE)           | No                        |
| Redis must be local Docker  | Yes                    | No                        |
| Human-readable              | No                     | Yes                       |
| Cross-host migrate          | No                     | Yes                       |
| Embedding re-inference      | Not needed             | Not needed                |
| Selective restore           | No                     | Possible (edit JSON)      |
| Restore requires Redis stop | Yes                    | No                        |

## Development

```bash
make help           # Show all targets
make build          # Build binary
make test           # Run tests
make docker-up      # Start Redis Stack
make docker-down    # Stop Redis Stack
make redis-ui       # Open RedisInsight (http://localhost:8001)

# claude
make setup          # Full install (docker + build + config + MCP + hook + specs)
make mcp            # Register MCP (project-local)
make mcp-global     # Register MCP (all projects)
make hook           # Install SessionEnd hook
make specs          # Install agent specs into ~/.claude/memory/

# OpenAI Codex CLI
make setup-codex    # Full install for Codex CLI
make mcp-codex      # Register MCP with Codex CLI
make hook-codex     # Install Stop hook
make specs-codex    # Install agent specs into ~/.codex/AGENTS.md

make backup         # Hot backup Redis data dir via BGSAVE (default: ~/.mnemoir/backups/YYYYMMDD)
make restore        # Cold restore Redis data dir (INPUT=path/to/dir)
make backup-json    # Dump memories to JSON (default: ~/.mnemoir/backups/YYYYMMDD.json)
make restore-json   # Restore memories from JSON (INPUT=path/to/file.json)
make clean          # Remove build artifacts
make clean-data     # Stop Redis + wipe data/
make install        # Install to $GOPATH/bin
make uninstall      # Remove everything (binary, MCP, config, hook, specs)
```

Redis data persists in `./data/` (gitignored, capped at 512MB). Run `make clean-data` to reclaim disk space.

## Agent Specs

See [docs/agent-specs.md](docs/agent-specs.md) for the ready-to-copy prompt block that teaches agents how to use mnemoir. Installed automatically by `make setup` (claude) or `make setup-codex` (Codex CLI). For other clients, copy the content after the `---` separator into your agent's system prompt.

## TODO

- [x] Project rename
- [ ] Cross-project recall
- [x] Memory export/import (JSON backup/restore + native BGSAVE)

## License

[MIT](LICENSE)
