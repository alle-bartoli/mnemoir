# Agent Instructions

Copy the block below into your project's `CLAUDE.md` (or equivalent system prompt for other agents) to teach the agent how to use agentmem.

---

## Memory (agentmem)

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

Call `recall` before:

- Starting work on a feature (what was decided before?)
- Debugging (has this happened before?)
- Making architecture decisions (what constraints exist?)
- Touching unfamiliar code (what does the team know about it?)

Use `search_mode: "hybrid"` (default) for best results. Use `"fulltext"` when searching for a specific keyword or name. Use `"vector"` when searching by concept or meaning.

Always pass `project` to scope results. Use `type` filter when you know what you are looking for (e.g. `type: "fact"` for configuration values).

### When to forget

Call `forget` to clean up:

- Outdated facts after a migration or version bump
- Memories about deleted features or abandoned approaches
- Duplicates or low-value clutter

### When to update

Call `update_memory` when:

- A fact changes (port number, dependency version, API endpoint)
- Importance should be adjusted after new context emerges
- Tags need correction

### Example flow

```
1. start_session(project: "my-app")
   -> reads previous summary, loads top memories

2. recall(query: "authentication flow", project: "my-app")
   -> finds relevant memories about auth

3. [do the work]

4. store_memory(content: "Switched from JWT to session cookies because...",
                type: "concept", project: "my-app",
                tags: "auth,cookies,decision", importance: 7)

5. end_session(observations: "Migrated auth from JWT to session cookies.
   Updated middleware in auth.go. Tests passing. Cookie domain set to
   .example.com for staging.")
   -> auto-extracts facts/concepts/narratives from observations
```
