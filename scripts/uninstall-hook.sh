#!/usr/bin/env bash
# Remove the mnemoir SessionEnd hook from Claude Code settings.
# Usage: ./scripts/uninstall-hook.sh

set -euo pipefail

SETTINGS_FILE="${HOME}/.claude/settings.json"

if ! command -v jq &> /dev/null; then
  echo "ERROR: jq is required but not installed."
  exit 1
fi

if [ ! -f "$SETTINGS_FILE" ]; then
  echo "No settings file found at $SETTINGS_FILE"
  exit 0
fi

# Check if hook exists
if ! jq -e '.hooks.SessionEnd[]?.hooks[]? | select(.command | contains("session-end-hook.sh"))' "$SETTINGS_FILE" > /dev/null 2>&1; then
  echo "No mnemoir hook found in $SETTINGS_FILE"
  exit 0
fi

# Remove entries whose command contains session-end-hook.sh
UPDATED=$(jq '
  .hooks.SessionEnd |= [.[] | select(.hooks | all(.command | contains("session-end-hook.sh") | not))] |
  if (.hooks.SessionEnd | length) == 0 then del(.hooks.SessionEnd) else . end |
  if (.hooks | length) == 0 then del(.hooks) else . end
' "$SETTINGS_FILE")

echo "$UPDATED" > "$SETTINGS_FILE"
echo "Hook removed from $SETTINGS_FILE"
