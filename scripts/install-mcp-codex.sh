#!/usr/bin/env bash
# Register mnemoir as an MCP server with OpenAI Codex CLI.
# Usage: ./scripts/install-mcp-codex.sh [path/to/mnemoir-binary]
#
# Requires: codex CLI installed and MNEMOIR_REDIS_PASSWORD set.

set -euo pipefail

# go install appends .exe on Windows; match it so the default path resolves.
BIN_EXT=""
if [ "$(go env GOOS)" = "windows" ]; then
  BIN_EXT=".exe"
fi
BINARY="${1:-$(go env GOPATH)/bin/mnemoir${BIN_EXT}}"
CONFIG_DIR="${HOME}/.mnemoir"
CODEX_CONFIG="${HOME}/.codex/config.toml"

if ! command -v codex &> /dev/null; then
  echo "ERROR: codex CLI not found. Install it from https://github.com/openai/codex"
  exit 1
fi

if [ -z "${MNEMOIR_REDIS_PASSWORD:-}" ]; then
  echo "ERROR: MNEMOIR_REDIS_PASSWORD is not set. Set it in .env or export it."
  exit 1
fi

if [ ! -x "$BINARY" ]; then
  echo "ERROR: mnemoir binary not found or not executable: $BINARY"
  exit 1
fi

# Idempotency check
if [ -f "$CODEX_CONFIG" ] && grep -q '^\[mcp_servers\.mnemoir\]' "$CODEX_CONFIG" 2>/dev/null; then
  echo "MCP server 'mnemoir' already registered in $CODEX_CONFIG"
  exit 0
fi

# Pass the config path via env, not a flag: env values are immune to argument
# word-splitting when the home path contains spaces (e.g. "First Last" on
# Windows). Normalize MSYS paths (/c/Users/...) to native (C:/Users/...) so the
# Windows binary can open the file.
CONFIG_FILE="${CONFIG_DIR}/config.toml"
if command -v cygpath &> /dev/null; then
  CONFIG_FILE="$(cygpath -m "$CONFIG_FILE")"
fi

codex mcp add mnemoir \
  --env "MNEMOIR_REDIS_PASSWORD=${MNEMOIR_REDIS_PASSWORD}" \
  --env "MNEMOIR_CONFIG=${CONFIG_FILE}" \
  -- "$BINARY"

echo "MCP server 'mnemoir' registered in $CODEX_CONFIG"
echo "  Binary: $BINARY"
echo "  Config: ${CONFIG_FILE}"
