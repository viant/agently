# Shared mock MCP datasources

The filesystem-backed mock server exposes JSON files as MCP tools using
`github.com/viant/mcp`. The shared client resolves Forge datasource service
metadata and calls those tools over Streamable HTTP. These packages can serve both
report preview and [Forge window preview](window-preview.md) without depending on a renderer.

## Run the server

Create a fixture folder:

```text
window-fixtures/
  ds/
    projects.json
    settings.json
  variants/
    empty/
      ds/
        projects.json
```

Each base JSON filename defines a tool name: `projects.json` becomes `projects`.
Files contain the decoded response body, not JSON-RPC or MCP wrappers. A fixture
can be an object, a record array, or a public tabular response.

Example `ds/projects.json`:

```json
{
  "status": "ok",
  "rows": [
    {"project": "Alpha", "owner": "North", "units": 30},
    {"project": "Beta", "owner": "Central", "units": 40},
    {"project": "Alpha", "owner": "South", "units": 10}
  ],
  "hasMore": false,
  "meta": {"source": "synthetic-example"}
}
```

Start the shared server from the agently repository root:

```sh
go run ./preview/cmd/mock-mcp --root ./window-fixtures --addr 127.0.0.1:8097
```

Connect an MCP client to `http://127.0.0.1:8097/mcp` using Streamable HTTP. The
`initialize`, `tools/list`, and `tools/call` lifecycle is handled by `viant/mcp`.
Use `--data-root fixtures` for a different relative base directory, or
`--variant empty` for a different default overlay. Ctrl-C stops the server.

For report folders, enable report-specific schema validation and aggregations:

```sh
go run ./preview/cmd/mock-mcp --root ./preview/report/examples/demo --report --addr 127.0.0.1:8097
```

The report preview CLI also starts an embedded `/mcp` endpoint automatically.
The shared server itself does not know about report blocks or window layouts.

## Configure a datasource

```yaml
endpoints:
  mockData:
    type: mcp
    transport: streamable
    baseURL: http://127.0.0.1:8097/mcp

dataSources:
  projects:
    service:
      endpoint: mockData
      uri: projects
      method: POST
```

The existing Forge `Service` fields identify the endpoint and tool. For this Go
MCP adapter, `uri` is the tool name, not an HTTP path appended to `baseURL`.
A preview host may resolve a relative `/mcp` URL against its own origin.
The [native window preview host](window-preview.md) uses the shared Go client
for its datasource bridge.

## Query at request time

Call a tool with no `query` argument to return the fixture body unchanged. A tool
call with `query` applies filtering, projection, sorting, and pagination on demand.
It never rewrites the file.

Example `tools/call` parameters:

```json
{
  "name": "projects",
  "arguments": {
    "variant": "default",
    "query": {
      "filter": {"field": "project", "op": "eq", "value": "Alpha"},
      "projection": {"fields": ["owner", {"field": "units", "alias": "total"}]},
      "orderBy": [{"field": "total", "direction": "desc"}],
      "page": {"limit": 1, "offset": 0}
    }
  }
}
```

This selects project Alpha, returns only `owner` and `total`, sorts by total, and
returns the first matching row with `hasMore: true`. To select several projects,
use `{"field":"project","op":"in","value":["Alpha","Beta"]}`.

| Operation | Contract |
| --- | --- |
| Filtering | Nested `and`, `or`, `not`; `eq`, `neq`, `in`, `notIn`, `lt`, `lte`, `gt`, `gte`, `between`, `isNull`, `isNotNull`, `contains`, `startsWith`, `endsWith` |
| Column selection | `projection.fields` in output order; fields can be names or `{field, alias}` objects |
| Sorting | Multiple `orderBy` entries; `asc`/`desc`; stable ties; nulls default to last, or explicit `nulls: first` |
| Pagination | Nonnegative offset, positive limit, maximum 1,000 rows per page |
| Result selection | `query.resultName` selects one result when a tabular envelope contains several |
| Variants | `variant` overlays `variants/<name>/ds`; missing overrides inherit base fixtures |

Filtering runs before projection; sorting runs before pagination. Named tabular
results preserve column metadata and matrix encoding. Record results preserve the
response envelope and metadata. A bare array remains an array and cannot expose
an envelope-level `hasMore` flag.

The generic server infers record columns from the fixture rows. Empty record
fixtures have no inferable schema; an explicit projection supplies output field
names. Use a schema-aware transform when authoritative types and empty-result
schemas are required. The report adapter supplies that behavior.

The generic query engine supports projection using `dimensions`/`measures` as well
as `fields`, but does not aggregate. Report tools use their declared aggregation
rules and support grouped queries. Generic tools reject requested aggregations
rather than inventing them.

## Reuse from Go

- [`preview/mcp/mock`](../mcp/mock): `New(Config)` creates the shared
  server; `HTTPHandler()` mounts the `viant/mcp` transport. `LocalOnly` applies
  loopback Host/origin checks for a standalone host.
- `Config.Tools` optionally maps tool names to files, descriptions, and input
  schemas. Omit it to discover `ds/*.json` automatically at startup.
- `Config.Transform` receives the selected fixture body and tool arguments. It
  can implement schema-aware operations without replacing the transport or
  filesystem loader. The report host uses this seam for its typed query engine.
- [`preview/datasource`](../datasource): `Resolve` resolves
  `*types.Service` and endpoint configuration; `Dial` initializes a `viant/mcp`
  client; `Call` returns the decoded public JSON body. Close clients after use.
- [`preview/report/mcp.go`](../report/mcp.go): report adapter
  registering tools from datasource services and fixture declarations.

Every call reads the current fixture. Editing a file is visible on the next tool
call; adding or renaming tool files requires restarting discovery. Fixtures stay
inside the selected root, including symlink resolution. File size is capped at
25 MB and concurrent tool execution at eight. Tool failures use MCP `isError`
with structured diagnostics and text fallback. A fixture's public
`{"status":"error", ...}` remains a public datasource response.

Mocks do not authenticate users, enforce business authorization, or call live
services. Keep sample data synthetic and never put credentials in fixture folders.
Report/window hosts must not silently fall back to fixtures when an MCP connection
fails.

## Verification

```sh
go test -race ./preview/mcp/mock ./preview/datasource ./preview/report/... \
  ./preview/cmd/mock-mcp ./preview/cmd/report-preview
```

Integration tests use real HTTP sessions with `viant/mcp` initialization, tool
discovery, and calls. They verify dynamic queries, envelope preservation, fixture
reload, variant overlays, filesystem boundaries, and report compilation through
MCP with no fallback on connection failure.
