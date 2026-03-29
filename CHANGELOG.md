# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased] - 2026-03-29 (Alessandro Bartoli)

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
