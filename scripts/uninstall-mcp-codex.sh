#!/usr/bin/env bash
# Remove mnemoir MCP server registration from OpenAI Codex CLI.
# Usage: ./scripts/uninstall-mcp-codex.sh

set -euo pipefail

CODEX_CONFIG="${HOME}/.codex/config.toml"

if ! command -v codex &> /dev/null; then
  echo "codex CLI not found; skipping MCP removal"
  exit 0
fi

if [ ! -f "$CODEX_CONFIG" ] || ! grep -q '^\[mcp_servers\.mnemoir\]' "$CODEX_CONFIG" 2>/dev/null; then
  echo "No mnemoir MCP entry found in $CODEX_CONFIG"
  exit 0
fi

codex mcp remove mnemoir
echo "MCP server 'mnemoir' removed from $CODEX_CONFIG"
