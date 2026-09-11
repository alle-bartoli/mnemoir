# Client setup

## Claude Code

For the full setup, run:

```bash
task setup
```

This configures the global MCP server, `SessionEnd` hook, and agent specs. 
If Claude Code is not installed, the client-specific steps are skipped.

## OpenAI Codex CLI

```bash
task setup:codex
```

This configures Codex's MCP server, `Stop` hook, and agent specs.

## Claude Desktop and generic MCP clients

Run `task install`, copy `config/default.toml` to `~/.mnemoir/config.toml`, and run `task prewarm`. 
Add this server block to the client's MCP configuration:

```json
{
  "mcpServers": {
    "mnemoir": {
      "command": "/absolute/path/to/mnemoir",
      "args": ["--config", "/absolute/path/to/.mnemoir/config.toml"],
      "env": { "MNEMOIR_REDIS_PASSWORD": "your-secret" }
    }
  }
}
```

Use absolute paths; `~` is not expanded by every MCP client. 
Clients without a session hook must call `end_session` manually.

## Pi

Pi can use the portable wrapper on Linux and macOS:

```bash
task docker:up
task install
task prewarm
task mcp:wrapper
pi install npm:pi-mcp-adapter
```

Add `~/.local/bin/mnemoir-mcp` to Pi's MCP configuration. 
The wrapper reads `MNEMOIR_REDIS_PASSWORD` from the environment or the `.env` file in `~/.mnemoir` (then the repository `.env`).

## Optional providers

Only configure API keys when changing from the default local providers:

```bash
export ANTHROPIC_API_KEY="..."  # Claude compressor
export OPENAI_API_KEY="..."     # OpenAI embeddings
```

## Platform notes

- macOS, Linux, and Windows amd64 builds are supported; macOS Apple Silicon is tested.
- Windows requires Docker Desktop with the WSL 2 backend and Git for Windows.
- On Windows, run Task from Git Bash or set `MNEMOIR_BASH` to the path of Git Bash.
