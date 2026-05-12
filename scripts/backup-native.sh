#!/usr/bin/env bash
# Backs up mnemoir Redis data by triggering BGSAVE and copying the data directory.
# Redis must be running. The backup captures both RDB and AOF files.
#
# Usage: backup-native.sh <output-dir>
set -euo pipefail

OUTPUT="${1:?Usage: backup-native.sh <output-dir>}"
DATA_DIR="$(cd "$(dirname "$0")/.." && pwd)/data"
PASSWORD="${MNEMOIR_REDIS_PASSWORD:?Set MNEMOIR_REDIS_PASSWORD env var}"
REDIS_CONTAINER="${MNEMOIR_REDIS_CONTAINER:-mnemoir-redis-1}"

redis_cli() {
  if command -v redis-cli &>/dev/null; then
    redis-cli -a "$PASSWORD" "$@"
  else
    docker exec "$REDIS_CONTAINER" redis-cli -a "$PASSWORD" "$@"
  fi
}

if [ ! -d "$DATA_DIR" ]; then
  echo "error: data directory not found: $DATA_DIR" >&2
  exit 1
fi

if [ -e "$OUTPUT" ]; then
  echo "error: output already exists: $OUTPUT" >&2
  exit 1
fi

echo "triggering BGSAVE..."
redis_cli BGSAVE > /dev/null

# Poll LASTSAVE until the background save completes.
BEFORE=$(redis_cli LASTSAVE)
echo "waiting for BGSAVE to complete..."
while true; do
  sleep 1
  AFTER=$(redis_cli LASTSAVE)
  if [ "$AFTER" -gt "$BEFORE" ]; then
    break
  fi
done

echo "copying data directory..."
mkdir -p "$(dirname "$OUTPUT")"
cp -r "$DATA_DIR" "$OUTPUT"

echo "backup complete: $OUTPUT"
