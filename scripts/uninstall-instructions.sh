#!/usr/bin/env bash
# Remove mnemoir agent instructions from ~/.claude/CLAUDE.md.
# Removes the "## Memory (mnemoir)" section, preserving everything else.

set -euo pipefail

CLAUDE_MD="${HOME}/.claude/CLAUDE.md"

if [ ! -f "$CLAUDE_MD" ]; then
  echo "No CLAUDE.md found at $CLAUDE_MD"
  exit 0
fi

if ! grep -q '^## Memory (mnemoir)' "$CLAUDE_MD"; then
  echo "No mnemoir section found in $CLAUDE_MD"
  exit 0
fi

# Remove from "## Memory (mnemoir)" to the next "## " heading or EOF
RESULT=$(awk '
  /^## Memory \(mnemoir\)/ {skip=1; next}
  skip && /^## / {skip=0}
  !skip {print}
' "$CLAUDE_MD")

printf '%s\n' "$RESULT" > "$CLAUDE_MD"
echo "Instructions removed from $CLAUDE_MD"
