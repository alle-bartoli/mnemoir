# Backup and restore

## Native backup

Use native backups for routine snapshots and disaster recovery. 
Redis stays online while BGSAVE creates the snapshot.

```bash
task backup
task backup OUTPUT=~/my-backups/$(date +%Y%m%d)
task restore INPUT=~/.mnemoir/backups/20260512
```

Restore stops and restarts the Redis container managed by the local Docker Compose file. 
The snapshot preserves the complete Redis state.

## JSON backup

Use JSON for portability, migrations, or human-readable exports. 
Embedding vectors are included, so restore does not re-infer them.

```bash
task backup:json
task backup:json OUTPUT=~/my-backups/snapshot.json
task restore:json INPUT=~/.mnemoir/backups/20260512.json

# Clean restore instead of additive restore
bin/mnemoir restore --flush --input snapshot.json --config ~/.mnemoir/config.toml
```

JSON backup is not atomic. Stop the MCP server when consistency matters. 
Restore is additive by default; `--flush` clears the database first.
