#!/usr/bin/env bash
# Claude Code SessionEnd hook for mnemoir.
# Calls the /end-session HTTP endpoint to gracefully close the active session.
#
# Configure in ~/.claude/settings.json or .claude/settings.json:
#   "hooks": {
#     "SessionEnd": [{
#       "matcher": "",
#       "hooks": [{ "type": "command", "command": "/path/to/scripts/session-end-hook.sh" }]
#     }]
#   }

MNEMOIR_PORT="${MNEMOIR_HEALTH_PORT:-9090}"
MNEMOIR_URL="http://localhost:${MNEMOIR_PORT}/end-session"

# Read hook input from stdin (contains session_id, reason, cwd, etc.)
INPUT=$(cat)
REASON=$(echo "$INPUT" | grep -o '"reason":"[^"]*"' | head -1 | cut -d'"' -f4)

# Send end-session request with reason as observation
curl -s -X POST "$MNEMOIR_URL" \
  -H "Content-Type: application/json" \
  -d "{\"observations\": \"Session closed by Claude Code hook (reason: ${REASON:-unknown})\"}" \
  > /dev/null 2>&1

# Always exit 0 so the hook never blocks Claude Code shutdown
exit 0
