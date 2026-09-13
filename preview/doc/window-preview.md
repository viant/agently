# Forge window preview

Preview native Forge windows against the shared filesystem-backed MCP server.
The preview uses `WindowManager`, `WindowContent`, the standard metadata loader,
and ordinary declarative window links. It does not create a separate widget or
cross-window navigation grammar.

## Run

From the agently repository root:

```sh
npm --prefix ui run build:preview
go run ./preview/cmd/window-preview --root ./preview/window/examples/projects
```

Open <http://127.0.0.1:8098/?window=projects>.

The frontend build requires the repository's JavaScript dependencies. The Go host
serves assets from `preview/ui/dist` by default. Use `--assets` to override
the built assets directory and `--addr` to change the loopback address. The generated
bundle includes the normal Forge runtime and is intentionally not committed.

## Window IDs and links

`?window=<id>` selects a window from the workspace catalog. `parameters` contains
an optional JSON object. For example:

```text
/?window=project-tasks&parameters=%7B%22projectId%22%3A2%7D
```

This opens the tasks window for project 2. Links and active tab changes update the
URL, so the selected window and parameters can be bookmarked or reloaded.
`variant` selects fixture overlays. `query` optionally supplies a normalized MCP
query override for the initially selected window.

A native table link opens another catalog window:

```yaml
- id: project
  name: Project
  type: link
  link:
    kind: window
    windowKey: project-tasks
    windowTitle: Project Tasks
    parameters:
      projectId: {source: row, selector: projectId}
```

The preview uses Forge's existing `handlers.window.openWindow` and
`ui.window.open` command. Window parameters become initial datasource inputs;
explicit datasource request inputs take precedence. `filterFields` in the shared
datasource definition declares which inputs become fixture predicates.

## Workspace layout

```text
workspace/
  preview.yaml
  windows/
    projects.yaml
    project-tasks.yaml
  ds/
    projects.json
    tasks.json
  variants/
    empty/ds/projects.json
    error/ds/projects.json
```

`preview.yaml` declares the allowlisted catalog and reusable datasources:

```yaml
title: Business workspace preview
defaultWindow: projects
windows:
  projects: {title: Projects, file: windows/projects.yaml}
  project-tasks: {title: Project Tasks, file: windows/project-tasks.yaml}
endpoints:
  mockData: {type: mcp, transport: streamable, baseURL: /mcp}
dataSources:
  tasks:
    cardinality: collection
    selectors: {data: rows, dataInfo: dataInfo}
    backend: {kind: mcp_tool, service: mockData, method: tasks}
    filterFields: {projectId: projectId}
    query:
      orderBy: [{field: taskId, direction: asc}]
      page: {limit: 25, offset: 0}
```

Native window YAML references a shared datasource:

```yaml
namespace: Project Tasks
dataSource:
  tasks: {dataSourceRef: tasks}
view:
  content:
    id: taskTable
    dataSourceRef: tasks
    table:
      columns:
        - {id: task, name: Task}
        - {id: status, name: Status}
```

The host merges shared datasource metadata and rewrites the frontend service to
`POST /v1/api/datasources/{id}/fetch`, with the `inputs` envelope used by
agently-core. That HTTP bridge uses the shared `viant/mcp` client to invoke the
configured backend tool. It never falls back to local rows after an MCP failure.

Use an absolute `endpoints.mockData.baseURL` to connect to a standalone mock server.
See [Shared mock MCP datasources](mock-mcp-datasources.md).

Window YAML and fixture edits are read on subsequent loads/calls; reload the page
to clear runtime metadata. Restart the host after changing `preview.yaml` or adding
fixture tool filenames. Relative local YAML imports are preflighted before loading;
remote imports and paths escaping the workspace are rejected.

Workspace visual overrides use the standard optional
`extension/forge/styles/manifest.yaml` contract. Only CSS files explicitly listed
by that manifest are compiled and served; paths outside the workspace, remote
stylesheets, and unlisted files are rejected. The preview attaches the versioned
workspace stylesheet after Forge's base styles and checks for a new revision once
per second, so a valid CSS edit is visible without rebuilding Agently or copying
workspace content into this repository. A broken edit retains the last valid style
snapshot and is reported in `/api/workspace` under `uiStyleDiagnostics`.

## Query controls

The side panel selects a window and variant and accepts parameter/query JSON.
**Apply and reload** applies those inputs. Query overrides use the shared MCP
filtering, sorting, projection, and pagination contract. Native paging supplies
`size` and `offset`, overriding default query paging. Table column sorting retains
the standard Forge widget behavior; use a query `orderBy` to sort the complete
fixture result before paging.

Example query override:

```json
{
  "filter": {"field": "region", "op": "eq", "value": "North"},
  "orderBy": [{"field": "budget", "direction": "desc"}]
}
```

This initial host supports record-array datasource responses (`rows` or a bare
array), `mcp_tool` backends, and synthetic fixtures. It does not implement the full
agently datasource cache/composite backend stack, production authorization, or
MCP resources for window opening. The preview's cross-window links execute locally
through the existing Forge runtime.

## Relationship to agently-core

The existing agently-core `ui/view` service exposes `list`, `get`, and `open`.
`open` resolves the view and its parameters, then sends `ui.window.open` to Forge.
The window preview reuses that final command and rendering path; it does not
replace or duplicate the agently tool.

The runnable preview host, frontend, examples, mock server, datasource adapter,
and launchers live in `agently/preview`. Forge supplies reusable UI components,
metadata models, report compilation, and rendering/export libraries. This keeps
application hosting and datasource orchestration out of Forge.

A future `agently window preview --id <id>` command can adapt the current workspace
catalog and call this host. This migration retains standalone launchers under
`preview/cmd`; it does not yet register the proposed resource commands in the main
agently CLI. The existing agently-core datasource/view services remain the target
for full workspace integration.

## Verification

```sh
go test -race ./preview/window ./preview/cmd/window-preview
node --no-warnings preview/ui/window/route.test.js
npm --prefix ui run build:preview
```

The demo contains two linked business-neutral windows and default, empty, error,
and large fixtures. Backend tests cover catalog references, native link metadata,
MCP filtering/paging, empty/error variants, import boundaries, and connection
failure without fallback. Route tests verify typed parameters and URL preservation.

### Local role and feature simulation

The preview shell's Roles and Features dropdowns are checkbox multi-selects.
Options are discovered from the served window metadata and its initial
`authorizationSnapshot`. Selections update the in-memory metadata snapshot's
principal for mounted preview windows, so Forge's conditional tabs/controls
react immediately. Reload restores the server-provided initial snapshot.

This is local presentation testing, not an authentication or permission grant.
The resource/account snapshot and resource capabilities are preserved; the
read-only preview bridge remains responsible for rejecting mutations. Removing
a role or feature does not rewrite capability grants. No production account,
server authorization policy, or workspace file is changed by the selectors.
