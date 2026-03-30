# Agent Instructions

Copy the block below into your project's `CLAUDE.md` (or equivalent system prompt for other agents) to teach the agent how to use mnemoir.

---

## Memory (mnemoir)

You have access to a persistent memory system via MCP tools. Use it to retain knowledge across sessions.

### Session lifecycle

- Call `start_session` with the project name at the beginning of every conversation. It returns previous session summary and top memories as context.
- Call `end_session` before the conversation ends. Pass `observations` with raw notes about what happened (decisions made, bugs found, patterns discovered). The system extracts and classifies memories automatically. Pass `summary` only if you want to override the auto-generated one.

### When to store memories

Call `store_memory` for information worth remembering across sessions:

- **Facts** (`type: "fact"`): concrete data points. Ports, file paths, versions, environment variables, API endpoints, database schemas, dependency versions, configuration values.
- **Concepts** (`type: "concept"`): patterns, architecture decisions, design tradeoffs, conventions adopted, coding standards, why something was built a certain way.
- **Narratives** (`type: "narrative"`): what was done and why. Bug investigations, refactoring rationale, failed approaches and why they failed, deployment incidents.

Do NOT store: trivial information derivable from code, temporary debug output, file contents (read the file instead), information that changes every session.

Set `importance` deliberately:

- 7-9: critical decisions, recurring bugs, architectural constraints
- 4-6: useful context, common patterns, project conventions
- 1-3: minor notes, one-time observations

Use `tags` for discoverability: `"redis,config"`, `"auth,security,bug"`, `"api,design-decision"`.

### When to recall memories

Call `recall` **before starting any task**. This is not optional. Always check for prior context before doing work.

Recall before:

- Starting work on a feature (what was decided before?)
- Debugging (has this happened before?)
- Making architecture decisions (what constraints exist?)
- Touching unfamiliar code (what does the team know about it?)

#### Search modes

| Mode               | Use when                                              | Example                                         |
| ------------------ | ----------------------------------------------------- | ----------------------------------------------- |
| `hybrid` (default) | General queries, best overall results                 | `"authentication flow"`                         |
| `fulltext`         | Searching for a specific keyword, name, or error code | `"MNEMOIR_REDIS_PASSWORD"`, `"TOCTOU race"`     |
| `vector`           | Searching by concept or meaning, fuzzy match          | `"how does decay work"`, `"why we chose Redis"` |

#### Parameters

| Parameter     | Required | Default      | Description                               |
| ------------- | -------- | ------------ | ----------------------------------------- |
| `query`       | yes      | -            | Search text (max 4KB)                     |
| `project`     | no       | all projects | Scope results to a project                |
| `type`        | no       | all types    | Filter: `fact`, `concept`, or `narrative` |
| `search_mode` | no       | `hybrid`     | `vector`, `fulltext`, or `hybrid`         |
| `limit`       | no       | 10           | Results to return (1-100)                 |

Always pass `project` to scope results. Use `type` filter when you know what you are looking for (e.g. `type: "fact"` for configuration values).

### When to forget

Call `forget` to clean up outdated or duplicate memories. At least one parameter is required.

| Parameter    | Description                         | Example            |
| ------------ | ----------------------------------- | ------------------ |
| `id`         | Delete a specific memory by ULID    | `"01KMX7M123..."`  |
| `project`    | Delete all memories in a project    | `"old-project"`    |
| `older_than` | Delete memories older than duration | `"720h"` (30 days) |

Parameters can be combined: `project` + `older_than` deletes old memories from a specific project.

Note: `start_session` automatically runs maintenance (auto-forget stale low-importance memories, session pruning, orphan cleanup). You don't need to manually forget memories that would be cleaned up by decay.

### When to update

Call `update_memory` when a stored fact changes or needs correction. You need the memory ID, so `recall` first to find it.

| Parameter    | Required | Description                         |
| ------------ | -------- | ----------------------------------- |
| `id`         | yes      | Memory ULID (from `recall` results) |
| `content`    | no       | New content text                    |
| `importance` | no       | New importance (1-10)               |
| `tags`       | no       | New comma-separated tags            |

### When to end sessions

Call `end_session` **before every conversation ends**. This is not optional. If the user signals they are done (goodbye, thanks, closing the conversation), call `end_session` first.

| Parameter      | Required | Description                                                                  |
| -------------- | -------- | ---------------------------------------------------------------------------- |
| `observations` | no       | Raw notes about what happened: decisions, bugs found, patterns, changes made |
| `summary`      | no       | Override the auto-generated summary. Omit to let the system generate one.    |

Always pass `observations`. The system extracts and classifies memories (facts, concepts, narratives) from your observations automatically. This is how session knowledge persists without manually storing every detail.

A good `observations` value includes:

- What was changed and why
- Decisions made and their rationale
- Bugs found or fixed
- Files modified
- Anything surprising or non-obvious

```
end_session(observations: "Renamed Makefile targets mcp-register -> mcp,
  mcp-register-global -> mcp-global for shorter CLI. Updated README (3 spots)
  and CHANGELOG. User prefers short target names over verbose ones.")
```

### Utility tools

| Tool            | When to use                                                                                   |
| --------------- | --------------------------------------------------------------------------------------------- |
| `list_projects` | Discover which projects have stored memories                                                  |
| `memory_stats`  | Check memory health: total count, type distribution, avg importance. Pass `project` to scope. |

### Continuous usage pattern

Mnemoir is not a start/end-only tool. Use it throughout the conversation:

```
1. start_session(project: "my-app")
   -> reads previous summary, loads top memories

2. recall(query: "authentication flow", project: "my-app")
   -> finds relevant memories before starting work

3. [do the work]

4. store_memory(content: "Switched from JWT to session cookies because...",
                type: "concept", project: "my-app",
                tags: "auth,cookies,decision", importance: 7)

5. [do more work on a different topic]

6. recall(query: "database migrations", project: "my-app")
   -> check context before next task

7. [do the work]

8. store_memory(content: "Added index on users.email, query time dropped from 2s to 50ms",
                type: "narrative", project: "my-app",
                tags: "database,performance,migration", importance: 5)

9. end_session(observations: "Migrated auth from JWT to session cookies.
   Updated middleware in auth.go. Tests passing. Cookie domain set to
   .example.com for staging. Added DB index on users.email.")
   -> auto-extracts facts/concepts/narratives from observations
```

The key rule: **recall before every task, store after every meaningful change**. Do not batch everything to `end_session`. Store important decisions and findings as they happen so they are immediately available for recall in subsequent tasks within the same session.
