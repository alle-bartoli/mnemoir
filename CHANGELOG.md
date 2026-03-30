# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased] - 2026-03-30 (Alessandro Bartoli)

### Fixed

- `list_projects` did not return projects that only had sessions but no stored memories; `SaveSession` now registers the project in the `projects` SET
- `cleanupOrphans` removed projects from `projects` SET when they had 0 memories even if sessions still existed; now checks both `memCount == 0 AND sessCount == 0`
- `CleanupOrphans` test ghost session used score `1.0` (oldest), causing `pruneSessions` to remove it before `cleanupOrphans` could detect it; fixed with a future timestamp score

### Added

- `internal/redis/keys.go`: centralized Redis key prefix constants (`KeyPrefixMemory`, `KeyPrefixSession`, `KeyPrefixProjectSessions`, `KeyProjects`, `KeyPrefixMaintLastRun`, `KeyTagFrequency`)
- `internal/config/providers.go`: centralized provider identifiers, environment variable names, and default configuration values as shared constants
- `POST /end-session` HTTP endpoint on sideband server: accepts `{"observations": "...", "summary": "..."}`, delegates to existing `EndSession` handler, returns 404 if no active session
- `scripts/session-end-hook.sh`: Claude Code `SessionEnd` hook script that calls `/end-session` via curl for graceful session closure on `ctrl+c`
- `scripts/install-hook.sh`: idempotent installer that merges `SessionEnd` hook into `~/.claude/settings.json` via `jq`
- `scripts/uninstall-hook.sh`: removes the mnemoir hook entry from Claude Code settings
- `scripts/install-instructions.sh`: installs agent instructions into `~/.claude/CLAUDE.md`, replacing existing section or appending
- `scripts/uninstall-instructions.sh`: removes the mnemoir section from `~/.claude/CLAUDE.md`
- `make hook` target: installs the Claude Code `SessionEnd` hook
- `make instructions` target: installs agent instructions into global `CLAUDE.md`
- `StartHealthServer` now accepts variadic `extraRoutes` to register additional HTTP handlers alongside `/healthz`
- `NewServer` returns `*Handlers` alongside `*server.MCPServer` for sideband HTTP wiring
- `maint:last_run:{project}` now stores a Redis hash with `timestamp`, `forgotten_count`, `pruned_sessions`, `orphan_cleaned` instead of a plain timestamp
- `TestMaintenance/LastRunStats` integration test: verifies hash type, all fields present, non-zero timestamp, and positive TTL
- `make mcp-register-global` target: registers MCP server with `-s user` scope so it is available in all Claude Code projects
- Explicit `-s local` flag on existing `mcp-register` target for clarity

### Changed

- Replaced all scattered Redis key prefix strings across `memory/store.go`, `memory/search.go`, `memory/maintenance.go`, `redis/schema.go`, `compressor/local.go`, and test files with constants from `redis/keys.go`
- Replaced inline `"idx:memories"` usage (8 occurrences in memory package) with `redis.IndexName`
- Replaced provider name strings in `embedding/embedder.go`, `compressor/compressor.go`, and `config/config.go` validation with typed constants from `config/providers.go`
- Replaced environment variable strings in `config/config.go`, `compressor/claude.go`, and `embedding/openai.go` with `config.Env*` constants
- Replaced default model/URL fallback values in provider constructors with `config.Default*` constants, eliminating duplication between `config.go` defaults and individual constructors
- Replaced raw `"fact"/"concept"/"narrative"` strings in `parseAggregateStats` and `computeStats` with typed `memory.Fact`/`memory.Concept`/`memory.Narrative` constants
- Replaced raw `"vector"/"fulltext"/"hybrid"` enum values in `mcp/server.go` tool registration with typed `memory.Vector`/`memory.FullText`/`memory.Hybrid` constants
- Default `server.health_addr` changed from `""` (disabled) to `":9090"` (enabled) so the sideband HTTP server and session hook work out of the box
- `make setup` is now a full installation: docker + build + config + MCP global registration + SessionEnd hook + agent instructions in `~/.claude/CLAUDE.md`
- `make uninstall` now also removes the SessionEnd hook from Claude Code settings
- Renamed `mcp-register` to `mcp` and `mcp-register-global` to `mcp-global` for shorter CLI usage
- Rewrote `docs/agent-instructions.md` with comprehensive tool documentation: parameter tables for all 8 tools, search mode tradeoffs, `forget`/`update_memory` workflows, dedicated `end_session` section, utility tools, and continuous usage pattern
- README updated with both registration scopes (project-local vs global) in Quick Start, Claude Code section, and Development commands

