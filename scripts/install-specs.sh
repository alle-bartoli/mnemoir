#!/usr/bin/env bash
# Install mnemoir agent specs into Claude Code auto-memory.
#
# Writes full specs to ~/.claude/memory/reference_mnemoir.md (with frontmatter).
# Adds a minimal behavioral pointer in ~/.claude/CLAUDE.md.
# Adds an index entry in ~/.claude/MEMORY.md.
#
# Usage: ./scripts/install-specs.sh [path/to/agent-specs.md]

set -euo pipefail

SPECS_FILE="${1:-$(cd "$(dirname "$0")/.." && pwd)/docs/agent-specs.md}"
CLAUDE_DIR="${HOME}/.claude"
CLAUDE_MD="${CLAUDE_DIR}/CLAUDE.md"
MEMORY_DIR="${CLAUDE_DIR}/memory"
MEMORY_FILE="${MEMORY_DIR}/reference_mnemoir.md"
MEMORY_INDEX="${CLAUDE_DIR}/MEMORY.md"

if [ ! -f "$SPECS_FILE" ]; then
  echo "ERROR: Specs file not found: $SPECS_FILE"
  exit 1
fi

# Extract content after the "---" separator (skip the preamble)
CONTENT=$(awk '/^---$/{found=1; next} found' "$SPECS_FILE")

if [ -z "$CONTENT" ]; then
  echo "ERROR: No content found after --- separator in $SPECS_FILE"
  exit 1
fi

# --- 1. Write full specs to memory file with frontmatter ---
mkdir -p "$MEMORY_DIR"

cat > "$MEMORY_FILE" << 'FRONTMATTER'
---
name: Mnemoir MCP memory system
description: Full reference for mnemoir MCP tools - session lifecycle, recall, store, forget, update, search modes
type: reference
---
FRONTMATTER

printf '%s\n' "$CONTENT" >> "$MEMORY_FILE"
echo "Specs written to $MEMORY_FILE"

# --- 2. Add minimal pointer in CLAUDE.md ---
POINTER_SECTION='## Memory (mnemoir)

Persistent memory via MCP tools (`mnemoir`). Full reference in `memory/reference_mnemoir.md`.

**Mandatory lifecycle (every conversation):**

1. `start_session(project)` at conversation start
2. `recall(query, project)` before every task
3. `store_memory(content, type, project, tags, importance)` after meaningful changes
4. `end_session(observations)` before conversation ends

Do not batch to `end_session`. Store important findings as they happen.'

mkdir -p "$(dirname "$CLAUDE_MD")"

if [ ! -f "$CLAUDE_MD" ]; then
  printf '%s\n' "$POINTER_SECTION" > "$CLAUDE_MD"
  echo "Pointer section written to $CLAUDE_MD"
elif grep -q '^## Memory (mnemoir)' "$CLAUDE_MD"; then
  # Replace existing section
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
    printf '%s\n' "$POINTER_SECTION"
    if [ -n "$AFTER" ]; then
      printf '%s\n' "$AFTER"
    fi
  } > "$CLAUDE_MD"
  echo "Pointer section updated in $CLAUDE_MD"
else
  printf '\n%s\n' "$POINTER_SECTION" >> "$CLAUDE_MD"
  echo "Pointer section appended to $CLAUDE_MD"
fi

# --- 3. Add index entry in MEMORY.md ---
INDEX_ENTRY="- [Mnemoir MCP memory system](memory/reference_mnemoir.md) - Session lifecycle, recall, store, forget, search modes"

if [ ! -f "$MEMORY_INDEX" ]; then
  printf '%s\n' "$INDEX_ENTRY" > "$MEMORY_INDEX"
  echo "Index entry written to $MEMORY_INDEX"
elif ! grep -q 'reference_mnemoir\.md' "$MEMORY_INDEX"; then
  printf '%s\n' "$INDEX_ENTRY" >> "$MEMORY_INDEX"
  echo "Index entry appended to $MEMORY_INDEX"
else
  echo "Index entry already present in $MEMORY_INDEX"
fi
