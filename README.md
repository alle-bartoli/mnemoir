# Mnemoir

MCP server that gives AI coding agents long-term memory. Mnemoir runs over stdio and stores memories in a local Redis Stack instance.

- **Offline-first**: local ONNX embeddings and rule-based compression; no API keys required
- **Hybrid search**: semantic, keyword, and importance scoring
- **Session-aware**: automatic summaries and spaced-repetition recall
- **Multi-project**: memories are scoped by project

## Quick start

### Requirements

- [Go](https://go.dev/dl/) 1.25+
- [Docker](https://docs.docker.com/get-docker/) with Compose v2
- [Task](https://taskfile.dev/installation/) 3.x
- `jq` (used by setup hooks)
- Git; Windows users need [Git for Windows](https://git-scm.com/downloads)

Create a local `.env` with a strong Redis password, then run the setup for your client:

```bash
git clone https://github.com/alle-bartoli/mnemoir.git
cd mnemoir
echo 'MNEMOIR_REDIS_PASSWORD=your-secret' > .env
task setup
```

`task setup` starts Redis, builds Mnemoir, copies the default configuration, prewarms the local embedding model, and configures Claude Code when it is installed. For Codex CLI use `task setup:codex`.

For manual clients, use `~/.mnemoir/.env` instead of a repository `.env` when the client starts outside the repository directory.

> The default configuration is fully local and does not require API keys. Redis still runs locally through Docker.

## Supported clients

| Client | Setup | Session hook |
| --- | --- | --- |
| Claude Code | `task setup` | `SessionEnd` |
| OpenAI Codex CLI | `task setup:codex` | `Stop` |
| Claude Desktop, Pi, Cursor, Windsurf, Cline, Zed | [manual setup](docs/client-setup.md) | Manual `end_session` |

All MCP-compatible clients can use Mnemoir. Clients without a session hook should call `end_session` before the conversation ends.

## Configuration

The setup copies `config/default.toml` to `~/.mnemoir/config.toml`. Edit that file to change providers, model paths, Redis, scoring, or the sideband health server.

The default providers are:

| Provider | Default | API key |
| --- | --- | --- |
| Compressor | `local` | None |
| Embeddings | `local` | None |

See [client setup](docs/client-setup.md) for manual configuration and optional provider keys.

## Common commands

```bash
task setup             # Install for Claude Code
task setup:codex       # Install for Codex CLI
task prewarm           # Download the local embedding model
task test              # Run tests
task build:all         # Cross-compile supported platforms
task docker:up         # Start Redis Stack
task docker:down       # Stop Redis Stack
task backup            # Native Redis backup
task backup:json       # Portable JSON backup
task uninstall         # Remove installed artifacts
```

## Documentation

- [Client setup](docs/client-setup.md): manual MCP configuration and platform notes
- [Troubleshooting](docs/troubleshooting.md): common startup and installation problems
- [Backup and restore](docs/backup-restore.md): native and JSON backups
- [Search scoring and retention](docs/scoring.md): hybrid ranking, decay, and auto-forget
- [Agent specs](docs/agent-specs.md): instructions for AI agents using Mnemoir

## Development

```bash
task --list    # List all tasks
task test      # Run the test suite
task build     # Build for the current platform
```

## License

[MIT](LICENSE)