## [Unreleased] - 2026-03-29 (Alessandro Bartoli)

### Added

- Automatic maintenance system: runs on `start_session`, throttled by `min_run_interval` to avoid redundant work
- Auto-forget: deletes memories with effective importance <= `forget_threshold` that haven't been accessed in `forget_inactive_days`; uses Lua script to compute effective importance server-side in a single round-trip per batch
- Session pruning: keeps only `max_sessions_per_project` most recent sessions, deletes older session hashes and sorted set entries
- Orphan cleanup: removes stale entries from `project_sessions:{project}` sorted set (pointing to deleted sessions) and removes project from `projects` SET when it has 0 memories
- `MaintenanceConfig` struct with `enabled`, `forget_threshold`, `forget_inactive_days`, `max_sessions_per_project`, `min_run_interval` fields
- `[maintenance]` section in `config/default.toml` with sane defaults
- `RunMaintenance`, `autoForget`, `pruneSessions`, `cleanupOrphans` methods on `Store`
- Maintenance stats included in `start_session` response when work was done
- Integration tests: auto-forget, session pruning, orphan cleanup, skip-when-recent, disabled config
- Config validation tests for all `MaintenanceConfig` fields

### Fixed

- `saveExtracted` always produced empty `session_id` on auto-extracted memories because `activeSession` was cleared before the call; now receives `sessionID` as parameter
- TOCTOU race in `StartSession`: two concurrent calls could both pass the nil-check and create duplicate sessions; `activeSession` is now set under the same lock, with rollback on save failure
- `DeleteByFilter` potential infinite loop when RediSearch index is stale; added `maxIterations` cap and early exit when no keys are actually deleted in a batch
- `saveExtracted` and `UpdateMemory` silently swallowed errors; now logged via `slog.Error` before returning generic user-facing messages
- Missing `^` (caret) in `escapeQueryText` RediSearch special characters, allowing prefix-match query injection; removed unnecessary `'` (single quote) escape
- `parseDimensionFromInfo` unreliable `fmt.Sprintf` string-parsing pass removed; now relies solely on typed structure walker (`parseDimensionNested`)
- `hashToSession` fragile offset-based prefix stripping replaced with `strings.TrimPrefix`
- `computeStats` legacy path divided `sumImportance` by `total_results` (which can exceed the 10k entry cap) instead of actual entry count, producing wrong `AvgImportance`
- Test helpers now read `MNEMOIR_REDIS_PASSWORD` from env or `.env` file, allowing integration tests to run against password-protected Redis
- `reVersion` regex required 3-part versions (`major.minor.patch`); now matches 2-part versions like `v1.0` via optional patch group

### Changed

- Project renamed from `agentmem` to `mnemoir` (mnemonic + memoir)
- Go module: `github.com/alle-bartoli/agentmem` to `github.com/alle-bartoli/mnemoir`
- Binary: `agentmem` to `mnemoir`
- Config directory: `~/.agentmem/` to `~/.mnemoir/`
- Log file: `agentmem.log` to `mnemoir.log`
- Env var: `AGENTMEM_REDIS_PASSWORD` to `MNEMOIR_REDIS_PASSWORD`
- MCP server name: `agentmem` to `mnemoir`
- Entry point directory: `cmd/agentmem/` to `cmd/mnemoir/`
- All documentation, README, and client registration examples updated

### Added

