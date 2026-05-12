#!/usr/bin/env bash
# Backs up mnemoir Redis data by triggering BGSAVE and copying the data directory.
# Redis must be running. The backup captures both RDB and AOF files.
#
# Usage: backup-native.sh <output-dir>
set -euo pipefail

OUTPUT="${1:?Usage: backup-native.sh <output-dir>}"
DATA_DIR="$(cd "$(dirname "$0")/.." && pwd)/data"
PASSWORD="${MNEMOIR_REDIS_PASSWORD:?Set MNEMOIR_REDIS_PASSWORD env var}"

if [ ! -d "$DATA_DIR" ]; then
  echo "error: data directory not found: $DATA_DIR" >&2
  exit 1
fi

if [ -e "$OUTPUT" ]; then
  echo "error: output already exists: $OUTPUT" >&2
  exit 1
fi

echo "triggering BGSAVE..."
redis-cli -a "$PASSWORD" BGSAVE > /dev/null

# Poll LASTSAVE until the background save completes.
BEFORE=$(redis-cli -a "$PASSWORD" LASTSAVE)
echo "waiting for BGSAVE to complete..."
while true; do
  sleep 1
  AFTER=$(redis-cli -a "$PASSWORD" LASTSAVE)
  if [ "$AFTER" -gt "$BEFORE" ]; then
    break
  fi
done

echo "copying data directory..."
cp -r "$DATA_DIR" "$OUTPUT"

echo "backup complete: $OUTPUT"
