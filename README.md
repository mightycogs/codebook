# codebase-memory-mcp

<!-- BADGES_START -->
[![Tests](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/mightycogs/d1c003ef95d36236bf88a08fa3674396/raw/main_tests.json)](https://gist.github.com/mightycogs/d1c003ef95d36236bf88a08fa3674396)
[![Coverage](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/mightycogs/d1c003ef95d36236bf88a08fa3674396/raw/main_coverage.json)](https://gist.github.com/mightycogs/d1c003ef95d36236bf88a08fa3674396)
<!-- BADGES_END -->

<img src="docs/small_create_with_ai.png" style="float: left; margin: 0 15px 15px 0;" width="150">


> **This is a customized fork of [DeusData/codebase-memory-mcp](https://github.com/DeusData/codebase-memory-mcp)** -- an excellent project by the DeusData team. Full credit to the original authors for the architecture, tree-sitter integration, and the idea itself. This fork is tailored for internal use and is not recommended for general consumption. Changes from upstream: removed self-update mechanism for supply chain security, rebranded to mightycogs namespace, restructured documentation, removed setup scripts in favor of Makefile-based workflow.

Every time an AI agent explores your codebase, it burns thousands of tokens grepping through files, rebuilding the same understanding from scratch. This MCP server indexes your code into a persistent knowledge graph -- one query returns what would take dozens of file reads. 99% fewer tokens, sub-millisecond responses, survives session restarts.

Single Go binary. No Docker, no databases, no API keys.

## Getting Started

Requires Go 1.26+ and a C compiler (`xcode-select --install` on macOS).

```bash
git clone https://github.com/mightycogs/codebase-memory-mcp.git
cd codebase-memory-mcp
make install
codebase-memory-mcp install
```

Restart your editor. The `install` command auto-detects Claude Code, Codex CLI, Cursor, Windsurf, Gemini CLI, VS Code, and Zed -- registers the MCP server and installs task-specific skills.

Now open any project and say **"Index this project"**. That's it. The graph persists in `~/.cache/codebase-memory-mcp/` and auto-syncs when files change.

## Everyday Usage

Once indexed, just talk to your AI agent naturally. The graph tools are called automatically behind the scenes.

**"What calls ProcessOrder?"** -- traces inbound call chains across files and packages:

```
trace_call_path(function_name="ProcessOrder", direction="inbound", depth=3)
```

**"Find dead code"** -- functions with zero callers, excluding entry points:

```
search_graph(label="Function", relationship="CALLS", direction="inbound",
             max_degree=0, exclude_entry_points=true)
```

**"What changed and what might break?"** -- maps your git diff to affected symbols with risk labels:

```
detect_changes(scope="staged", depth=3)
```

**"Show me the architecture"** -- languages, packages, entry points, hotspots, clusters, all in one call:

```
get_architecture(aspects=["all"])
```

For complex structural queries, the agent writes Cypher on the fly:

```
query_graph(query="MATCH (a)-[r:HTTP_CALLS]->(b) WHERE r.confidence > 0.5
                   RETURN a.name, b.name, r.url_path ORDER BY r.confidence DESC")
```

Search is case-insensitive by default. The graph covers 64 languages -- from Python and Go to COBOL and Verilog.

## MCP Tools

12 tools exposed over MCP:

**Indexing**: `index_repository`, `list_projects`, `delete_project`

**Querying**: `search_graph`, `trace_call_path`, `detect_changes`, `query_graph`, `get_graph_schema`, `get_code_snippet`, `get_architecture`, `manage_adr`

**Text search**: `search_code` -- grep-like search within indexed files

All search tools support pagination (`limit`/`offset`) and return `has_more` and `total` counts. Full parameter reference: [docs/MCP_TOOLS.md](docs/MCP_TOOLS.md). Query examples: [docs/EXAMPLES.md](docs/EXAMPLES.md). Cypher syntax: [docs/CYPHER.md](docs/CYPHER.md).

## CLI Mode

Every tool works from the command line too -- no MCP client needed:

```bash
codebase-memory-mcp cli search_graph '{"name_pattern": ".*Handler.*"}'
codebase-memory-mcp cli trace_call_path '{"function_name": "main", "direction": "outbound"}'
codebase-memory-mcp cli --raw query_graph '{"query": "MATCH (f:Function) RETURN f.name LIMIT 5"}' | jq
```

See [docs/CLI.md](docs/CLI.md) for more.

## Graph Model

The graph stores `Function`, `Class`, `Module`, `Route`, `Package`, `File` and other node types connected by edges like `CALLS`, `HTTP_CALLS`, `IMPORTS`, `IMPLEMENTS`, `TESTS`, and `CONFIGURES`. Edge properties carry metadata -- HTTP calls have `confidence` scores, routes have `method` and `path`.

Full schema: [docs/GRAPH-MODEL.md](docs/GRAPH-MODEL.md)

## Excluding Files

Place a `.cgrignore` in your project root (one glob pattern per line) to skip directories or files during indexing. Common directories like `.git`, `node_modules`, `vendor`, `__pycache__`, `dist`, and `build` are always excluded.

## Reference

- [MCP tools reference](docs/MCP_TOOLS.md)
- [Query examples](docs/EXAMPLES.md)
- [Cypher subset](docs/CYPHER.md)
- [Graph model](docs/GRAPH-MODEL.md)
- [CLI usage](docs/CLI.md)
- [Troubleshooting](docs/TROUBLESHOOTING.md)
- [Benchmark (64 languages)](docs/BENCHMARK.md)
- [Contributing](docs/CONTRIBUTING.md)

## License

MIT
