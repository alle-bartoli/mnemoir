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
