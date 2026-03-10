# CLI Mode

Every MCP tool can be invoked directly from the command line -- no MCP client needed. Useful for testing, scripting, CI pipelines, and quick one-off queries.

```bash
codebase-memory-mcp cli <tool_name> [json_args]
```

By default, the CLI prints a **human-friendly summary**. Use `--raw` for full JSON output (same format the MCP server returns).

### Examples

```bash
# Index a repository
codebase-memory-mcp cli index_repository '{"repo_path": "/path/to/repo"}'
# -> Indexed "repo": 1017 nodes, 2574 edges
# ->   db: ~/.cache/codebase-memory-mcp/codebase-memory.db

# List indexed projects
codebase-memory-mcp cli list_projects
# -> 2 project(s) indexed:
# ->   my-api       1017 nodes, 2574 edges  (indexed 2026-02-26T18:10:24Z)
# ->   my-frontend   450 nodes,  312 edges  (indexed 2026-02-26T17:34:06Z)

# Search for functions
codebase-memory-mcp cli search_graph '{"name_pattern": ".*Handler.*", "label": "Function"}'
# -> 5 result(s) found
# ->   [Function] HandleRequest  cmd/server/main.go:42

# Trace call paths
codebase-memory-mcp cli trace_call_path '{"function_name": "Search", "direction": "both"}'
# -> Trace from "Search": 8 node(s), 8 edge(s), 2 hop(s)

# Run Cypher queries
codebase-memory-mcp cli query_graph '{"query": "MATCH (f:Function) RETURN f.name LIMIT 5"}'
# -> 5 row(s) returned  [f.name]
# ->   main
# ->   HandleRequest

# View graph schema
codebase-memory-mcp cli get_graph_schema

# No args needed for tools without required parameters
codebase-memory-mcp cli list_projects

# Full JSON output for scripting
codebase-memory-mcp cli --raw search_graph '{"label": "Function", "limit": 100}' | jq '.results[].name'

# List available tools
codebase-memory-mcp cli --help
```

The CLI uses the same SQLite database as the MCP server (`~/.cache/codebase-memory-mcp/codebase-memory.db`). No watcher is started in CLI mode -- each invocation is a single-shot operation.