- Learned tag vocabulary: `LocalCompressor` reads tech keywords from Redis sorted set (`tags:frequency`) instead of a hardcoded list
- `seedTags()`: populates `tags:frequency` with default keywords (score 0) on first run, idempotent via `ZADD NX`
- `IncrementTags()`: increments tag frequency scores when memories are stored (both user-supplied and compressor-extracted)
- `loadVocabulary()`: reads learned tags from Redis sorted by frequency, falls back to defaults if Redis is unavailable
- `defaultTechKeywords` package-level variable replaces the inline hardcoded list, used as seed and fallback

### Fixed

- Makefile now loads `.env` file automatically via `-include .env` + `export`, so `MNEMOIR_REDIS_PASSWORD` check no longer falsely warns when the variable is set in `.env`
- `mcp-register` Makefile target now passes `MNEMOIR_REDIS_PASSWORD` via `-e` flag so the MCP server can authenticate with Redis
- MCP client registration examples (Claude Code, Cursor, Windsurf) now include the `env` block for `MNEMOIR_REDIS_PASSWORD`

## [Unreleased] - 2026-03-29 (Alessandro Bartoli)

### Added

- `make uninstall` target: removes binary from `$GOPATH/bin`, MCP registration, config directory (`~/.mnemoir`), and build artifacts
- End-to-end example in `docs/architecture.md` illustrating the full agent lifecycle (start session, store, recall, end session, next session)
- Auto-summarize implementation: `end_session` now generates a summary from extracted memories when none is provided and `session.auto_summarize` is enabled
- `buildAutoSummary()` helper that produces a summary with memory counts and up to 3 key points
- `IEmbedder.Close()` method for proper resource cleanup (ONNX session, HTTP clients)
- `Config.Validate()` method for fail-fast config validation at load time (checks providers, dimension, decay params, weight sums, importance range)
- `validateTags()` per-tag validation against TAG allowlist in `StoreMemory` and `UpdateMemory`
- Atomic `UpdateAccess` via Redis Lua script (replaces non-atomic read-modify-write pipeline)
- `/healthz` sideband HTTP endpoint (configurable via `server.health_addr`) exposing Redis connectivity status
- Structured logging with `log/slog` (JSON output to `~/.mnemoir/mnemoir.log`), replaces all `log.Printf` calls
- `slogWriter` bridge to redirect standard `log` package output into slog
- Security section in `docs/architecture.md` covering input validation, injection prevention, network safety, cryptography, and concurrency
- Search internals documented in `docs/tools-reference.md` (vector, fulltext, hybrid merge algorithm)
- Local compressor classification rules documented in `docs/configuration.md`
- `test/config/config_test.go`: 14 unit tests for `Config.Validate()` covering all invariants (providers, dimension, decay, weights, importance, session)
- `TestValidateTagValue` and `TestValidMemoryType` pure unit tests (no Redis)
- `TestStore/SessionSaveAndGetLast`, `GetLastSessionNoSessions`, `GetStats`, `GetTopMemories` integration tests
- `TestHybridSearchSingleAccessCount` verifying single access increment per unique result
- `make clean-data` target: stops Redis and wipes `./data/` to reclaim disk space
- MCP client registration instructions in README for Claude Code, Cursor, Windsurf, Continue.dev, Cline, and Zed
- RedisInsight Workbench query examples in README (search, filter, aggregate)
- TODO section in README with roadmap items (multi-session, CI tests, cross-project recall, export/import, auto-forget)
- `docs/agent-instructions.md`: ready-to-copy prompt block for `CLAUDE.md` or equivalent system prompts, covering session lifecycle, when to store/recall/forget/update, importance guidelines, and example flow

### Changed

