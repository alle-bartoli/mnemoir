# Configuration

Config file location: `~/.mnemoir/config.toml`

Created by `task setup` or `task setup:codex` from `config/default.toml` if
it does not already exist.

## Redis

```toml
[redis]
addr = "localhost:6379"
password = "$MNEMOIR_REDIS_PASSWORD"
db = 0
tls = false
```

Set `MNEMOIR_REDIS_PASSWORD` when Redis requires authentication. Enable `tls` for
Redis connections that require TLS.

## Embedding Provider

Choose between OpenAI, Ollama, or local ONNX for vector embeddings.

```toml
[embedding]
provider = "local" # "openai" | "ollama" | "local"
dimension = 384
# OpenAI: 1536, Ollama/nomic-embed-text: 768, local: 384
```

### OpenAI

Requires `OPENAI_API_KEY` environment variable.

```toml
[embedding.openai]
model = "text-embedding-3-small"
```

### Ollama

Requires Ollama running locally with the model pulled.

```toml
[embedding.ollama]
url = "http://localhost:11434"
model = "nomic-embed-text"
```

When using Ollama, set `dimension = 768`.

### Local (ONNX)

Runs an ONNX model locally via [hugot](https://github.com/knights-analytics/hugot).
No API key or embedding service is needed. The model downloads from HuggingFace
on first use, so initial setup requires network access.

```toml
[embedding.local]
model = "sentence-transformers/all-MiniLM-L6-v2"
model_dir = "~/.mnemoir/models"
```

When using local, set `dimension = 384`. Pure Go backend, no CGO required.

## Compressor Provider

Choose between Claude API, Ollama, or local rule-based extraction for memory compression.

```toml
[compressor]
provider = "local" # "claude" | "ollama" | "local"
```

### Claude

Requires `ANTHROPIC_API_KEY` environment variable.

```toml
[compressor.claude]
model = "claude-haiku-4-5-20251001"
```

### Ollama

```toml
[compressor.ollama]
url = "http://localhost:11434"
model = "llama3.2"
```

### Local (Rule-Based)

Pure Go text analysis using regex pattern matching. No API keys or external services needed.

**Text splitting**: input is split into sentences by newlines, bullet points
(`-`, `*`, `\u2022`), and period-followed-by-uppercase patterns. Sentences shorter
than 10 characters are discarded.

**Classification** uses a scoring approach:

| Category       | Detection                                              | Importance |
| -------------- | ------------------------------------------------------ | ---------- |
| **Facts**      | Regex: paths, ports, versions, URLs, IPs, env vars     | 6-7        |
| **Concepts**   | 15 keywords (e.g. `pattern`, `architecture`, `design`) | 5-6        |
| **Narratives** | Everything else                                        | 4          |

A sentence with 2+ fact regex matches is classified as a fact. A sentence with
1+ concept keyword is a concept. A sentence with exactly 1 fact match and no
concept keywords is still a fact. Importance increases with more matches within
a category.

**Tag extraction**: technology keywords (40+ terms like `redis`, `docker`,
`kubernetes`, `jwt`), concept keywords, and capitalized proper nouns are
extracted as tags (max 5 per memory).

## Memory Settings

```toml
[memory]
default_importance = 5
auto_decay = true
decay_interval = "2160h" # 90 days
decay_factor = 0.88      # Multiplier per decay interval
vector_weight = 0.60
fts_weight = 0.25
importance_weight = 0.15
access_boost_factor = 0.1
access_boost_cap = 1.0
```

With these defaults, a memory's importance decays by 12% per 90-day interval.
The weights control hybrid search ranking and should sum to approximately 1.

## Session Settings

```toml
[session]
auto_summarize = true
max_recall_items = 30 # Max memories loaded by start_session, ranked by importance
```

Increasing `max_recall_items` adds more memory context to each new session.

## Environment Variables

| Variable                 | Required For         | Description       |
| ------------------------ | -------------------- | ----------------- |
| `OPENAI_API_KEY`         | OpenAI embeddings    | OpenAI API key    |
| `ANTHROPIC_API_KEY`      | Claude compressor    | Anthropic API key |
| `MNEMOIR_REDIS_PASSWORD` | Redis authentication | Redis password    |

Local embedding and compressor providers need no provider API keys. Redis may
still require `MNEMOIR_REDIS_PASSWORD`.

## Server and maintenance settings

```toml
[server]
health_addr = "127.0.0.1:9090" # Empty disables the sideband HTTP server

[maintenance]
enabled = true
forget_threshold = 2.0
forget_inactive_days = 730 # Two years without access before auto-forget eligibility
max_sessions_per_project = 200
min_run_interval = "1h"
```

Maintenance runs when `start_session` is called and is throttled per project by
`min_run_interval`. A memory is forgotten only when it is old enough and its
effective importance is at or below `forget_threshold`. Setting the threshold to
`0` disables automatic memory deletion because effective importance is clamped to
at least `1`.

## Changing Embedding Dimension

If you switch between providers, update `dimension` in config accordingly:

| Provider | Dimension |
| -------- | --------- |
| OpenAI   | 1536      |
| Ollama   | 768       |
| Local    | 384       |

The RediSearch index is automatically recreated on next startup if the configured
dimension differs from the existing index.

**Warning**: Existing stored vectors are not automatically regenerated when the
dimension changes. Treat this as a data migration: back up first, then rebuild or
recreate memories with the new dimension before relying on vector search.
