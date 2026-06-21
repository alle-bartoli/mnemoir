#!/usr/bin/env bash
# Remove the mnemoir Stop hook from OpenAI Codex CLI config.
# Usage: ./scripts/uninstall-hook-codex.sh

set -euo pipefail

CODEX_CONFIG="${HOME}/.codex/config.toml"

if [ ! -f "$CODEX_CONFIG" ]; then
  echo "No config file found at $CODEX_CONFIG"
  exit 0
fi

if ! grep -qF "session-end-hook.sh" "$CODEX_CONFIG" 2>/dev/null; then
  echo "No mnemoir hook found in $CODEX_CONFIG"
  exit 0
fi

# Remove the [[hooks.Stop]] block that references session-end-hook.sh.
# Deletes from [[hooks.Stop]] lines until the next top-level section or EOF,
# only when that block contains session-end-hook.sh.
RESULT=$(awk '
  /^\[\[hooks\.Stop\]\]/ {
    # Buffer this block
    buf = $0 "\n"
    while ((getline line) > 0) {
      if (line ~ /^\[/ && line !~ /^\[\[hooks\.Stop/) {
        # New section - flush buffer if it does not contain our hook
        if (buf !~ /session-end-hook\.sh/) {
          printf "%s", buf
        }
        buf = ""
        print line
        break
      }
      buf = buf line "\n"
    }
    # EOF reached - flush if not our hook
    if (buf != "" && buf !~ /session-end-hook\.sh/) {
      printf "%s", buf
    }
    next
  }
  { print }
' "$CODEX_CONFIG")

printf '%s\n' "$RESULT" > "$CODEX_CONFIG"
echo "Hook removed from $CODEX_CONFIG"
