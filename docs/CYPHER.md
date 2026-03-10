# Supported Cypher Subset

`query_graph` supports a subset of the Cypher query language. Results are capped at 200 rows.

**Supported:**
- `MATCH` with node labels: `(f:Function)`
- `MATCH` with relationship types: `-[:CALLS]->`
- `MATCH` with variable-length paths: `-[:CALLS*1..3]->`
- `WHERE` with `=`, `<>`, `>`, `<`, `>=`, `<=`
- `WHERE` with `=~` (regex), `CONTAINS`, `STARTS WITH`
- `WHERE` with `AND`, `OR`, `NOT`
- `RETURN` with property access: `f.name`, `r.confidence`
- `RETURN` with `COUNT(x)`, `DISTINCT`
- `ORDER BY` with `ASC`/`DESC`
- `LIMIT`
- Edge property access: `r.confidence`, `r.url_path`

**Not supported:**
- `WITH` clauses
- `COLLECT`, `SUM`, or other aggregation functions (except `COUNT`)
- `CREATE`, `DELETE`, `SET`, `MERGE` (read-only)
- `OPTIONAL MATCH`
- `UNION`
- Variable-length path edge property binding (can't access individual edges in a path like `*1..3`)
