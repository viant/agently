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
npm --prefix ui run build:window-preview
go run ./preview/cmd/window-preview --root ./preview/window/examples/projects

# Standalone fixture MCP server.
go run ./preview/cmd/mock-mcp --root ./preview/report/examples/demo --report --addr 127.0.0.1:8097
```

- Report preview: <http://127.0.0.1:8095>
- Window preview: <http://127.0.0.1:8098/?window=projects>
- Standalone MCP endpoint: <http://127.0.0.1:8097/mcp>

The window bundle is built under `preview/ui/dist` and is not committed. Its Vite
configuration uses agently/ui's dependencies and the sibling Forge checkout,
consistent with agently's existing UI development setup.

These are the relocated standalone launchers. The proposed `agently report ...`,
`agently window ...`, and `agently mcp mock serve ...` command groups are not yet
registered in the main CLI. This move also preserves the current preview datasource
adapter; full integration with agently-core's workspace datasource service is a
separate step.

## Guides

- [Report preview builder](doc/report-preview-builder.md)
- [Window preview and cross-window links](doc/window-preview.md)
- [Shared mock MCP datasources](doc/mock-mcp-datasources.md)

## Verify

```sh
go test -race ./preview/...
go vet ./preview/...
node --no-warnings preview/ui/window/route.test.js
npm --prefix ui run build:window-preview
```
