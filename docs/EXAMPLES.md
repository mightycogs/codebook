# Usage Examples

### Index a project

```
index_repository(repo_path="/path/to/your/project")
```

### Get codebase architecture overview

```
get_architecture(aspects=["all"])
# -> languages, packages, entry points, routes, hotspots, boundaries, services, layers, clusters, file tree

get_architecture(aspects=["languages", "packages"])
# -> quick orientation -- just language breakdown and top packages

get_architecture(aspects=["hotspots", "boundaries", "clusters"])
# -> dependency analysis -- most-called functions, cross-package calls, community detection
```

### Manage Architecture Decision Records (ADR)

```
# Store a new ADR
manage_adr(mode="store", content="## PURPOSE\nOrder processing service\n\n## STACK\n- Go: speed\n- SQLite: embedded storage")

# Update specific sections (others preserved)
manage_adr(mode="update", sections={"PATTERNS": "- Pipeline pattern\n- Repository pattern"})

# Retrieve the full ADR with parsed sections
manage_adr(mode="get")

# View ADR via architecture overview
get_architecture(aspects=["adr"])

# Delete the ADR
manage_adr(mode="delete")
```

### Find all functions matching a pattern

Search is **case-insensitive by default** -- no need for `(?i)`:

```
search_graph(label="Function", name_pattern=".*handler")
# -> matches "Handler", "handler", "HANDLER", "RequestHandler", etc.

# Use regex alternatives for broad matching:
search_graph(name_pattern="auth|authenticate|authorization")

# Opt in to exact case matching when needed:
search_graph(name_pattern=".*Handler", case_sensitive=true)
```

### Search code (text search)

```
search_code(pattern="TODO")
# -> case-insensitive by default, matches "TODO", "Todo", "todo"

search_code(pattern="TODO|FIXME|HACK", regex=true)
# -> find all issue markers

search_code(pattern="TODO", case_sensitive=true)
# -> exact case match only
```

### Trace what a function calls

```
trace_call_path(function_name="ProcessOrder", depth=3, direction="outbound")
```

### Find what calls a function

```
trace_call_path(function_name="ProcessOrder", depth=2, direction="inbound")
```

### Risk-classified impact analysis

```
trace_call_path(function_name="ProcessOrder", direction="inbound", depth=3, risk_labels=true)
```

### Detect changes (git diff impact)

```
detect_changes()
detect_changes(scope="staged")
detect_changes(scope="branch", base_branch="main", depth=3)
```

### Dead code detection

```
search_graph(
  label="Function",
  relationship="CALLS",
  direction="inbound",
  max_degree=0,
  exclude_entry_points=true
)
```

### Cross-service HTTP calls

```
search_graph(label="Function", relationship="HTTP_CALLS", direction="outbound")
```

### Query all REST routes

```
search_graph(label="Route")
```

### Cypher queries

```
query_graph(query="MATCH (f:Function)-[:CALLS]->(g:Function) WHERE f.name = 'main' RETURN g.name, g.qualified_name LIMIT 20")
```

```
# Case-insensitive regex in Cypher (use (?i) flag):
query_graph(query="MATCH (f:Function) WHERE f.name =~ '(?i).*handler.*' RETURN f.name LIMIT 20")
```

```
query_graph(query="MATCH (a)-[r:HTTP_CALLS]->(b) RETURN a.name, b.name, r.url_path, r.confidence LIMIT 10")
```

### High fan-out functions (calling 10+ others)

```
search_graph(label="Function", relationship="CALLS", direction="outbound", min_degree=10)
```

### Scope queries to a single project

When multiple repositories are indexed, use `project` to avoid cross-project contamination:

```
search_graph(label="Function", name_pattern=".*Handler", project="my-api")
```

### Discover then trace (when you don't know the exact name)

`trace_call_path` requires an exact function name match. Use `search_graph` first to discover the correct name:

```
search_graph(label="Function", name_pattern=".*Order.*")
# -> finds "ProcessOrder", "ValidateOrder", etc.

trace_call_path(function_name="ProcessOrder", direction="inbound", depth=3)
```

### Paginate large result sets

All search tools support pagination. The response includes `total`, `has_more`, `limit`, and `offset`:

```
search_graph(label="Function", limit=50, offset=0)
# -> {total: 449, has_more: true, limit: 50, offset: 0, results: [...]}

search_graph(label="Function", limit=50, offset=50)
# -> next page
```