- `start_session` now respects `session.max_recall_items` config instead of hardcoded limit of 10
- `end_session` checks `session.auto_summarize` config before compressing observations
- Fixed hybrid search weights in `docs/architecture.md` (was 0.7/0.3, now correctly 0.60/0.25/0.15)
- Go version requirement updated from 1.21+ to 1.25+ across all docs (README, setup, implementation guide)
- Session settings comments updated in `docs/configuration.md` to reflect actual behavior
- `HybridSearch` now uses raw search methods internally to prevent double `access_count` increment on overlapping results
- `GetLastSession` replaced O(N) SCAN with O(log N) sorted set lookup (`project_sessions:{project}`)
- `GetStats` uses `FT.AGGREGATE` for server-side computation (no 10k result cap), falls back to legacy FT.SEARCH
- `SaveSession` now indexes sessions in a sorted set for fast latest-session retrieval
- Graceful shutdown removes `os.Exit(0)` from signal handler, defers now execute properly (embedder close, Redis close)
- `UpdateMemory` validates content length, tags, importance range, and ULID format
- `Recall` limit parameter clamped server-side to `[1, 100]` regardless of schema validation
- Error response in `UpdateMemory` sanitized to not leak internal details
- Replaced all `log.Printf`/`log.Fatalf` with structured `slog` calls across all packages
- Test cleanup helpers now remove `session:test-*` keys and `project_sessions:{project}` sorted sets
- Redis persistence switched from Docker named volume to local bind mount (`./data:/data`) for portability
- `data/` added to `.gitignore`
- `start_session` now rejects with an explicit error if a session is already active (prevents silent override)

### Security

- Hardcoded `changeme` Redis password removed from `config/default.toml` and `docker-compose.yml`
- `docker-compose.yml`: Redis fails to start without `MNEMOIR_REDIS_PASSWORD` env var (was silent fallback)
- `Makefile`: config directory created with `0700` permissions, config file with `0600`
- Docker: added `restart: unless-stopped`, memory limit (512MB), `maxmemory` policy, healthcheck
- Per-tag validation in `StoreMemory` and `UpdateMemory` prevents TAG injection via comma-separated values

## [Unreleased] - 2026-03-28 (Alessandro Bartoli)

### Added

- Spaced repetition engine with lazy temporal decay and recall-based boosting
- `EffectiveImportance()` method on `Memory`: computes decayed importance + access boost, clamped to [1, 10]
- `MemoryConfig` fields: `vector_weight`, `fts_weight`, `importance_weight`, `access_boost_factor`, `access_boost_cap`
- Importance as a third scoring signal in `HybridSearch` (default weight 0.15)
- `UpdateAccess` now recalculates and persists importance using spaced repetition formula
- Unit tests for decay math (6 cases: no decay, 1/3 weeks, boost, boost cap, floor)
- Integration tests: `TestUpdateAccess_PersistsNewImportance`, `TestHybridSearch_ImportanceAffectsRanking`
- README section documenting spaced repetition: formula, decay examples, search weights, tuning guide

### Changed

- `NewStore` now accepts `config.MemoryConfig` for decay/boost parameters
- `mergeResults` signature extended with importance weight and decay parameters
- Moved all tests from `internal/` to `test/` directory mirroring module structure
- Tests converted to black-box (`package X_test`) using only exported API
- Grouped flat test functions under parent `t.Run` subtests (`TestStore/*`, `TestVectorSearch/*`, etc.)
- Shared fixtures extracted into `helpers_test.go` per test package

### Security

- Redis bound to `127.0.0.1` with `requirepass` via `MNEMOIR_REDIS_PASSWORD` env var
- TAG injection prevented via `ValidateTagValue` allowlist regex at handler boundary
- `os.ExpandEnv` replaced with allowlist (`expandAllowedEnv`) for `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `HOME`, `MNEMOIR_REDIS_PASSWORD`
- HTTP client timeout (30s) on all external API calls (OpenAI, Ollama, Anthropic)
- `sync.Mutex` on `activeSession` to prevent race conditions
- Input length validation (`content` 50KB, `query` 4KB, `project` 128, `tags` 1KB)
- FTS query injection: added missing escape chars (`\`, `"`, `'`, `$`, `#`) to `escapeQueryText`
- API error responses sanitized: full body logged internally, generic message returned
- Optional TLS on Redis connection via `redis.tls` config
- SSRF prevention: Ollama URL validated to localhost only
- Compression error details removed from session summary, logged internally
- ULID entropy switched from `math/rand` to `crypto/rand`
- Config and model directories created with `0700` instead of `0755`
- API response reads capped at 10MB via `io.LimitReader`
- `DeleteByFilter` loops in batches of 1000 instead of capping at 10,000

