# MCP Tools

12 tools exposed over the Model Context Protocol (MCP).

## Indexing

| Tool | Key Parameters | Description |
|------|---------------|-------------|
| `index_repository` | `repo_path` (required) | Index a repository into the graph. Only needed once -- auto-sync keeps it fresh after that. Supports incremental reindex via content hashing. |
| `list_projects` | -- | List all indexed projects with `indexed_at` timestamps and node/edge counts. |
| `delete_project` | `project_name` (required) | Remove a project and all its graph data. Irreversible. |

## Querying

| Tool | Key Parameters | Description |
|------|---------------|-------------|
| `search_graph` | `label`, `name_pattern`, `project`, `file_pattern`, `relationship`, `direction`, `min_degree`, `max_degree`, `exclude_entry_points`, `case_sensitive`, `limit` (default 100), `offset` | Structured search with filters. Case-insensitive by default (set `case_sensitive=true` for exact case). Use `project` to scope to a single repo when multiple are indexed. Supports pagination via `limit`/`offset` -- response includes `has_more` and `total`. |
| `trace_call_path` | `function_name` (required), `direction` (inbound/outbound/both), `depth` (1-5, default 3), `risk_labels` (boolean) | BFS traversal from/to a function (exact name match). Returns call chains with signatures, constants, and edge types. Capped at 200 nodes. With `risk_labels=true`, adds CRITICAL/HIGH/MEDIUM/LOW classification and `impact_summary`. |
| `detect_changes` | `scope` (unstaged/staged/all/branch), `base_branch`, `depth` (1-5, default 3) | Map git diff to affected graph symbols + blast radius. Returns changed files, changed symbols, and impacted callers with risk classification. Requires git in PATH. |
| `query_graph` | `query` (required) | Execute Cypher-like graph queries (read-only). String matching in WHERE is case-sensitive by default -- use `(?i)` flag for case-insensitive regex. See [CYPHER.md](CYPHER.md). |
| `get_graph_schema` | -- | Node/edge counts, relationship patterns, sample names. Run this first to understand what's in the graph. |
| `get_code_snippet` | `qualified_name` (required) | Read source code for a function by its qualified name (reads from disk). See [GRAPH-MODEL.md](GRAPH-MODEL.md#qualified-names) for the format. |
| `get_architecture` | `aspects` (array, default `["all"]`), `project` | Codebase architecture overview computed from the code graph. Aspects: `languages`, `packages`, `entry_points`, `routes`, `hotspots`, `boundaries`, `services`, `layers` (heuristic), `clusters` (Louvain community detection), `file_tree`, `adr` (stored Architecture Decision Record). Call with `["all"]` for full orientation. |
| `manage_adr` | `mode` (required: `get`/`store`/`update`/`delete`), `project`, `content`, `sections` | CRUD for Architecture Decision Records. `get`: retrieve ADR with parsed sections. `store`: create/replace full ADR (max 8000 chars). `update`: patch specific sections (unmentioned preserved). `delete`: remove ADR. Fixed sections: PURPOSE, STACK, ARCHITECTURE, PATTERNS, TRADEOFFS, PHILOSOPHY. |

## Text Search

File reading and directory listing are handled natively by your coding agent (Claude Code `Read` tool, Codex CLI `cat`/`ls`, etc.). This tool provides grep-like text search within indexed project files.

| Tool | Key Parameters | Description |
|------|---------------|-------------|
| `search_code` | `pattern` (required), `file_pattern`, `regex`, `case_sensitive`, `max_results` (default 100), `offset` | Grep-like text search within indexed project files. Case-insensitive by default (set `case_sensitive=true` for exact case). Supports pagination via `max_results`/`offset`. |
