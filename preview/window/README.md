# Window preview host

Reusable native Forge window preview with a workspace catalog and shared MCP
fixtures. See the [window preview guide](../doc/window-preview.md).

```sh
npm --prefix ui run build:window-preview
go run ./preview/cmd/window-preview --root ./preview/window/examples/projects
```

Open `http://127.0.0.1:8098/?window=projects` and select a project link to open its
tasks. Window IDs and parameters are carried in the query string.
