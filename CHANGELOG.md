# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased] - 2026-04-09 (Alessandro Bartoli)

### Fixed

- **`trackAccess` pipeline**: switched `updateAccessScript` from `.Run` to `.Eval`
  - `Script.Run` does EVALSHA then falls back to EVAL on NOSCRIPT, but the
    fallback cannot fire inside a `redis.Pipeline`: the batch ships in one shot
    with no per-command retry, so a cold Redis (fresh container, `SCRIPT FLUSH`,
    failover) lost every access-count update in the batch
  - `.Eval` always sends the script body; Redis caches it after first execution,
    so the extra bandwidth is negligible for fire-and-forget post-search tracking
  - Non-pipelined `UpdateAccess` path in `internal/memory/store.go` still uses
    `.Run` correctly; its EVALSHA→EVAL fallback works fine on a plain client
  - Fixes flaky `TestHybridSearchSingleAccessCount` and lost access counts after
    Redis restarts

## [Unreleased] - 2026-04-01 (Alessandro Bartoli)

### Changed

- **`make specs`**: renamed from `make instructions`
  - Installs to `~/.claude/memory/reference_mnemoir.md` (Claude Code auto-memory)
    instead of injecting into `~/.claude/CLAUDE.md`
  - Adds a minimal behavioral pointer in `CLAUDE.md` and an index entry in
    `MEMORY.md`
- **`scripts/install-specs.sh`**: renamed from `scripts/install-instructions.sh`
  - Writes the memory file with frontmatter
  - Adds the pointer section in `CLAUDE.md`
  - Adds the index entry in `MEMORY.md`
- **`scripts/uninstall-specs.sh`**: renamed from `scripts/uninstall-instructions.sh`
  - Removes the memory file, the `CLAUDE.md` pointer, and the `MEMORY.md` entry
- **`docs/agent-specs.md`**: renamed from `docs/agent-instructions.md`
  - Preamble updated to document the new installation target

## [Unreleased] - 2026-03-30 (Alessandro Bartoli)

### Added

- **`Store.CountAllByProject`**: fetches memory counts grouped by project in a
  single `FT.AGGREGATE` call
- **`internal/redis/keys.go`**: centralized Redis key prefix constants
  - `KeyPrefixMemory`, `KeyPrefixSession`, `KeyPrefixProjectSessions`
  - `KeyProjects`, `KeyPrefixMaintLastRun`, `KeyTagFrequency`
- **`internal/config/providers.go`**: centralized provider identifiers, env var
  names, and default configuration values as shared constants
- **`POST /end-session`**: sideband HTTP endpoint
  - Accepts `{"observations": "...", "summary": "..."}`
  - Delegates to the existing `EndSession` handler
  - Returns 404 if no active session
- **`scripts/session-end-hook.sh`**: Claude Code `SessionEnd` hook script
  - Calls `/end-session` via curl for graceful session closure on `ctrl+c`
- **`scripts/install-hook.sh`**: idempotent installer that merges the `SessionEnd`
  hook into `~/.claude/settings.json` via `jq`
- **`scripts/uninstall-hook.sh`**: removes the mnemoir hook entry from Claude
  Code settings
- **`scripts/install-instructions.sh`**: installs agent instructions into
  `~/.claude/CLAUDE.md`, replacing the existing section or appending
- **`scripts/uninstall-instructions.sh`**: removes the mnemoir section from
  `~/.claude/CLAUDE.md`
- **`make hook` target**: installs the Claude Code `SessionEnd` hook
- **`make instructions` target**: installs agent instructions into global
  `CLAUDE.md`
- **`StartHealthServer` variadic `extraRoutes`**: registers additional HTTP
  handlers alongside `/healthz`
- **`NewServer`**: now returns `*Handlers` alongside `*server.MCPServer` for
  sideband HTTP wiring
- **`maint:last_run:{project}`**: now stores a Redis hash instead of a plain
  timestamp
  - Fields: `timestamp`, `forgotten_count`, `pruned_sessions`, `orphan_cleaned`
- **`TestMaintenance/LastRunStats`**: integration test verifying hash type, all
  fields present, non-zero timestamp, and positive TTL
- **`make mcp-register-global` target**: registers the MCP server with `-s user`
  scope so it is available in all Claude Code projects
