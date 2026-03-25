# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
