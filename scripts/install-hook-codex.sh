#!/usr/bin/env bash
# Install the mnemoir Stop hook into OpenAI Codex CLI config.
# Usage: ./scripts/install-hook-codex.sh [path/to/session-end-hook.sh]
#
# Appends a [[hooks.Stop]] block to ~/.codex/config.toml.
# The Stop event fires at the end of each Codex turn — closest analog to
# Claude Code's SessionEnd. The hook script exits 0 on 404 so it is a no-op
# when no mnemoir session is active.

set -euo pipefail

HOOK_SCRIPT="${1:-$(cd "$(dirname "$0")" && pwd)/session-end-hook.sh}"
CODEX_CONFIG="${HOME}/.codex/config.toml"

if [ ! -x "$HOOK_SCRIPT" ]; then
  echo "ERROR: Hook script not found or not executable: $HOOK_SCRIPT"
  exit 1
fi

# Resolve absolute path
HOOK_SCRIPT="$(cd "$(dirname "$HOOK_SCRIPT")" && pwd)/$(basename "$HOOK_SCRIPT")"

# Idempotency check
if [ -f "$CODEX_CONFIG" ] && grep -qF "$HOOK_SCRIPT" "$CODEX_CONFIG" 2>/dev/null; then
  echo "Hook already installed in $CODEX_CONFIG"
  exit 0
fi

mkdir -p "$(dirname "$CODEX_CONFIG")"
touch "$CODEX_CONFIG"

cat >> "$CODEX_CONFIG" << TOML

[[hooks.Stop]]
matcher = ""
[[hooks.Stop.hooks]]
type = "command"
command = "${HOOK_SCRIPT}"
TOML

echo "Hook installed in $CODEX_CONFIG"
echo "  Script: $HOOK_SCRIPT"