- **Explicit `-s local` flag**: added on the existing `mcp-register` target for
  clarity

### Changed

- **`trackAccess`**: pipelines all Lua script calls into a single Redis round-trip
  instead of N sequential calls per search result
- **`HybridSearch`**: runs vector and FTS searches concurrently via goroutines
  - Latency reduced from `embed + vector + fts` to `embed + max(vector, fts)`
- **`escapeTag`**: replaced custom byte-by-byte `replaceAll` with a package-level
  `strings.NewReplacer` (single allocation, reusable)
- **`ListProjects` handler**: replaced the N+1 `CountByProject` loop with a
  single `FT.AGGREGATE GROUPBY @project REDUCE COUNT 0` query
- **`buildSearchFilter` / `buildFilterQuery`**: now use `strings.Join` instead
  of manual string concatenation
- **`getExtraAttributes`**: fast-paths string type assertions before falling back
  to `fmt.Sprintf`, avoiding reflection on the common case
- **Pre-allocated result slices**: in `extractIDsFromSearch`,
  `extractMemoriesFromSearch`, `extractSearchResults`, `extractFTSResults`, and
  `getResultEntries`
- **`LocalEmbedder.Embed`**: added `sync.Mutex` to protect against the
  hugot/gomlx data race under concurrent access
  - Exposed by the new `HybridSearch` parallelism
- **Redis key prefixes centralized**: replaced scattered string literals with
  constants from `redis/keys.go`
  - Touched: `memory/store.go`, `memory/search.go`, `memory/maintenance.go`,
    `redis/schema.go`, `compressor/local.go`, and test files
- **`redis.IndexName`**: replaced inline `"idx:memories"` usage (8 occurrences
  in the memory package)
- **Provider name constants**: replaced string literals in
  `embedding/embedder.go`, `compressor/compressor.go`, and `config/config.go`
  validation with typed constants from `config/providers.go`
- **Environment variable constants**: replaced strings in `config/config.go`,
  `compressor/claude.go`, and `embedding/openai.go` with `config.Env*` constants
- **Default value constants**: replaced default model/URL fallbacks in provider
  constructors with `config.Default*` constants
  - Eliminates duplication between `config.go` defaults and individual
    constructors
- **Memory type constants**: replaced raw `"fact"` / `"concept"` / `"narrative"`
  strings in `parseAggregateStats` and `computeStats` with typed `memory.Fact` /
  `memory.Concept` / `memory.Narrative`
- **Search mode constants**: replaced raw `"vector"` / `"fulltext"` / `"hybrid"`
  strings in `mcp/server.go` tool registration with typed `memory.Vector` /
  `memory.FullText` / `memory.Hybrid`
- **`server.health_addr` default**: changed from `""` (disabled) to `":9090"`
  (enabled) so the sideband HTTP server and session hook work out of the box
- **`make setup`**: now a full installation
  - docker + build + config + MCP global registration + `SessionEnd` hook +
    agent instructions in `~/.claude/CLAUDE.md`
- **`make uninstall`**: now also removes the `SessionEnd` hook from Claude Code
  settings
- **`mcp-register` / `mcp-register-global` renamed**: to `mcp` / `mcp-global`
  for shorter CLI usage
- **`docs/agent-instructions.md`**: comprehensive rewrite
  - Parameter tables for all 8 tools
  - Search mode tradeoffs
  - `forget` / `update_memory` workflows
  - Dedicated `end_session` section
  - Utility tools and continuous usage pattern
- **README**: documents both registration scopes (project-local vs global)
  - Updated in Quick Start, Claude Code section, and Development commands

### Fixed

- **`list_projects`**: did not return projects that only had sessions but no
  stored memories
  - `SaveSession` now registers the project in the `projects` SET
- **`cleanupOrphans`**: removed projects from the `projects` SET when they had
  0 memories even if sessions still existed
  - Now checks both `memCount == 0 AND sessCount == 0`
- **`CleanupOrphans` test ghost session**: used score `1.0` (oldest), causing
  `pruneSessions` to remove it before `cleanupOrphans` could detect it
  - Fixed with a future timestamp score

## [Unreleased] - 2026-03-29 (Alessandro Bartoli)

### Added

