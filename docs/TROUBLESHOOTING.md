# Troubleshooting

| Problem | Cause | Fix |
|---------|-------|-----|
| `/mcp` doesn't show the server | Config not loaded or binary not found | Check `.mcp.json` path is absolute and correct. Restart Claude Code. Verify binary runs: `/path/to/codebase-memory-mcp` should output JSON. |
| `index_repository` fails | Missing `repo_path` or path doesn't exist | Pass an absolute path: `index_repository(repo_path="/absolute/path")` |
| `get_architecture` returns empty sections | No project indexed or project has few nodes | Run `index_repository` first. Some aspects (routes, hotspots, clusters) require enough graph data to produce meaningful results. |
| `get_code_snippet` returns "node not found" | Wrong qualified name format | Use `search_graph` first to find the exact `qualified_name`, then pass it to `get_code_snippet`. See [Qualified Names](GRAPH-MODEL.md#qualified-names). |
| `trace_call_path` returns 0 results | Exact name match -- no fuzzy matching | Use `search_graph(name_pattern=".*PartialName.*")` to discover the exact function name first. |
| Queries return results from wrong project | Multiple projects indexed, no filter | Add `project="your-project-name"` to `search_graph`. Use `list_projects` to see indexed project names. |
| Graph is missing recently added files | Auto-sync hasn't caught up yet, or project was never indexed | Wait a few seconds for auto-sync, or run `index_repository` manually. Auto-sync polls at 1--60s intervals depending on repo size. |
| `detect_changes` fails with "git not found" | git not installed or not on PATH | Install git. Required at runtime only for `detect_changes`. |
| Binary not found after install | `~/.local/bin` not on PATH | Add to your shell profile: `export PATH="$HOME/.local/bin:$PATH"` |
| Cypher query fails with parse error | Unsupported Cypher feature | See [Supported Cypher Subset](CYPHER.md). `WITH`, `COLLECT`, `OPTIONAL MATCH` are not supported. |
