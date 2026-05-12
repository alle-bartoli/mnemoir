#!/usr/bin/env bash
# Restores mnemoir Redis data from a backup directory.
# Stops Redis, replaces the data directory, then restarts Redis.
#
# Usage: restore-native.sh <backup-dir>
set -euo pipefail

INPUT="${1:?Usage: restore-native.sh <backup-dir>}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DATA_DIR="$REPO_ROOT/data"

if [ ! -d "$INPUT" ]; then
  echo "error: backup directory not found: $INPUT" >&2
  exit 1
fi

echo "stopping Redis..."
docker compose -f "$REPO_ROOT/docker-compose.yml" stop redis

echo "replacing data directory..."
rm -rf "$DATA_DIR"
cp -r "$INPUT" "$DATA_DIR"

echo "starting Redis..."
docker compose -f "$REPO_ROOT/docker-compose.yml" start redis

echo "restore complete from: $INPUT"
