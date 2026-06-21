#!/usr/bin/env bash
# Remove mnemoir agent specs from OpenAI Codex CLI.
#
# Removes: ~/.codex/memory/reference_mnemoir.md
# Removes: "## Memory (mnemoir)" section from ~/.codex/AGENTS.md

set -euo pipefail

CODEX_DIR="${HOME}/.codex"
AGENTS_MD="${CODEX_DIR}/AGENTS.md"
MEMORY_FILE="${CODEX_DIR}/memory/reference_mnemoir.md"

# --- 1. Remove memory file ---
if [ -f "$MEMORY_FILE" ]; then
  rm -f "$MEMORY_FILE"
  echo "Removed $MEMORY_FILE"
else
  echo "No memory file found at $MEMORY_FILE"
fi

# --- 2. Remove pointer section from AGENTS.md ---
if [ -f "$AGENTS_MD" ] && grep -q '^## Memory (mnemoir)' "$AGENTS_MD"; then
  RESULT=$(awk '
    /^## Memory \(mnemoir\)/ {skip=1; next}
    skip && /^## / {skip=0}
    !skip {print}
  ' "$AGENTS_MD")

  printf '%s\n' "$RESULT" > "$AGENTS_MD"
  echo "Pointer section removed from $AGENTS_MD"
else
  echo "No mnemoir section found in $AGENTS_MD"
fi