- **Automatic maintenance system**: runs on `start_session`, throttled by
  `min_run_interval` to avoid redundant work
- **Auto-forget**: deletes memories with effective importance
  `<= forget_threshold` not accessed in `forget_inactive_days`
  - Uses a Lua script to compute effective importance server-side in a single
    round-trip per batch
- **Session pruning**: keeps only `max_sessions_per_project` most recent sessions
  - Deletes older session hashes and sorted set entries
- **Orphan cleanup**: removes stale entries from `project_sessions:{project}`
  sorted set (pointing to deleted sessions)
  - Also removes the project from the `projects` SET when it has 0 memories
- **`MaintenanceConfig` struct**: new fields
  - `enabled`, `forget_threshold`, `forget_inactive_days`,
    `max_sessions_per_project`, `min_run_interval`
- **`[maintenance]` section**: added in `config/default.toml` with sane defaults
- **Store methods**: `RunMaintenance`, `autoForget`, `pruneSessions`,
  `cleanupOrphans`
- **Maintenance stats**: included in the `start_session` response when work was
  done
- **Integration tests**: auto-forget, session pruning, orphan cleanup,
  skip-when-recent, disabled config
- **Config validation tests**: for all `MaintenanceConfig` fields
- **Learned tag vocabulary**: `LocalCompressor` now reads tech keywords from a
  Redis sorted set (`tags:frequency`) instead of a hardcoded list
  - `seedTags()`: populates `tags:frequency` with default keywords (score 0)
    on first run, idempotent via `ZADD NX`
  - `IncrementTags()`: increments tag frequency scores when memories are stored
    (both user-supplied and compressor-extracted)
  - `loadVocabulary()`: reads learned tags from Redis sorted by frequency, falls
    back to defaults if Redis is unavailable
  - `defaultTechKeywords` package-level variable replaces the inline hardcoded
    list, used as seed and fallback

### Changed

- **Project rename**: `agentmem` → `mnemoir` (mnemonic + memoir)
  - Go module: `github.com/alle-bartoli/agentmem` →
    `github.com/alle-bartoli/mnemoir`
  - Binary: `agentmem` → `mnemoir`
  - Config directory: `~/.agentmem/` → `~/.mnemoir/`
  - Log file: `agentmem.log` → `mnemoir.log`
  - Env var: `AGENTMEM_REDIS_PASSWORD` → `MNEMOIR_REDIS_PASSWORD`
  - MCP server name: `agentmem` → `mnemoir`
  - Entry point directory: `cmd/agentmem/` → `cmd/mnemoir/`
  - All documentation, README, and client registration examples updated

### Fixed

- **`saveExtracted` empty `session_id`**: auto-extracted memories always had an
  empty session id because `activeSession` was cleared before the call
  - Now receives `sessionID` as a parameter
- **`StartSession` TOCTOU race**: two concurrent calls could both pass the
  nil-check and create duplicate sessions
  - `activeSession` is now set under the same lock, with rollback on save
    failure
- **`DeleteByFilter` potential infinite loop**: triggered when the RediSearch
  index was stale
  - Added a `maxIterations` cap and early exit when no keys are actually
    deleted in a batch
- **Silent errors in `saveExtracted` and `UpdateMemory`**: now logged via
  `slog.Error` before returning generic user-facing messages
- **`escapeQueryText` missing `^`**: caret was absent from RediSearch special
  characters, allowing prefix-match query injection
  - Also removed the unnecessary `'` (single quote) escape
- **`parseDimensionFromInfo`**: removed the unreliable `fmt.Sprintf`
  string-parsing pass
  - Now relies solely on the typed structure walker (`parseDimensionNested`)
- **`hashToSession`**: fragile offset-based prefix stripping replaced with
  `strings.TrimPrefix`
- **`computeStats` wrong `AvgImportance`**: legacy path divided `sumImportance`
  by `total_results` (which can exceed the 10k entry cap) instead of the actual
  entry count
- **Test helpers**: now read `MNEMOIR_REDIS_PASSWORD` from env or `.env` file
  - Allows integration tests to run against password-protected Redis
- **`reVersion` regex**: required 3-part versions (`major.minor.patch`)
  - Now matches 2-part versions like `v1.0` via an optional patch group
