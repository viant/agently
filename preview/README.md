# Agently preview

Report and native Forge window preview applications, backed by shared MCP fixture
infrastructure. Agently owns the runnable hosts and datasource connections; Forge
supplies reusable UI components, report compilation, and rendering/export libraries.

## Layout

| Directory | Responsibility |
| --- | --- |
| `report/` | Report package loading, typed queries, compilation and export orchestration, HTTP preview |
| `window/` | Window catalog, native metadata loading, HTTP datasource bridge |
| `datasource/` | Shared `viant/mcp` client and endpoint resolution |
| `mcp/mock/` | Shared filesystem-backed mock MCP server and basic queries |
| `ui/` | Window preview frontend using Forge components |
| `cmd/` | Standalone Go launchers for reports, windows and mock MCP |
| `doc/` | Authoring, connection, and usage guides |

The generic mock server remains here with the other moved code. It is independent
of either renderer and can be extracted to `viant/mcp` separately.

## Run from the agently repository root

```sh
# Reports: no frontend build required.
go run ./preview/cmd/report-preview serve ./preview/report/examples/demo

# Windows: reuse the dependencies installed for agently/ui.
npm --prefix ui run build:preview
go run ./preview/cmd/window-preview --root ./preview/window/examples/projects

# Standalone fixture MCP server.
go run ./preview/cmd/mock-mcp --root ./preview/report/examples/demo --report --addr 127.0.0.1:8097
```

Catalog-backed reporting packages for all 25 authored Advanced Reporting profile
identities are listed in
[`report/examples/advanced-reporting/README.md`](report/examples/advanced-reporting/README.md).
The real report-group definitions and presentation profiles are loaded at
runtime; the packages supply only small datasource fixtures.

- Report preview: <http://127.0.0.1:8095>
- Window preview: <http://127.0.0.1:8098/?window=projects>
- Standalone MCP endpoint: <http://127.0.0.1:8097/mcp>

The window bundle is built under `preview/ui/dist` and is not committed. Its Vite
configuration uses agently/ui's dependencies and the sibling Forge checkout,
consistent with agently's existing UI development setup.

The main Agently CLI exposes `agently report-preview`; the standalone launchers
remain useful for focused development. Report selection uses generic `groupId` and
`reportId`, and definitions may be loaded from a local catalog or a remote MCP
contract.

Remote mode accepts either the full optional preview-definition extension or a
production `describe` response containing one report, its field catalog, and named
result sets. When authored presentation is absent, Agently generates a neutral
generic tab/chart/evidence layout and maps queries back into the server's declared
`run` request shape.

## Guides

- [Report preview builder](doc/report-preview-builder.md)
- [Window preview and cross-window links](doc/window-preview.md)
- [Shared mock MCP datasources](doc/mock-mcp-datasources.md)

## Verify

```sh
go test -race ./preview/...
go vet ./preview/...
node --no-warnings preview/ui/window/route.test.js
npm --prefix ui run build:preview
```
