# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
- Docker Compose setup for Redis Stack
- Zero API keys mode with `local` providers for both embedding and compression