- **Makefile `.env` loading**: now loads `.env` automatically via
  `-include .env` + `export`
  - `MNEMOIR_REDIS_PASSWORD` check no longer warns when the variable is set in
    `.env`
- **`mcp-register` password passthrough**: Makefile target now passes
  `MNEMOIR_REDIS_PASSWORD` via `-e` flag so the MCP server can authenticate
- **MCP client registration examples**: now include the `env` block for
  `MNEMOIR_REDIS_PASSWORD`
  - Claude Code, Cursor, Windsurf

## [Unreleased] - 2026-03-29 (Alessandro Bartoli)

### Added

- **`make uninstall` target**: removes binary from `$GOPATH/bin`, MCP
  registration, config directory (`~/.mnemoir`), and build artifacts
- **End-to-end lifecycle example**: added in `docs/architecture.md`
  - Start session, store, recall, end session, next session
- **Auto-summarize**: `end_session` now generates a summary from extracted
  memories when none is provided and `session.auto_summarize` is enabled
- **`buildAutoSummary()`**: helper producing a summary with memory counts and
  up to 3 key points
- **`IEmbedder.Close()`**: method added for proper resource cleanup
  - ONNX session, HTTP clients
- **`Config.Validate()`**: fail-fast config validation at load time
  - Checks providers, dimension, decay params, weight sums, importance range
- **`validateTags()`**: per-tag validation against the TAG allowlist in
  `StoreMemory` and `UpdateMemory`
- **Atomic `UpdateAccess`**: now a Redis Lua script
  - Replaces the non-atomic read-modify-write pipeline
- **`/healthz`**: sideband HTTP endpoint
  - Configurable via `server.health_addr`
  - Exposes Redis connectivity status
- **Structured logging**: `log/slog` with JSON output to
  `~/.mnemoir/mnemoir.log`
  - Replaces all `log.Printf` calls
- **`slogWriter`**: bridge redirecting standard `log` package output into `slog`
- **Security section**: added in `docs/architecture.md`
  - Covers input validation, injection prevention, network safety, cryptography,
    concurrency
- **Search internals documentation**: added in `docs/tools-reference.md`
  - Vector, fulltext, hybrid merge algorithm
- **Local compressor classification rules**: documented in
  `docs/configuration.md`
- **`test/config/config_test.go`**: 14 unit tests for `Config.Validate()`
  - Covers all invariants: providers, dimension, decay, weights, importance,
    session
- **Pure unit tests (no Redis)**: `TestValidateTagValue` and
  `TestValidMemoryType`
- **Store integration tests**: `TestStore/SessionSaveAndGetLast`,
  `GetLastSessionNoSessions`, `GetStats`, `GetTopMemories`
- **`TestHybridSearchSingleAccessCount`**: verifies single access increment per
  unique result
- **`make clean-data` target**: stops Redis and wipes `./data/` to reclaim disk
  space
- **MCP client registration instructions**: added in README
  - Claude Code, Cursor, Windsurf, Continue.dev, Cline, Zed
- **RedisInsight Workbench examples**: query examples in README
  - Search, filter, aggregate
- **TODO section**: added in README with roadmap items
  - Multi-session, CI tests, cross-project recall, export/import, auto-forget
- **`docs/agent-instructions.md`**: ready-to-copy prompt block for `CLAUDE.md`
  or equivalent system prompts
  - Covers session lifecycle, when to store/recall/forget/update, importance
    guidelines, and example flow

### Changed

- **`start_session`**: now respects `session.max_recall_items` config instead
  of the hardcoded limit of 10
- **`end_session`**: checks `session.auto_summarize` config before compressing
  observations
- **Hybrid search weights**: fixed in `docs/architecture.md`
  - Was 0.7/0.3, now correctly 0.60/0.25/0.15
- **Go version requirement**: updated from 1.21+ to 1.25+ across all docs
  - README, setup guide, implementation guide
- **Session settings comments**: updated in `docs/configuration.md` to reflect
  actual behavior
- **`HybridSearch`**: now uses raw search methods internally to prevent double
  `access_count` increments on overlapping results
- **`GetLastSession`**: replaced O(N) `SCAN` with O(log N) sorted set lookup
  (`project_sessions:{project}`)
- **`GetStats`**: uses `FT.AGGREGATE` for server-side computation (no 10k
  result cap), falls back to legacy `FT.SEARCH`
