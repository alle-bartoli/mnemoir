#!/usr/bin/env bash
# Install mnemoir agent specs into OpenAI Codex CLI.
#
# Writes full specs to ~/.codex/memory/reference_mnemoir.md.
# Adds a minimal behavioral pointer in ~/.codex/AGENTS.md.
#
# Usage: ./scripts/install-specs-codex.sh [path/to/agent-specs.md]

set -euo pipefail

SPECS_FILE="${1:-$(cd "$(dirname "$0")/.." && pwd)/docs/agent-specs.md}"
CODEX_DIR="${HOME}/.codex"
AGENTS_MD="${CODEX_DIR}/AGENTS.md"
MEMORY_DIR="${CODEX_DIR}/memory"
MEMORY_FILE="${MEMORY_DIR}/reference_mnemoir.md"

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

# --- 1. Write full specs to memory file ---
mkdir -p "$MEMORY_DIR"

cat > "$MEMORY_FILE" << 'FRONTMATTER'
# Mnemoir MCP memory system

Full reference for mnemoir MCP tools - session lifecycle, recall, store, forget, update, search modes.
FRONTMATTER

printf '%s\n' "$CONTENT" >> "$MEMORY_FILE"
echo "Specs written to $MEMORY_FILE"

# --- 2. Add minimal pointer in AGENTS.md ---
POINTER_SECTION='## Memory (mnemoir)

Persistent memory via MCP tools (`mnemoir`). Full reference in `memory/reference_mnemoir.md`.

**Mandatory lifecycle (every conversation):**

1. `start_session(project)` at conversation start
2. `recall(query, project)` before every task
3. `store_memory(content, type, project, tags, importance)` after meaningful changes
4. `end_session(observations)` before conversation ends

Do not batch to `end_session`. Store important findings as they happen.'

mkdir -p "$(dirname "$AGENTS_MD")"

if [ ! -f "$AGENTS_MD" ]; then
  printf '%s\n' "$POINTER_SECTION" > "$AGENTS_MD"
  echo "Pointer section written to $AGENTS_MD"
elif grep -q '^## Memory (mnemoir)' "$AGENTS_MD"; then
  BEFORE=$(awk '/^## Memory \(mnemoir\)/{exit} {print}' "$AGENTS_MD")
  AFTER=$(awk '
    BEGIN {skip=0; found=0}
    /^## Memory \(mnemoir\)/ {skip=1; found=1; next}
    skip && /^## / {skip=0}
    !skip && found {print}
  ' "$AGENTS_MD")

  {
    if [ -n "$BEFORE" ]; then
      printf '%s\n' "$BEFORE"
    fi
    printf '%s\n' "$POINTER_SECTION"
    if [ -n "$AFTER" ]; then
      printf '%s\n' "$AFTER"
    fi
  } > "$AGENTS_MD"
  echo "Pointer section updated in $AGENTS_MD"
else
  printf '\n%s\n' "$POINTER_SECTION" >> "$AGENTS_MD"
  echo "Pointer section appended to $AGENTS_MD"
fi
