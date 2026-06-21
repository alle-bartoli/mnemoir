#!/usr/bin/env bash
# Register mnemoir as an MCP server with OpenAI Codex CLI.
# Usage: ./scripts/install-mcp-codex.sh [path/to/mnemoir-binary]
#
# Requires: codex CLI installed and MNEMOIR_REDIS_PASSWORD set.

set -euo pipefail

BINARY="${1:-$(go env GOPATH)/bin/mnemoir}"
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

codex mcp add mnemoir \
  --env "MNEMOIR_REDIS_PASSWORD=${MNEMOIR_REDIS_PASSWORD}" \
  -- "$BINARY" --config "${CONFIG_DIR}/config.toml"

echo "MCP server 'mnemoir' registered in $CODEX_CONFIG"
echo "  Binary: $BINARY"
echo "  Config: ${CONFIG_DIR}/config.toml"
