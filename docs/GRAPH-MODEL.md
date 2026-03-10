# Graph Data Model

### Node Labels

`Project`, `Package`, `Folder`, `File`, `Module`, `Class`, `Function`, `Method`, `Interface`, `Enum`, `Type`, `Route`

### Edge Types

`CONTAINS_PACKAGE`, `CONTAINS_FOLDER`, `CONTAINS_FILE`, `DEFINES`, `DEFINES_METHOD`, `IMPORTS`, `CALLS`, `HTTP_CALLS`, `ASYNC_CALLS`, `IMPLEMENTS`, `HANDLES`, `USAGE`, `CONFIGURES`, `WRITES`, `MEMBER_OF`, `TESTS`, `USES_TYPE`, `FILE_CHANGES_WITH`

### Node Properties

- **Function/Method**: `signature`, `return_type`, `receiver`, `decorators`, `is_exported`, `is_entry_point`
- **Module**: `constants` (list of module-level constants)
- **Route**: `method`, `path`, `handler`
- **All nodes**: `name`, `qualified_name`, `file_path`, `start_line`, `end_line`

### Edge Properties

- **HTTP_CALLS**: `confidence` (0.0--1.0), `url_path`, `http_method`
- **CALLS**: `via` (e.g. `"route_registration"` for handler wiring)

Edge properties are accessible in Cypher queries: `MATCH (a)-[r:HTTP_CALLS]->(b) RETURN r.confidence, r.url_path`

### Qualified Names

`get_code_snippet` and graph results use **qualified names** in the format `<project>.<path_parts>.<name>`:

| Language | Source | Qualified Name |
|----------|--------|---------------|
| Go | `cmd/server/main.go` -> `HandleRequest` | `myproject.cmd.server.main.HandleRequest` |
| Python | `services/orders.py` -> `ProcessOrder` | `myproject.services.orders.ProcessOrder` |
| Python | `services/__init__.py` -> `setup` | `myproject.services.setup` |
| TypeScript | `src/components/App.tsx` -> `App` | `myproject.src.components.App.App` |
| Method | `UserService.GetUser` | `myproject.pkg.service.UserService.GetUser` |

The format is: project name, file path with `/` replaced by `.` and extension removed, then the symbol name. Use `search_graph` to discover qualified names before passing them to `get_code_snippet`.