- **`SaveSession`**: now indexes sessions in a sorted set for fast
  latest-session retrieval
- **Graceful shutdown**: removed `os.Exit(0)` from the signal handler
  - Defers now execute properly (embedder close, Redis close)
- **`UpdateMemory`**: validates content length, tags, importance range, and
  ULID format
- **`Recall` limit**: clamped server-side to `[1, 100]` regardless of schema
  validation
- **`UpdateMemory` error response**: sanitized to not leak internal details
- **Structured logging rollout**: replaced all `log.Printf` / `log.Fatalf` with
  `slog` calls across all packages
- **Test cleanup helpers**: now remove `session:test-*` keys and
  `project_sessions:{project}` sorted sets
- **Redis persistence**: switched from Docker named volume to local bind mount
  (`./data:/data`) for portability
- **`.gitignore`**: added `data/`
- **`start_session`**: now rejects with an explicit error if a session is
  already active (prevents silent override)

### Security

- **Hardcoded `changeme` Redis password**: removed from `config/default.toml`
  and `docker-compose.yml`
- **`docker-compose.yml`**: Redis now fails to start without
  `MNEMOIR_REDIS_PASSWORD` (was a silent fallback)
- **`Makefile` permissions**: config directory created with `0700`, config file
  with `0600`
- **Docker hardening**: added `restart: unless-stopped`, memory limit (512MB),
  `maxmemory` policy, healthcheck
- **Per-tag validation**: in `StoreMemory` and `UpdateMemory`
  - Prevents TAG injection via comma-separated values

## [Unreleased] - 2026-03-28 (Alessandro Bartoli)

### Added

- **Spaced repetition engine**: lazy temporal decay and recall-based boosting
- **`EffectiveImportance()` on `Memory`**: computes decayed importance + access
  boost, clamped to `[1, 10]`
- **`MemoryConfig` fields**: `vector_weight`, `fts_weight`, `importance_weight`,
  `access_boost_factor`, `access_boost_cap`
- **Importance as third scoring signal**: in `HybridSearch` (default weight
  0.15)
- **`UpdateAccess`**: now recalculates and persists importance using the spaced
  repetition formula
- **Decay math unit tests**: 6 cases
  - No decay, 1 week, 3 weeks, boost, boost cap, floor
- **Integration tests**: `TestUpdateAccess_PersistsNewImportance`,
  `TestHybridSearch_ImportanceAffectsRanking`
- **README**: new section documenting spaced repetition
  - Formula, decay examples, search weights, tuning guide

### Changed

- **`NewStore`**: now accepts `config.MemoryConfig` for decay/boost parameters
- **`mergeResults`**: signature extended with importance weight and decay params
- **Test layout**: moved all tests from `internal/` to `test/` mirroring the
  module structure
- **Black-box tests**: converted to `package X_test` using only the exported API
- **Test grouping**: flat test functions grouped under parent `t.Run` subtests
  - `TestStore/*`, `TestVectorSearch/*`, etc.
- **Shared fixtures**: extracted into `helpers_test.go` per test package

### Security

- **Redis binding**: bound to `127.0.0.1` with `requirepass` via
  `MNEMOIR_REDIS_PASSWORD`
- **TAG injection prevention**: via `ValidateTagValue` allowlist regex at the
  handler boundary
- **Env var expansion allowlist**: `os.ExpandEnv` replaced with
  `expandAllowedEnv`
  - Allowed keys: `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `HOME`,
    `MNEMOIR_REDIS_PASSWORD`
- **HTTP client timeout (30s)**: on all external API calls
  - OpenAI, Ollama, Anthropic
- **`sync.Mutex` on `activeSession`**: prevents race conditions
- **Input length validation**: `content` 50KB, `query` 4KB, `project` 128
  bytes, `tags` 1KB
- **FTS query injection**: added missing escape chars to `escapeQueryText`
  - `\`, `"`, `'`, `$`, `#`
- **API error sanitization**: full body logged internally, generic message
  returned
- **Optional TLS on Redis**: via `redis.tls` config
- **SSRF prevention**: Ollama URL validated to localhost only
- **Compression errors**: details removed from session summary, logged
  internally