## [Unreleased] - 2026-03-25 (Alessandro Bartoli)

### Added

- `@dev` doc comments on `IEmbedder` interface and `NewEmbedder` factory with provider dimensions
- `@dev` doc comment on `MemoryType` explaining string alias trade-offs
- Test suite for local embedder:
  - dimension check
  - embedding output validation
  - cosine similarity between similar/unrelated texts
  - empty text handling
- Integration test suite for memory store:
  - Save/Get
  - Delete
  - UpdateAccess
  - Update
  - ListProjects
- Search integration tests:
  - VectorSearch semantic match
  - conceptual match,
  - FullTextSearch exact keyword
  - multi-word
  - HybridSearch combined results
  - type filter
- `@dev` doc comments on all search functions with explanations of scoring, RESP3 parsing, and merge algorithm
- RESP3 helper functions: `getResultEntries`, `getExtraAttributes`, `getMapString`, `getMapFloat`, `stripMemPrefix`

### Changed

- Promoted `hugot` from indirect to direct dependency in `go.mod`

### Fixed

- All `FT.SEARCH` result parsers assumed RESP2 flat array format (`[]any`), but go-redis uses RESP3 which returns `map[any]any` with `total_results`, `results`, `extra_attributes` structure. Rewrote `extractSearchResults`, `extractFTSResults`, `extractIDsFromSearch`, `extractTotalFromSearch`, `extractMemoriesFromSearch`, and `computeStats`.

## [Unreleased] - 2026-02-16 (Alessandro Bartoli)

### Added

- Init repo.
- Created scaffolding project.
- MCP server with 8 tools: `store_memory`, `recall`, `forget`, `update_memory`, `start_session`, `end_session`, `list_projects`, `memory_stats`
- Three memory types: `fact`, `concept`, `narrative`
- Hybrid search engine: vector (KNN/HNSW), full-text (RediSearch FTS), and weighted hybrid mode (0.7 vector + 0.3 FTS)
- Embedding providers: OpenAI (`text-embedding-3-small`, 1536d), Ollama (`nomic-embed-text`, 768d), local ONNX via hugot (`all-MiniLM-L6-v2`, 384d)
- Compressor providers: Claude API (`claude-haiku-4-5-20251001`), Ollama (`llama3.2`), local rule-based (regex pattern matching)
- Session management with automatic context loading (previous summary + top memories)
- Temporal decay: importance scores degrade over time, boosted on access (spaced repetition)
- Multi-project support with scoped memories and cross-project recall
- TOML configuration with environment variable expansion
- Redis Stack backend with RediSearch for vector and full-text indexing
- ULID-based IDs for chronological ordering
- Docker Compose setup for Redis Stack with integrated RedisInsight UI (port 8001)
- Zero API keys mode with `local` providers for both embedding and compression
- `make help` target to show all available Makefile commands
- `make redis-ui` target to open RedisInsight web UI in browser
- RedisInsight usage documentation in README and setup guide

### Fixed

- `EnsureIndex` failed with "Index already exists" when index was present. `parseDimensionNested` did not handle `map[any]any` type returned by go-redis `FT.INFO`, causing dimension check to fail and triggering a redundant `FT.CREATE`.
- Case-sensitive `DIM` match in index dimension parser. Redis returns lowercase `dim` but parser only matched uppercase `DIM`.
- Local embedder failed on `all-MiniLM-L6-v2` because the model ships multiple ONNX variants. Now explicitly selects `onnx/model.onnx` via `hugot.DownloadOptions.OnnxFilePath`.
- Default config had `dimension = 1536` while `provider = "local"` which produces 384d vectors. Fixed to `dimension = 384`.
