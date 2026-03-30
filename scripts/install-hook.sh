#!/usr/bin/env bash
# Install the mnemoir SessionEnd hook into Claude Code settings.
# Usage: ./scripts/install-hook.sh [path/to/session-end-hook.sh]
#
# Merges the hook into ~/.claude/settings.json without overwriting existing config.
# Requires jq.

set -euo pipefail

HOOK_SCRIPT="${1:-$(cd "$(dirname "$0")" && pwd)/session-end-hook.sh}"
SETTINGS_FILE="${HOME}/.claude/settings.json"

if ! command -v jq &> /dev/null; then
  echo "ERROR: jq is required but not installed. Install it with: brew install jq (macOS) or apt install jq (Linux)"
  exit 1
fi

if [ ! -x "$HOOK_SCRIPT" ]; then
  echo "ERROR: Hook script not found or not executable: $HOOK_SCRIPT"
  exit 1
fi

# Create settings file if it doesn't exist
mkdir -p "$(dirname "$SETTINGS_FILE")"
if [ ! -f "$SETTINGS_FILE" ]; then
  echo '{}' > "$SETTINGS_FILE"
fi

# Check if mnemoir hook is already installed
if jq -e '.hooks.SessionEnd[]?.hooks[]? | select(.command | contains("session-end-hook.sh"))' "$SETTINGS_FILE" > /dev/null 2>&1; then
  echo "Hook already installed in $SETTINGS_FILE"
  exit 0
fi

# Build the new hook entry
HOOK_ENTRY=$(jq -n --arg cmd "$HOOK_SCRIPT" '{
  matcher: "",
  hooks: [{ type: "command", command: $cmd }]
}')

# Merge into settings: append to existing SessionEnd array or create it
UPDATED=$(jq --argjson entry "$HOOK_ENTRY" '
  .hooks //= {} |
  .hooks.SessionEnd //= [] |
  .hooks.SessionEnd += [$entry]
' "$SETTINGS_FILE")

echo "$UPDATED" > "$SETTINGS_FILE"
echo "Hook installed in $SETTINGS_FILE"
echo "  Script: $HOOK_SCRIPT"
