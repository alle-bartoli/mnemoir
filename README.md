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
- **9 MCP tools**:
  - `store_memory`,
  - `recall`,
  - `forget`,
  - `update_memory`,
  - `start_session`,
  - `end_session`,
  - `list_projects`,
  - `memory_stats`,
  - `rename_project`

## Prerequisites

| Dependency                                    | Version         | Purpose                     |
| --------------------------------------------- | --------------- | --------------------------- |
| [Go](https://go.dev/dl/)                      | 1.25+           | Build the binary            |
| [Docker](https://docs.docker.com/get-docker/) | with Compose v2 | Run Redis Stack             |
| [Task](https://taskfile.dev/installation/)    | 3.x             | Task runner (replaces Make) |
| [jq](https://jqlang.github.io/jq/download/)   | any             | Used by install scripts     |

**macOS / Linux (Homebrew)**

```bash
brew install go docker docker-compose go-task jq

# Start Docker Desktop (required for Redis)
open -a Docker
```

**Linux (apt)**

```bash
sudo apt update && sudo apt install -y golang docker.io docker-compose-v2 jq
sudo systemctl start docker

# Install Task
sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin
```

**Windows (PowerShell)**

```powershell
# winget
winget install GoLang.Go Docker.DockerDesktop Task.Task stedolan.jq

# or Chocolatey
choco install golang docker-desktop task jq

# or Scoop
scoop install go docker task jq

# Start Docker Desktop, then enable WSL 2 backend if prompted
```

> Windows requires Docker Desktop with WSL 2 backend for Redis Stack.

### Supported platforms

| Platform | Architecture          | Status           |
| -------- | --------------------- | ---------------- |
| macOS    | Apple Silicon (arm64) | Tested           |
| macOS    | Intel (amd64)         | Builds, untested |
| Linux    | x86_64 (amd64)        | Builds, untested |
| Windows  | x86_64 (amd64)        | Builds, untested |

### Client support

| Client                       | Setup              | MCP    | Hook         | Specs  |
| ---------------------------- | ------------------ | ------ | ------------ | ------ |
| Claude Code (CLI)            | `task setup`       | auto   | `SessionEnd` | auto   |
| OpenAI Codex CLI             | `task setup:codex` | auto   | `Stop`       | auto   |
| Claude Desktop               | manual             | manual | none         | manual |
| Pi                           | manual             | manual | none         | manual |
| Cursor, Windsurf, Cline, Zed | manual             | manual | none         | manual |

Clients without a hook require the agent to call `end_session` manually before the conversation ends.

## Installation

### 1. Clone and configure

```bash
git clone https://github.com/alle-bartoli/mnemoir.git
cd mnemoir

# Create .env to persist your Redis password across sessions
echo 'MNEMOIR_REDIS_PASSWORD=your-secret' > .env
```

Replace `your-secret` with any strong password. The `.env` file is gitignored.

### 2. Install the binary

```bash
task install
```

This builds the binary and installs it to `$(go env GOPATH)/bin/mnemoir`. Verify:

```bash
which mnemoir   # should print e.g. /Users/you/go/bin/mnemoir
```

### 3. Run setup for your AI client

#### Claude Code (CLI) - full support

```bash
task setup
```

Starts Redis, copies config to `~/.mnemoir/config.toml`, registers the MCP server globally, installs the `SessionEnd` hook, and installs agent specs.
The `SessionEnd` hook calls `/end-session` automatically when a conversation ends, so memories are never lost even if you forget to call `end_session`.

#### OpenAI Codex CLI - full support

```bash
task setup:codex
```

Same as above, targeting `~/.codex/config.toml` and `~/.codex/AGENTS.md`. Uses the `Stop` hook (fires per-turn) instead of `SessionEnd`.

#### Claude Desktop - manual setup, no hook

`task setup` does **not** cover Claude Desktop. Add it manually to the config file:

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "mnemoir": {
      "command": "/Users/you/go/bin/mnemoir",
      "args": ["--config", "/Users/you/.mnemoir/config.toml"],
      "env": { "MNEMOIR_REDIS_PASSWORD": "your-secret" }
    }
  }
}
```

Use absolute paths (no `~`).  
Replace `/Users/you` with your actual home directory (`echo $HOME`).  
Restart Claude Desktop after saving.

Claude Desktop has no session-end hook.  
The agent must call `end_session` manually before the conversation ends, or memories from that session will be lost.

#### Pi, Cursor, Windsurf, Cline, Zed, Continue.dev - manual setup, no hook

Any MCP-compatible client can use mnemoir. Add the same `mcpServers` JSON block above to your client's config file.

These clients lack a session-end hook.  
The agent must call `end_session` manually before the conversation ends.
Copy the agent specs from [docs/agent-specs.md](docs/agent-specs.md) (content after the `---` separator) into your agent's system prompt so it knows the lifecycle.

### Optional API keys

Only needed if switching from the default `local` providers:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."  # for Claude compressor
export OPENAI_API_KEY="sk-..."         # for OpenAI embeddings
```

## Troubleshooting

**macOS: "Failed to spawn process: Permission denied" in Claude Desktop**

macOS may quarantine the binary after download or build. Remove the quarantine flag:

```bash
xattr -dr com.apple.quarantine "$(go env GOPATH)/bin/mnemoir"
```

If that does not help, sign the binary ad-hoc:

```bash
codesign --force --deep --sign - "$(go env GOPATH)/bin/mnemoir"
```

Restart Claude Desktop after either fix.

**Binary not found after `task install`**

`$(go env GOPATH)/bin` must be on your `PATH`. Add to `~/.zshrc` or `~/.bashrc`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Then reload: `source ~/.zshrc`

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

Two complementary strategies are available.  
Use **native** for routine backups and disaster recovery; use **JSON** for portability, cross-host migrations, or human-readable snapshots.

### Native backup (recommended)

Captures the full Redis dataset (RDB + AOF) with a single atomic BGSAVE. Redis keeps running throughout.

```bash
# Backup — triggers BGSAVE, waits for completion, copies data/
# Default output: ~/.mnemoir/backups/YYYYMMDD
task backup

# Override output location
task backup OUTPUT=~/my-backups/$(date +%Y%m%d)

# Restore — stops Redis, replaces data/, restarts Redis
task restore INPUT=~/.mnemoir/backups/20260512
```

**When to use**: daily snapshots, disaster recovery, same-machine restores.

**Guarantees**: atomic snapshot (BGSAVE point-in-time), exact Redis state preserved including AOF, no data loss.

**Restore caveats**: Redis must be managed by the local Docker Compose. The `restore` target stops and restarts the container.

### JSON backup

Exports all memories, sessions, projects, and tag frequencies to a portable JSON file.
Embedding vectors are stored as base64 of the raw `float32` bytes, so no re-embedding is needed at restore time.

```bash
# Backup — writes indented JSON
# Default output: ~/.mnemoir/backups/YYYYMMDD.json
task backup:json

# Override output location
task backup:json OUTPUT=~/my-backups/snapshot.json

# Restore (additive — merges with existing data)
task restore:json INPUT=~/.mnemoir/backups/20260512.json

# Restore (clean — wipes DB first, then restores)
bin/mnemoir restore --flush --input ~/.mnemoir/backups/20260512.json \
  --config ~/.mnemoir/config.toml
```

**When to use**: cross-host migrations, human-readable snapshots, selective restores, environments where the Docker volume is not accessible.

**Guarantees**: embedding vectors are bit-exact (no re-inference). Restore validates the embedding dimension against the live RediSearch index before writing any data — a dimension mismatch returns a clear error with no partial writes. After restore, the command blocks until RediSearch finishes re-indexing (up to 2 minutes) and prints a warning if the timeout is reached.

**Restore caveats**: JSON backup is **not atomic** — concurrent writes during `backup-json` may produce an inconsistent snapshot. Stop the MCP server or use `task backup` if consistency is required. Without `--flush`, restore is additive (existing keys are overwritten on conflict but unrelated keys are left intact).

### Choosing between the two

|                             | Native (`task backup`) | JSON (`task backup:json`) |
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
task --list          # Show all targets
task build           # Build binary (current platform)
task build:all       # Cross-compile for all supported platforms
task test            # Run tests
task docker:up       # Start Redis Stack
task docker:down     # Stop Redis Stack
task redis:ui        # Open RedisInsight (http://localhost:8001)

# claude
task setup           # Full install (docker + build + config + MCP + hook + specs)
task mcp             # Register MCP (project-local)
task mcp:global      # Register MCP (all projects)
task hook            # Install SessionEnd hook
task specs           # Install agent specs into ~/.claude/memory/

# OpenAI Codex CLI
task setup:codex     # Full install for Codex CLI
task mcp:codex       # Register MCP with Codex CLI
task hook:codex      # Install Stop hook
task specs:codex     # Install agent specs into ~/.codex/AGENTS.md

task backup          # Hot backup Redis data dir via BGSAVE
task restore         # Cold restore Redis data dir (INPUT=path/to/dir)
task backup:json     # Dump memories to JSON
task restore:json    # Restore memories from JSON (INPUT=path/to/file.json)
task clean           # Remove build artifacts
task clean:data      # Stop Redis + wipe data/
task install         # Install to $GOPATH/bin
task uninstall       # Remove everything (binary, MCP, config, hook, specs)
```

Redis data persists in `./data/` (gitignored, capped at 512MB). Run `task clean:data` to reclaim disk space.

## Agent Specs

See [docs/agent-specs.md](docs/agent-specs.md) for the prompt block that teaches agents how to use mnemoir.

- **Claude Code**: installed automatically by `task setup`
- **Codex CLI**: installed automatically by `task setup:codex`
- **All other clients** (Pi, Claude Desktop, Cursor, etc.): copy the content after the `---` separator into your agent's system prompt manually

## License

[MIT](LICENSE)
