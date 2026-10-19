# Workspace applications from MCP

Agently hosts the workspace shell. Configured MCP connections publish their
applications, navigation, windows, reports, and dependencies. The managing MCP
selects resources for the authenticated user and authorizes its data operations.
Application ordering is workspace presentation configuration.

Configure the existing `ui/layout.yaml` with trusted application sources:

```yaml
applicationSources:
  - connection: aistudio-dsp
    resource: navigation://dspui/main
    applicationId: dspui
  - connection: ar
    resource: navigation://ar/main
    applicationId: advanced-reporting
  - connection: aistudio-management
    resource: navigation://aistudio/main
    applicationId: aistudio
  - connection: datastudio-management
    resource: navigation://datastudio/main
    applicationId: datastudio
includeUnlistedApplications: false
applications: []
```

Enable namespace-published MCP definitions through the existing trusted catalog
selection in workspace `config.yaml`:

```yaml
mcpCatalogs:
  - mcp://workspace/aistudio-dsp
  - mcp://workspace/ar
```

Each connection name refers to its normal workspace MCP definition. Applications
retain the published ID, title, icon, menu hierarchy, and window titles. Topbar,
history, sidebar sizing, and conversation behavior use the existing layout APIs.
Without application sources, the existing authored applications remain supported.

After sign-in the shared gateway discovers `namespaces/list`, lists navigation
with standard `resources/list`, and reads it with `resources/read`. Where the
server advertises filtering, the existing `io.viant/resources/filter` metadata
selects namespace and kind. Generic MCP servers retain unfiltered discovery.
`namespaces/get` is unnecessary for this application flow. The existing shared
connection pool and public/user/conversation caches remain in use; resource
notifications invalidate the corresponding cached entries.

Window list/get/open remain the primary host UI APIs. Their implementation uses
the same gateway as the navigation reader. Published windows render without an
additional host resource-access decision. Authored role and capability conditions
still control UI presentation; business authorization belongs to the managing MCP.
Namespace-published datasource and MCP definitions are consumed through the shared
readers and pool, preserving the selected window representation during dispatch.

A window action supplies `resourceUri: window://namespace/path` or the existing
local `windowKey`/optional `provider`. Relative links inside a published window
resolve within its namespace; explicit destination URIs remain explicit.
`parameters` supplies literal inputs. `parameterBindings` resolves explicitly
named active-window inputs, preserving types and arrays. Missing required inputs
disable the destination; optional bindings may be absent.

Report-builder windows use their published window URI and the normal reporting
engine. A `report://` reference is a report identity, not a window-key alias.

A `type: url` action opens its HTTPS destination with `noopener,noreferrer`;
plain HTTP is supported for loopback development hosts. Executable schemes, URL
credentials, relative URLs, and mixed window/URL actions are rejected.

Navigation discovery starts after the initial session probe settles or after an
`agently:authorized` event. Confirmed authentication denial clears navigation.
Transient probe failures preserve the optimistic shell and allow layout retry.
Credentials remain in the existing server-side authentication transport.
