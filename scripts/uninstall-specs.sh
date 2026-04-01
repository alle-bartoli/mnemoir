#!/usr/bin/env bash
# Remove mnemoir agent specs from Claude Code auto-memory.
#
# Removes: ~/.claude/memory/reference_mnemoir.md
# Removes: "## Memory (mnemoir)" section from ~/.claude/CLAUDE.md
# Removes: index entry from ~/.claude/MEMORY.md

set -euo pipefail

CLAUDE_DIR="${HOME}/.claude"
CLAUDE_MD="${CLAUDE_DIR}/CLAUDE.md"
MEMORY_FILE="${CLAUDE_DIR}/memory/reference_mnemoir.md"
MEMORY_INDEX="${CLAUDE_DIR}/MEMORY.md"

# --- 1. Remove memory file ---
if [ -f "$MEMORY_FILE" ]; then
  rm -f "$MEMORY_FILE"
  echo "Removed $MEMORY_FILE"
else
  echo "No memory file found at $MEMORY_FILE"
fi

# --- 2. Remove pointer section from CLAUDE.md ---
if [ -f "$CLAUDE_MD" ] && grep -q '^## Memory (mnemoir)' "$CLAUDE_MD"; then
  RESULT=$(awk '
    /^## Memory \(mnemoir\)/ {skip=1; next}
    skip && /^## / {skip=0}
    !skip {print}
  ' "$CLAUDE_MD")

  printf '%s\n' "$RESULT" > "$CLAUDE_MD"
  echo "Pointer section removed from $CLAUDE_MD"
else
  echo "No mnemoir section found in $CLAUDE_MD"
fi

# --- 3. Remove index entry from MEMORY.md ---
if [ -f "$MEMORY_INDEX" ] && grep -q 'reference_mnemoir\.md' "$MEMORY_INDEX"; then
  grep -v 'reference_mnemoir\.md' "$MEMORY_INDEX" > "${MEMORY_INDEX}.tmp"
  mv "${MEMORY_INDEX}.tmp" "$MEMORY_INDEX"
  echo "Index entry removed from $MEMORY_INDEX"
else
  echo "No mnemoir index entry found in $MEMORY_INDEX"
fi
