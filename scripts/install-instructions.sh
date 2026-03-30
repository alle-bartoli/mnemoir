#!/usr/bin/env bash
# Install mnemoir agent instructions into ~/.claude/CLAUDE.md.
# Replaces existing "## Memory (mnemoir)" section or appends if missing.
# Usage: ./scripts/install-instructions.sh [path/to/agent-instructions.md]

set -euo pipefail

INSTRUCTIONS_FILE="${1:-$(cd "$(dirname "$0")/.." && pwd)/docs/agent-instructions.md}"
CLAUDE_MD="${HOME}/.claude/CLAUDE.md"

if [ ! -f "$INSTRUCTIONS_FILE" ]; then
  echo "ERROR: Instructions file not found: $INSTRUCTIONS_FILE"
  exit 1
fi

# Extract content after the "---" separator (skip the preamble)
CONTENT=$(awk '/^---$/{found=1; next} found' "$INSTRUCTIONS_FILE")

if [ -z "$CONTENT" ]; then
  echo "ERROR: No content found after --- separator in $INSTRUCTIONS_FILE"
  exit 1
fi

mkdir -p "$(dirname "$CLAUDE_MD")"

if [ ! -f "$CLAUDE_MD" ]; then
  # No CLAUDE.md exists, just write the content
  printf '%s\n' "$CONTENT" > "$CLAUDE_MD"
  echo "Instructions written to $CLAUDE_MD"
  exit 0
fi

# Check if the section already exists
if grep -q '^## Memory (mnemoir)' "$CLAUDE_MD"; then
  # Replace: remove from "## Memory (mnemoir)" to the next "## " heading or EOF
  # Uses awk to rebuild the file
  BEFORE=$(awk '/^## Memory \(mnemoir\)/{exit} {print}' "$CLAUDE_MD")
  AFTER=$(awk '
    BEGIN {skip=0; found=0}
    /^## Memory \(mnemoir\)/ {skip=1; found=1; next}
    skip && /^## / {skip=0}
    !skip && found {print}
  ' "$CLAUDE_MD")

  {
    if [ -n "$BEFORE" ]; then
      printf '%s\n' "$BEFORE"
    fi
    printf '%s\n' "$CONTENT"
    if [ -n "$AFTER" ]; then
      printf '%s\n' "$AFTER"
    fi
  } > "$CLAUDE_MD"
  echo "Instructions updated in $CLAUDE_MD"
else
  # Append with a blank line separator
  printf '\n%s\n' "$CONTENT" >> "$CLAUDE_MD"
  echo "Instructions appended to $CLAUDE_MD"
fi