- **ULID entropy**: switched from `math/rand` to `crypto/rand`
- **Directory permissions**: config and model directories created with `0700`
  instead of `0755`
- **API response size cap**: 10MB via `io.LimitReader`
- **`DeleteByFilter` batching**: loops in batches of 1000 instead of capping at
  10,000

## [Unreleased] - 2026-03-25 (Alessandro Bartoli)

### Added

- **`@dev` doc comments**: on `IEmbedder` interface and `NewEmbedder` factory
  with provider dimensions
- **`@dev` doc comment on `MemoryType`**: explains the string alias trade-offs
- **Local embedder test suite**:
  - Dimension check
  - Embedding output validation
  - Cosine similarity between similar / unrelated texts
  - Empty text handling
- **Memory store integration tests**:
  - Save / Get
  - Delete
  - `UpdateAccess`
  - Update
  - `ListProjects`
- **Search integration tests**:
  - `VectorSearch` semantic match
  - Conceptual match
  - `FullTextSearch` exact keyword
  - Multi-word
  - `HybridSearch` combined results
  - Type filter
- **`@dev` doc comments on search functions**: scoring, RESP3 parsing, merge
  algorithm
- **RESP3 helper functions**: `getResultEntries`, `getExtraAttributes`,
  `getMapString`, `getMapFloat`, `stripMemPrefix`

### Changed

- **`hugot` dependency**: promoted from indirect to direct in `go.mod`

### Fixed

- **RESP3 result parsing**: all `FT.SEARCH` parsers assumed RESP2 flat array
  format (`[]any`), but go-redis uses RESP3 which returns `map[any]any` with
  `total_results`, `results`, and `extra_attributes` structure
  - Rewrote `extractSearchResults`, `extractFTSResults`, `extractIDsFromSearch`,
    `extractTotalFromSearch`, `extractMemoriesFromSearch`, and `computeStats`

## [Unreleased] - 2026-02-16 (Alessandro Bartoli)

### Added

- **Repository scaffolding**: initial commit and project layout
- **MCP server**: 8 tools registered
  - `store_memory`, `recall`, `forget`, `update_memory`
  - `start_session`, `end_session`, `list_projects`, `memory_stats`
- **Three memory types**: `fact`, `concept`, `narrative`
- **Hybrid search engine**: vector (KNN/HNSW), full-text (RediSearch FTS), and
  weighted hybrid mode (0.7 vector + 0.3 FTS)
- **Embedding providers**:
  - OpenAI `text-embedding-3-small` (1536d)
  - Ollama `nomic-embed-text` (768d)
  - Local ONNX via hugot `all-MiniLM-L6-v2` (384d)
- **Compressor providers**:
  - Claude API `claude-haiku-4-5-20251001`
  - Ollama `llama3.2`
  - Local rule-based (regex pattern matching)
- **Session management**: with automatic context loading
  - Previous summary + top memories
- **Temporal decay**: importance scores degrade over time, boosted on access
  (spaced repetition)
- **Multi-project support**: scoped memories and cross-project recall
- **TOML configuration**: with environment variable expansion
- **Redis Stack backend**: RediSearch for vector and full-text indexing
- **ULID-based IDs**: for chronological ordering
- **Docker Compose**: Redis Stack with integrated RedisInsight UI (port 8001)
- **Zero API keys mode**: with `local` providers for both embedding and
  compression
- **`make help` target**: shows all available Makefile commands
- **`make redis-ui` target**: opens RedisInsight web UI in the browser
- **RedisInsight documentation**: usage docs in README and setup guide

### Fixed

- **`EnsureIndex` "Index already exists"**: `parseDimensionNested` did not
  handle the `map[any]any` type returned by go-redis `FT.INFO`
  - Dimension check failed and triggered a redundant `FT.CREATE`
- **Case-sensitive `DIM` match**: index dimension parser only matched uppercase
  `DIM`
  - Redis returns lowercase `dim`
- **Local embedder ONNX variant selection**: `all-MiniLM-L6-v2` ships multiple
  ONNX variants
  - Now explicitly selects `onnx/model.onnx` via
    `hugot.DownloadOptions.OnnxFilePath`
- **Default dimension mismatch**: config had `dimension = 1536` while
  `provider = "local"` which produces 384d vectors
  - Fixed to `dimension = 384`
