# CLI Mode

Every MCP tool can be invoked directly from the command line -- no MCP client needed. Useful for testing, scripting, CI pipelines, and quick one-off queries.

```bash
codebook cli <tool_name> [json_args]
```

By default, the CLI prints a **human-friendly summary**. Use `--raw` for full JSON output (same format the MCP server returns).

### Examples

```bash
# Index a repository
codebook cli index_repository '{"repo_path": "/path/to/repo"}'
# -> Indexed "repo": 1017 nodes, 2574 edges
# ->   db: ~/.codebook/repo.db

# List indexed projects
codebook cli list_projects
# -> 2 project(s) indexed:
# ->   my-api       1017 nodes, 2574 edges  (indexed 2026-02-26T18:10:24Z)
# ->   my-frontend   450 nodes,  312 edges  (indexed 2026-02-26T17:34:06Z)

# Search for functions
codebook cli search_graph '{"name_pattern": ".*Handler.*", "label": "Function"}'
# -> 5 result(s) found
# ->   [Function] HandleRequest  cmd/server/main.go:42

# Trace call paths
codebook cli trace_call_path '{"function_name": "Search", "direction": "both"}'
# -> Trace from "Search": 8 node(s), 8 edge(s), 2 hop(s)

# Run Cypher queries
codebook cli query_graph '{"query": "MATCH (f:Function) RETURN f.name LIMIT 5"}'
# -> 5 row(s) returned  [f.name]
# ->   main
# ->   HandleRequest

# View graph schema
codebook cli get_graph_schema

# No args needed for tools without required parameters
codebook cli list_projects

# Full JSON output for scripting
codebook cli --raw search_graph '{"label": "Function", "limit": 100}' | jq '.results[].name'

# List available tools
codebook cli --help
```

The CLI uses the same SQLite database as the MCP server (`~/.codebook/<project>.db`). No watcher is started in CLI mode -- each invocation is a single-shot operation.
