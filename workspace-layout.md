# Workspace layout customization

Status: implementation in progress. The current code adds an embedded YAML default, workspace layout endpoint, configurable top-bar actions, application tabs/menus, role/feature filtering through the shared Forge authorization evaluator, remote MCP catalog and window loading, inline MCP tool datasources, and continuous cursor history. The embedded default was visually compared against a separate pre-change checkout at 1280×720; the main shell geometry matched. A custom two-app override was also checked live. Remote MCP provider end-to-end verification remains open.

## Intent

Make the main Agently shell configurable by the workspace. Ship an embedded default in Agently so a standard installation works without an additional configuration file.

Each workspace owns its complete layout and its own tabbed applications with custom menus. Application names, icons, ordering, menu groups, actions, and authorization rules are workspace configuration, not a fixed global Agently menu. A workspace can replace all default applications, including the Agently tab, without rebuilding the application.

Each running Agently instance serves exactly one workspace. Within that workspace, multiple application tabs have their own menu trees, gated by user roles, exposed features, and optionally global capabilities. Application tabs are not workspace selectors and do not require business-entity IDs. Entity-specific authorization remains available for windows and data operations that act on particular entities. A user sees only permitted applications; menu and destination checks can further restrict actions inside each application.

Each application tab's menu opens apps defined by that workspace in the right-hand UI workspace. A tab organizes related destinations; a menu item selects a Forge app/window exposed by a workspace-configured provider. Providers can load local workspace definitions, embedded Agently windows, or remote definitions supplied by an MCP server. They share the catalog/load contract below and use the same Forge renderer. Remote window definitions may include datasource definitions that call MCP; obtaining a definition and executing its data operations are separate requests.

A workspace may define a desktop sidebar split vertically into application navigation above conversation history. The upper section shows one tab per application, with menus belonging to the selected application. Show the tab strip only when more than one application is available to the current user. The lower section retains conversation history and New Conversation. The right side retains the existing chat and UI workspace behavior. The same layout schema also expresses the unchanged built-in Agently shell: a history-only sidebar and Automation in the top bar.

The initial division is 50/50 with a draggable divider, as illustrated below.

## User experience

```text
┌─────────────────────────────────────────────────────────────────────┐
│ Workspace branding                         Approvals · Account       │
├──────────────────────┬──────────────────────────────────────────────┤
│ [Agently] [My App]    │                                              │
│                      │   Existing chat / UI workspace               │
│ Application menus    │                                              │
│   Automation         │   Existing open-window tabs, focus/split      │
│   Tools              │   behavior, conversation attachments,         │
│   Models             │   transcript and composer                    │
│                      │                                              │
├──── draggable ───────┤                                              │
│ Conversations    [+] │                                              │
│ Search               │                                              │
│ Recent conversations │                                              │
│ …                    │                                              │
│ Pagination           │                                              │
└──────────────────────┴──────────────────────────────────────────────┘
```

The sidebar width and internal split can be resized independently. Both sections scroll independently; their headers stay visible. The default width is 300 px, with a configurable 240–480 px range. The initial upper-section fraction is 0.5. Each section has a 160 px minimum height, including its header; the divider occupies 8 px. For sidebar content-box height H and configured minimum M, split mode requires H >= 2*M + 8; otherwise use one scrolling sidebar and hide the divider. With defaults the boundary is exactly 328 CSS px (327 uses single scroll; 328 supports split). In viewport terms, switch when the visual viewport height is less than sidebar top offset + bottom inset + 328 px. Measure those offsets and H on resize so branding/theme header changes do not invalidate the threshold. Clamp the upper section to [M, H - 8 - M], applying the fraction to H - 8. With only one section enabled or permitted, it fills H without a divider.

The collapsed desktop sidebar remains a 64 px rail. It shows permitted application icons in configured order at the top, then a divider and a New Conversation icon (when enabled) plus an Expand Conversations icon when history is enabled. It does not show individual menu icons or conversation rows. An application without an icon uses its title's first Unicode character; all rail buttons have a title tooltip and accessible name. Clicking an application icon selects that app and expands the sidebar without opening a destination; Expand Conversations expands and focuses the history header. New Conversation uses the existing flow without expanding the rail. Preserve the expanded width and split while collapsed. Disabled applications remain disabled in the rail; filtered applications are absent. Compact mode uses the drawer, not the rail.

Application tabs choose a menu namespace, not a conversation or an open Forge window. Switching application tabs does not clear drafts, change the active conversation, close windows, or load every window in that app. Clicking a menu item performs its explicitly declared action. A selected menu is indicated only while its destination is active.

After authorization filtering, two or more applications show a tab strip; one shows an application heading; zero removes the upper section and gives history the available space. Empty menu groups and applications are removed. Disabled items may remain visible but cannot activate; hidden items are absent from the returned layout.

Conversation history remains workspace-wide with existing ownership filtering and new-conversation behavior. Application switching preserves the current search, loaded history, scroll position, and selected conversation. App-specific history is outside version 1: it requires a durable application identity on conversations and backend filtering, not a client-side approximation. The history interaction changes from replacing cursor pages to the continuous list described below.

### Conversation history interaction

Keep a sticky Conversations header with a labeled New Conversation action and a search field immediately below it. Use the placeholder “Search conversations”, an accessible label, and a clear button when nonempty. Only the conversation list scrolls; the header/search never scroll out of the lower panel. Rows show a single-line title with ellipsis, a readable relative activity time, and a clear selected state; expose the full title on hover and keyboard focus. Use at least 36 px row height on desktop and 44 px on touch targets. Secondary actions appear on hover/focus without changing row width.

Search uses the existing server `listConversations` query semantics across all accessible conversations, not just loaded rows. Do not imply full message-content search unless the backend supports it. Debounce input by 250 ms; Enter searches immediately. Changing the trimmed query resets cursors and loaded results to the newest matching page and scrolls to the top only when that query's response arrives. Ignore obsolete responses using a request generation keyed by query and identity. Keep the input focused while loading; show an inline progress indicator rather than replacing the whole panel. Preserve the latest unfiltered list and scroll anchor in memory so clearing search restores them; no search text is persisted across sessions.

Keep cursor-based backend pagination and the current 12-row request size. Load the latest page initially. Scrolling within 120 CSS px of the end appends the next older page, without replacing existing rows or moving the scroll anchor. Permit one outstanding append request; deduplicate by conversation ID while retaining backend cursor ordering. Auto-append is triggered by user scrolling, not repeatedly by an initially short list or panel resize. A labeled “Load older conversations” button at the end always provides a keyboard/touch alternative and can fill a short initial list. Disable it while loading, retain loaded rows on failure, and replace it with an inline Retry action for the same cursor. At the final cursor show “End of conversations”; omit the footer when an empty result already explains the state. Avoid page numbers and ambiguous left/right pagination arrows.

The header offers “Back to latest” after older pages are loaded or the list is scrolled away from the top. It fetches the newest page for the current query, replaces the accumulated list, and resets scroll on success. Incoming activity does not reorder visible rows while the user is browsing older history; show a “New activity — back to latest” indicator instead. At the top, updates may refresh the first page while preserving selection. Search and append failures offer retry without clearing the query or previously loaded results. Distinguish “No conversations yet — start a conversation” from “No conversations match your search — clear search”.

Selecting a conversation preserves search and the first visible row/offset so returning to the sidebar feels stable. New Conversation preserves the history query and list while opening the existing composer flow. Sidebar resizing, application switching, and collapse/expand also preserve that in-memory state. Use a virtualized list for accumulated pages, retaining loaded page data until the query is replaced, Back to latest is used, or the identity/session changes; keep focused rows mounted and announce appended counts/errors with a polite live region. Keyboard users can reach rows and the Load older button without relying on scroll-triggered loading. These behaviors apply equally inside the compact drawer.

On compact screens, the sidebar remains a dismissible drawer. Its sections use normal vertical flow, with no forced 50/50 split. Selecting a destination closes the drawer. Application tabs are horizontally scrollable. Desktop width/split preferences survive the compact transition. Keyboard navigation, tab roles, focus indication, named landmarks, and arrow-key-operable separators are required.

## Existing code and ownership

These observations are based on the repositories inspected for this design. Paths below are relative to this repository unless otherwise indicated.

| Concern | Existing implementation | Design consequence |
|---|---|---|
| Main shell and sidebar resizing | [Root.jsx](ui/src/components/Root.jsx) | Compose the new sidebar here; preserve the right-hand workspace subtree. |
| History and New Conversation | [Sidebar.jsx](ui/src/components/Sidebar.jsx) | Extract a reusable history section without duplicating its data lifecycle. |
| Branding, account, approvals, shortcuts | [MenuBar.jsx](ui/src/components/MenuBar.jsx) | Keep shell controls; route configurable navigation through one action dispatcher. |
| Chat/workspace presentation | [ConversationWorkspaceSurface.jsx](ui/src/components/ConversationWorkspaceSurface.jsx) and [conversationWindow.js](ui/src/services/conversationWindow.js) | Reuse existing conversation and window ownership semantics. |
| Workspace metadata client | [workspaceMetadata.js](ui/src/services/workspaceMetadata.js) | Discover the layout endpoint/capability alongside existing metadata. |
| Embedded app metadata | [metadata/embed.go](metadata/embed.go), [serve.go](serve.go) | Agently supplies the embedded default through host wiring. |
| Workspace configuration | [core config](../agently-core/workspace/config/config.go) | Parse and validate the workspace layout reference. |
| Workspace metadata endpoint | [core metadata](../agently-core/service/workspace/metadata.go) | Advertise the new contract; avoid embedding a large permission-sensitive tree in generic metadata. |
| Navigation and window discovery policy | [core UI handler](../agently-core/adapter/http/ui/handler.go), [policy contract](../agently-core/doc/authorization-policy.md) | Reuse `window.view` for window destinations and retain direct-load checks. |
| Window/widget capability authorization | [Forge types](../forge/backend/types/model.go), [core permittedview](../agently-core/service/ui/permittedview/compile.go) | Reuse authorization specifications, condition syntax, and server-side pruning. |

Current workspace metadata includes branding, composer settings, styles, and capability flags; it does not expose this layout contract. The UI handler already filters navigation and rechecks direct window loads when `policy.authorization.ui` is enabled. Resource-capability authorization is a separate mechanism, configured through `ui.authorization.tool`. Both must continue to apply.

## Configuration and defaults

The ownership hierarchy is workspace → layout → application tabs → custom menu groups/items. For example, a sales workspace can define CRM and Reports applications, while an operations workspace defines Monitoring and Administration. Each application has an independent ordered menu tree. Identical application or menu IDs in different workspaces do not share configuration, selection, or authorization state.

Proposed new paths:

- `agently/metadata/workspace-layout.yaml`: embedded default shipped by the main application.
- `<workspace>/ui/layout.yaml`: optional workspace replacement.
- `<workspace>/config.yaml`: optional `ui.layout.ref` selecting a different workspace-relative file.

Resolution order is explicit `ui.layout.ref`, conventional `ui/layout.yaml`, then the host-provided embedded default. Resolve through the workspace storage abstraction; examples use local paths but must not require local-only storage. Reject references escaping the workspace and arbitrary remote URLs. An explicitly configured missing file is an error. Only absence of an optional override selects the default.

The embedded default explicitly sets `left.navigation.enabled: false` and a top-bar Automation window action. Its width is configured as 220 px with `preferenceScope: global` and `preferenceKey: agently.sidebarWidth`. History is expressed through independent `search`, `list`, and `pagination` fields: placeholder, debounce, clear/Enter controls, empty-state text, virtualization, cursor presentation, and automatic older-page loading. The embedded values reproduce the prior behavior; workspace authors can change any field independently. There is no bundled legacy or classic presentation mode.

Version 1 uses replacement semantics: an override supplies the complete application list; it is not merged with embedded applications by array index. Omitted sizing fields receive schema defaults. An explicit empty application list intentionally produces a history-only sidebar. `left.navigation.enabled: false` also supports a history-only layout. Do not silently append built-in administration menus to a custom application.

```yaml
# <workspace>/config.yaml — proposed layout extension
ui:
  layout:
    ref: ui/layout.yaml
  authorization:
    tool: myMcp:resourceAuthorization
policy:
  authorization:
    mcpTool: myMcp:authorize
    ui: {}
```

The authorization settings above already exist. `ui.layout` and the following document are proposed. The default Agently layout does not require an external policy service; protected custom layouts must configure their resolver.

```yaml
# <workspace>/ui/layout.yaml — proposed schema, version 1
version: 1
id: main
left:
  width: {default: 300, min: 240, max: 480}
  split: {initial: 0.5, resizable: true, minSectionHeight: 160}
  navigation:
    enabled: true
  history:
    enabled: true
    newConversation: true
right:
  mode: existing
applications:
  - id: agently
    title: Agently
    icon: chat
    menus:
      - id: automation
        title: Automation
        icon: time
        action: {type: window, windowKey: schedule, refreshDataSources: [schedules]}
  - id: operations
    title: Operations
    authorization:
      resourceType: application
      requestedGlobalCapabilities: [manageTools]
    visibleWhen:
      all:
        - {source: authorization, field: principal.roles, contains: operator}
        - {source: authorization, field: principal.features, contains: OPERATIONS_UI}
    menus:
      - id: tools
        title: Manage tools
        visibleWhen:
          source: authorization
          field: globalCapabilities.manageTools
          equals: true
        action: {type: window, windowKey: operationsConsole}
```

In this example, `operationsConsole` is a workspace-defined app/window that the deployment must register through its existing workspace window definitions. Selecting Manage tools in the Operations tab opens that app on the right. The `schedule`, `tool`, and `model` entries illustrate built-in destinations, which may coexist with workspace-defined apps. Resolve both through the existing workspace-first window loader and its built-in fallback; layout configuration supplies navigation and does not inline the app's widgets or datasource definitions.

Applications, menu groups/items, provider descriptors, and loaded windows reuse the existing Forge `AuthorizationSpec` shape directly. Use the shared Go type rather than defining a layout-specific authorization DTO. Preserve `scope`, `resourceType`, `resource`, `dataSourceRef`, `requestedCapabilities`, `requestedGlobalCapabilities`, and `behavior` with their existing serialization and runtime semantics. Reuse the existing resolver request, snapshot, resource binding, and condition contracts. There is no new `mode` field, literal `resource.id.value` syntax, or layout-only scope value.

For app/menu role and feature gating, use the existing non-resource form: omit `scope` and `resource`, set `resourceType`, and request global capabilities only when conditions use them. The existing resolver request includes principal information (`includePrincipal: true`), so role/feature-only conditions do not need a capability request or entity ID. Use `scope: resource` only for entity-bound authorization; the existing compiler then requires the bound resource's `read` capability. Do not introduce `scope: global`. The layout authoring validator accepts the documented forms (omitted scope or `resource`), rejecting misspellings without changing existing Forge window compatibility.

The shared snapshot already carries a flexible `principal` object. Use `principal.roles` and `principal.features` as arrays of strings supplied by the trusted authorization resolver. Forge's existing tests already exercise `principal.features`; `principal.roles` is the documented resolver convention here, not a new typed field or automatically inferred claim. Keep user-level roles distinct from `resource.roles`, which describes an entity-specific grant. Exposed features describe which features this user/deployment may see; technical backend capability flags alone do not grant user access. If exposure originates in workspace configuration, the trusted resolver combines that configuration with user entitlements before producing `principal.features`.

Conditions use existing `contains`, `all`, `any`, and `not` operators. The example requires both the `operator` role and exposed `OPERATIONS_UI` feature; its Manage tools item additionally requires the global capability. Missing role/feature arrays are treated as empty for these positive membership checks. Role/feature names are case-sensitive identifiers owned by the deployment. No browser state or remote window definition may manufacture principal roles, exposed features, or grants; providers supply conditions, while the configured resolver supplies the trusted snapshot.

Node `parameters` remain available when an explicitly entity-bound node needs a `windowForm` context; window-action leaves bind `action.parameters`. A node with its own authorization uses its own context; descendants without one inherit the nearest resolved snapshot. Do not automatically merge app parameters into destination parameters. Entity-scoped requests must include `read`. Ordinary app/menu role and feature checks require none of these entity bindings.

In the example, a missing required role or feature removes the entire Operations tab and its menus before computing tab visibility. Another app can require a different role/feature combination without allocating an integer resource ID. Descendants may add restrictions but cannot override ancestor denial. Evaluate the app gate before catalog calls or protected datasource initialization. A shell node requiring a datasource/resource bootstrap that the shell cannot supply is rejected with a configuration diagnostic. Remote windows retain existing entity-authorization/bootstrap behavior. This feature adds no revocation watcher or polling lifecycle.

Tab-strip visibility is automatic in version 1, based on the permitted application count. There is no `tabs` configuration field; reject it as an unknown field.

An application or menu node can own an `authorization` specification and the Forge-style `visibleWhen`, `hiddenWhen`, `disabledWhen`, and `readOnlyWhen` conditions. Descendants inherit the nearest authorization context; a local specification resolves a new context. Ancestor denial always dominates. A protected condition without a resolvable context is a configuration error, never an implicit public item.

Disabled and read-only restrictions also accumulate from ancestors: a child cannot clear a parent's restriction by declaring its own authorization context. A disabled application or group disables all descendant actions; an inherited read-only state restricts mutating actions as described below.

Application IDs are unique in the document. Menu IDs are unique across each application's tree; canonical identity is `<applicationId>/<menuId>`. Ordered arrays define display order. A menu node is either a group with children or a leaf with an action. Validate duplicate IDs, unknown action types, invalid conditions, invalid sizing, and missing destinations. Version 1 supports only authorization-source conditions in the shell; window-form or datasource conditions remain inside windows.

### Condition validation before evaluation

Validate every condition recursively at layout load time, including conditions inside branches that would later be hidden. Do not treat malformed conditions as a boolean result: the current permittedview evaluator returns false for an unknown leaf operator, which can expose an item under `hiddenWhen` or enable one under `disabledWhen`.

- A condition is either a leaf or exactly one logical node. Reject unknown keys, duplicate YAML mapping keys, mixed leaf/logical nodes, and multiple operators.
- A leaf requires `source: authorization`, exactly one nonempty string path (`field` or the existing `selector` alias), and exactly one known operator: `equals`, `notEquals`, `in`, `contains`, `empty`, `notEmpty`, or `exists`.
- `in` requires an array. `empty`, `notEmpty`, and `exists` require booleans. `equals`, `notEquals`, and `contains` accept YAML values that can be represented as JSON; evaluate them with the existing Forge-compatible semantics rather than coercing strings to booleans or numbers.
- `all` and `any` require nonempty arrays of valid conditions. `not` requires one valid condition object. Reject nulls, scalars, and wrong container types in logical positions.
- Report the application/menu ID and condition path on failure. Reject the effective layout without returning a partial tree or falling back to embedded menus.

Required regression fixture: an otherwise valid protected menu with `hiddenWhen: {source: authorization, field: globalCapabilities.manageTools, equal: true}` must fail load-time validation for the unknown `equal` key. The endpoint must return a configuration error and no menu tree. Include equivalent malformed-condition coverage for all four guards and malformed `all`/`any`/`not` nesting when implementing the loader.

## Actions and window lifecycle

Version 1 supports a small typed action union:

| Type | Parameters | Behavior |
|---|---|---|
| `window` | `windowKey`, optional `provider` (default `workspace`), optional validated `parameters`, optional `refreshDataSources` | Resolve the provider's Forge window and open/focus it in the current right-hand host. |
| `conversation.new` | None | Invoke the existing new-conversation flow. |
| `conversation.current` | None | Return to the selected conversation through the existing conversation service. |

Groups do not execute actions. Arbitrary JavaScript, raw tool invocation, and external links are outside version 1. Later actions need explicit server-side validation and destination authorization.

`readOnlyWhen` makes mutating shell actions such as `conversation.new` unavailable; a read-only navigation item may open a destination, whose own authorization determines its editing capabilities. `disabledWhen` prevents all activation. Neither value grants access to a destination.

Extract a shared host navigation dispatcher from existing helpers rather than importing an entire MenuBar component into the new sidebar. Preserve existing parent, conversation, region, and presentation semantics. A menu action must not indiscriminately replace all tabbed windows. Replace the current `openWindow` window-key-only match with the following deterministic identity rule.

1. Resolve `provider` (default `workspace`) and trim `windowKey`; require a nonempty key advertised by that provider's permitted catalog. Preserve case and reject path aliases rather than silently rewriting them. Missing `parameters` becomes `{}`; explicit null or a non-object root is invalid.
2. Include every configured action parameter in identity. Recursively sort object keys by JavaScript UTF-16 code-unit order; preserve array order and length. Preserve strings (including whitespace), booleans, nulls, and value types. Accept only finite JSON numbers, normalize negative zero to zero, and reject integers outside JavaScript's safe range (encode large IDs as strings). Do not collapse a singleton array to a scalar, remove nulls, or insert datasource/form defaults. Reject non-JSON values and duplicate YAML keys.
3. Serialize the normalized object as compact JSON using JavaScript JSON number/string encoding. This immutable activation snapshot, not later edited form state, defines parameter identity. Thus `{b: 2, a: 1}` matches `{a: 1, b: 2}`, but `{id: 1}`, `{id: "1"}`, and `{id: [1]}` are distinct.
4. Match the tuple `(workspaceId, principalId, conversationId, parentWindowId, region, presentation, providerId, windowKey, canonicalParameters)`. Host IDs use their existing canonical values; absent optional host fields use empty strings. Capture the selected conversation at click time; no conversation uses an empty ID and never matches a conversation-owned instance. Application/menu IDs, labels, icons, and refresh lists are not identity fields. Identical destinations in two apps may therefore share an instance within the same host/conversation scope; identical keys from different providers never collide.
5. Store the tuple on the opened window and preserve it through supported window restoration. Never reparent an instance from another conversation to satisfy a match. Legacy instances lacking the tuple are not reused by this dispatcher. Recheck destination authorization before either focus or creation. Pass an explicit unique window ID into Forge to avoid its existing hash/region replacement path collapsing distinct tuples; equality uses the full tuple, not a short hash. If multiple matching instances exist, focus the selected match, otherwise the first match in active-window order.

`refreshDataSources` is an ordered array of nonempty datasource-reference strings, defaulting to empty. Trim entries and remove duplicates preserving first occurrence. Validate them against the destination's loaded permitted metadata; never initialize a reference removed by authorization. After a successful open/focus and context readiness, call each listed datasource's existing `fetchCollection` handler once; do not dispatch refresh before authorization or while the context is unavailable. An unknown reference is an actionable configuration error. This explicit post-open refresh preserves the existing Automation `['schedules']` behavior; normal window initialization may independently perform its standard initial fetch. Refresh lists do not change window identity.

For resource-bound shell nodes, use the existing `resource.id.source: windowForm` selector against the node/action parameter context described above. The shell does not fetch business data to satisfy resource selectors; unsupported bootstrap requirements produce a diagnostic rather than an alternative authorization shape. Resource identifiers and supplied parameters are identifiers, not evidence of access. Require the destination's own shared window authorization again when opened.

## Authorization contract

Provider-qualified destinations follow the provider policy identity rules in the next section; unqualified workspace destinations retain their existing window-key policy IDs.

Authorization is an intersection of the shell hierarchy, destination discovery policy, destination resource capabilities, and the underlying service permissions.

1. Load and validate the effective layout. Establish authenticated principal/workspace context on the server.
2. Collect window destinations and batch the existing `window.view` policy request when enabled, with an empty `conversationId`. Use canonical destination IDs, not menu IDs. Multiple menu references to a denied destination are all removed.
3. Resolve application/menu authorization specifications with the existing resource-authorization resolver. Reuse request and snapshot types and condition semantics. A new layout-tree adapter is required: the existing compiler accepts a Forge `Window`, so a layout cannot simply be passed into it.
4. Prune hidden nodes and denied destinations, compute disabled/read-only states, then prune empty groups and applications. Return only the effective tree and safe diagnostics. Determine tab visibility from the resulting application count.
5. On activation, recheck the destination through the existing window endpoint and resource preflight. Initialize protected widgets and datasources only after authorization succeeds. Conversation actions retain server ownership/access enforcement.

The shell must not send arbitrary window conditions to the browser and rely solely on client hiding. Extract a reusable condition evaluator/tree-pruning primitive from permittedview or provide a thin adapter with parity tests. Avoid maintaining different boolean-expression semantics in shell and Forge.

The existing policy contract treats `allow: true` with omitted or empty `allowedIds` as allowing all submitted candidates. Do not change that interpretation for this feature; complete denial uses `allow: false`. Resource snapshots use `authorizationVersion`, while discovery decisions use `policyVersion`. Preserve both independently.

If authorization is required and its resolver is absent, invalid, expired, or unavailable, protected navigation fails closed. No raw configuration or previously authorized menu tree is returned as a fallback. The UI shows a navigation-unavailable state with retry; history remains available only through its own existing access checks. Explicit denials produce an ordinary filtered tree, not a service error. Invalid configured layout produces a configuration error; it must not expose the more permissive embedded default.

A snapshot's expiry is the earliest expiry among relevant policy and resource decisions. Validate expiry when producing a layout response or reusing a server-side decision. Load navigation at startup, explicit reload, and sign-in/out or account changes; ignore responses from an earlier identity generation and clear previous-identity menu state. There is no live revocation feature: no permission polling, expiry-triggered client refresh, or automatic closing/locking of open windows. Displayed menus can remain until the next load and are not access grants. Each new destination request and subsequent data/mutation request retains its existing server-side authorization checks.

## Delivery API and caching

### Pluggable UI/window providers

The provider architecture is part of the proposed implementation. The existing workspace loader resolves local files under `extension/forge/windows`; it does not yet load remote window definitions. Core's datasource service already supports `mcp_tool` execution, while its `mcp_resource` datasource backend currently returns not implemented. The first remote provider therefore uses explicitly configured MCP tools for catalog and definition requests. None of the new configuration or contracts in this section should be presented as existing functionality.

Define a product-neutral `WindowProvider` interface in core with two operations: `ListWindows(ctx, request)` and `LoadWindow(ctx, request)`. Both receive authenticated server context, target platform, and application binding; load also receives window key, parameters, and conversation ID. Implement adapters for the existing workspace/embedded loader and for a workspace-configured MCP connection. A provider registry is initialized once for the instance's workspace; adding a new provider implementation must not require shell or Forge renderer changes.

```yaml
# Proposed additions to the layout document; MCP connection configuration
# itself remains in the workspace's existing MCP configuration.
windowProviders:
  - id: operations-ui
    type: mcp
    serverRef: operations
    catalogTool: list_ui_windows
    windowTool: get_ui_window
applications:
  - id: operations
    title: Operations
    authorization:
      resourceType: application
    visibleWhen:
      all:
        - {source: authorization, field: principal.roles, contains: operator}
        - {source: authorization, field: principal.features, contains: OPERATIONS_UI}
    windowCatalog:
      provider: operations-ui
      group: operations
    # Optional curated items can accompany the generated catalog menus.
    menus:
      - id: overview
        title: Overview
        action:
          type: window
          provider: operations-ui
          windowKey: overview
```

`workspace` is the reserved built-in provider, wrapping the current workspace-first/embedded-fallback loader. Custom provider IDs must be unique and cannot use that name. `serverRef` resolves only an existing workspace MCP connection; returned metadata cannot choose connection URLs, credentials, or another server. `catalogTool` and `windowTool` are configured tool names on that connection, not fixed MCP protocol methods. “UI/window provider” is an Agently contract transported over MCP, not a claim that MCP defines a standard window API.

An application may declare `windowCatalog` to generate menu entries dynamically, curated `menus`, or both. Catalog generation happens only after the application's authorization succeeds. `group` selects a provider group within that application's permitted catalog; omitting it includes the full returned catalog. Providers may expose groups of windows, allowing an app to organize several related screens. A generated entry becomes a normal typed window action and receives the same menu, destination, and resource checks. No window datasource is initialized during catalog discovery.

The version-1 catalog request is `{contractVersion: 1, applicationId, group, target, conversationId: "", cursor, limit}`. Omit the first cursor; default limit is 100. The response is `{contractVersion: 1, catalogRevision, groups, windows, nextCursor}`. Groups contain stable `id`, `title`, optional `parentId`, and `order`; windows contain stable `key`, `title`, optional `icon`, `groupId`, `order`, default `parameters`, and layout authorization/condition fields. Validate references, unique IDs/keys, acyclic groups, and condition syntax before use. Sort siblings by numeric `order` (default zero), then ID/key by code-unit order. Fetch cursor pages at one revision before publishing; restart once on a revision change, then return an unavailable error if unstable. Bound the catalog to 1,000 windows and 100 groups per app, reject excess or repeated cursors, and never silently truncate authorization/navigation results.

Generated node IDs use the reserved `catalog:` prefix; authored IDs cannot use it. Curated menu entries appear first. Suppress a generated entry if a curated action in that application has the same provider, key, and normalized parameters; different parameterized destinations remain distinct. Prune empty generated groups after filtering. Workspace configuration fixes the application's role/feature conditions and other authorization requirements; a remote descriptor may add restrictions but cannot replace or weaken them.

The definition request is `{contractVersion: 1, applicationId, windowKey, target, parameters, conversationId}`. Its response is `{contractVersion: 1, definitionRevision, window, dataSources}`: `window` is a self-contained Forge window definition, and optional `dataSources` contains inline core datasource definitions keyed by window-local ID. Validate the response and all referenced definitions before creating a window context. The first remote contract is declarative: remote definitions cannot introduce executable JavaScript, fetch arbitrary assets, or reference local filesystem paths. Referenced dialogs/schemas and other required metadata must be bundled in the response or refer to explicit workspace-approved assets. A provider failure shows an unavailable/retry state; do not fall back to another provider with the same key.

Inline MCP-backed datasource definitions use core's existing `mcp_tool` backend contract. The provider adapter maps window-local datasource IDs to the standard authorized core fetch path and generates the Forge datasource service bindings; it does not give the browser a direct MCP connection. These definitions are scoped to the loaded provider/window/definition revision and principal, not written into the global workspace catalog. Allow only the bound `serverRef` by default; additional MCP services require an explicit workspace allow-list. Parameter mapping, filtering, and data selectors remain datasource concerns. Each execution goes through the existing authenticated tool registry and datasource service; exposing a UI does not grant its tools or writes. The implementation must add the inline-definition registration/binding adapter—existing `mcp_tool` execution alone does not provide it.

Application role/feature and other configured authorization checks precede remote catalog calls. Whole-window policy checks precede definition loading where identity is known; returned window authorization then runs before protected data initialization. Use the existing `window.view` operation with unchanged IDs for provider `workspace`. For other providers use the collision-free ID `provider:<providerId>:<windowKey>`, reserving `provider:` against local window keys. Provider IDs contain lowercase letters, digits, and hyphens; version-1 remote window keys contain ASCII letters, digits, dots, underscores, and hyphens. Include provider/application/key as candidate metadata, and document these IDs for policy tools. The same identity is used for discovery and direct open. Provider selection is always resolved server-side from the workspace registry.

Keep `/window/{key}` compatible for the workspace provider. Add a separate provider-qualified route, proposed `/v1/workspace/ui/providers/{providerId}/windows/{encodedKey}`, that uses the same authorization/load pipeline for menu opens, direct navigation, and restored provider windows. Carry provider identity through window state and restoration; restored windows refetch their definition and authorization. Remote keys are opaque and encoded as one route value, never filesystem paths. The legacy `/navigation` endpoint remains unchanged and does not acquire remote menus implicitly.

Dynamic exposure means catalogs are fetched on layout load/Reload Navigation, and definitions on activation; it does not require live revocation, polling, or unsolicited window closure. Keep `layoutRevision` as the local source hash and return separate `catalogRevisions` keyed by application/provider/group binding. Remote catalog changes are discovered on the next layout load even when the YAML hash is unchanged. Keep `definitionRevision` with each loaded window; refreshing a definition is explicit and must not silently overwrite an edited open form. Authenticate and filter each response per principal; do not share resolved remote catalogs or definitions across users.

Add `GET /v1/workspace/layout` as a proposed authenticated, principal-specific endpoint. Advertise support and its URL through workspace metadata with a `workspaceLayout` capability and a `uiLayout` descriptor. The main application registers an embedded default provider with core; core owns loading, validation, authorization, and response construction. This avoids a dependency from core back to Agently.

Version 1 assumes **one active workspace per server process**, matching the current `workspace.Root()` and `policy.DefaultRuntime()` model. Bind the layout loader, store, embedded provider, and authorization runtimes to that workspace during startup. Different workspaces run in separate processes, or in successive process launches with fresh initialization. Do not switch the workspace root or policy runtime in a running process to serve another workspace request. The layout endpoint does not accept a workspace ID to select a different store. Each workspace still owns its independent layout and application menus.

Concurrent multi-workspace hosting would require per-workspace stores and both discovery-policy and resource-authorization runtime resolution before it could be supported. That refactor and an in-app workspace selector are outside version 1. Client preferences remain workspace-scoped so connecting to another workspace instance does not reuse its navigation selection.

The response contains `schemaVersion`, `layoutId`, `layoutRevision`, `workspaceId`, the permitted `layout`, and an authorization validity envelope with expiry and relevant versions. `layoutRevision` identifies the source configuration; authorization versions identify a decision, not a configuration change. The browser receives resolved navigation states, not raw policy credentials or unnecessary principal attributes.

Return `Cache-Control: private, no-store` initially. Do not use a configuration-only ETag or shared cache for principal-specific output. If server caching is introduced later, its key must include workspace, effective principal/account, authorization context, layout revision, and request scope, and its lifetime must not exceed the earliest authorization expiry.

Layout discovery always sends an empty `conversationId` to `window.view`, even when the browser has a selected conversation. Policy tools must explicitly support this workspace-level discovery request and return which destinations may be advertised without a conversation. They must not infer a conversation from a previous call or treat menu visibility as a grant for later calls. Opening a window rechecks `window.view` with the actual conversation ID supplied by the existing host (or empty if none). A conversation-specific grant alone does not make a destination appear in the workspace-level menu; deployments must configure their discovery policy accordingly.

A visible destination can therefore be denied on open. For the existing denial response (`404`, also used for unavailable destinations), show “This destination is unavailable or you do not have access in this conversation.” Keep the active conversation and its draft intact, do not initialize the denied window's datasources, and discard only any provisional opening state. Do not retry without conversation context or remove the menu globally because one conversation was denied. For authorization-service failure, show an unavailable/retry message instead. Menu selection becomes active only after a successful open.

### Relationship to the existing `/navigation` endpoint

The layout endpoint sits alongside `/navigation`. `/navigation` retains its Forge `NavigationItem` response and existing policy filtering for legacy clients and other consumers. It is not derived from the application layout, and the layout is not assembled by importing or merging its tree. The layout document is the sole menu source for the new Agently shell when `workspaceLayout` is advertised; that shell must not also render `/navigation` entries or use them as an error fallback.

Both endpoints must use the same shared `window.view` candidate-filtering helper/runtime, including candidate window-key normalization and denial semantics. This shares authorization logic while allowing different navigation structures. `/navigation` retains its existing request conversation context; layout discovery deliberately uses empty context. Add parity coverage for identical candidates under identical contexts, and explicit coverage of the intentional conversation-context difference. Direct window loads remain independently checked. Existing clients retain their current navigation behavior when layout support is absent.

Older clients ignore the new capability. New clients connected to servers without it retain the existing shell. When a server advertises layout support but fetching fails, the client shows an error/retry state instead of guessing an unrestricted menu.

### File changes, hotswap, and layout revision

Version 1 rereads the selected layout source through the workspace store on every layout request. Cache only parsed content by its content hash, not solely by path or mtime. Compute `layoutRevision` as SHA-256 of the source identity (embedded or workspace-relative reference), a zero-byte separator, and the exact source bytes. Any byte change, including a comment edit, changes the revision; unchanged bytes at the same source do not. A missing optional source follows the documented default precedence; a missing explicit source or invalid new content returns an error without serving a cached previous layout.

The existing `workspace/hotswap` manager does not by itself register layout handling or notify browsers. Version 1 does not add a layout watcher or a push channel: a file updated by a hotswap-enabled workflow becomes visible with its new revision on the next layout request. An open client discovers it through page reload or an explicit Reload Navigation action, which fetches the endpoint anew and replaces the menu tree atomically on success. Reload failure shows the navigation error/retry state, not the old menu tree. No browser polling or automatic window closure is introduced. Changes to startup `ui.layout.ref` require process restart; changes to the selected file's contents do not. A future watcher can invalidate the parsed cache but must retain these request/revision semantics.

## Preferences and compatibility

Persist only sidebar width, split fraction, collapsed state, and selected application ID. Scope the storage key by workspace ID, authenticated principal ID, and layout ID. Never persist the authorized tree or snapshots as preferences. Clamp saved dimensions to the current configuration. If the selected app disappears, select the first permitted app. Reset clears the scoped preference and restores configured defaults.

Use the versioned key `agently.layoutPreferences.v1:` followed by `JSON.stringify([workspaceId, principalId, layoutId])`. Wait for these identities before reading/writing preferences; anonymous sessions use a distinct `anonymous` principal namespace. Do not migrate or read the legacy unscoped `agently.sidebarWidth` value for the new layout. Missing scoped preferences start from the workspace's configured width/split, expanded state, and first permitted app. Leave the legacy key untouched for older clients and the old-server shell. Invalid stored JSON or invalid individual values fall back to defaults (finite out-of-range dimensions are clamped). Reset never consults the legacy key.

The embedded default disables application navigation, configures the prior search/list/pagination behavior field by field, and declares Automation as a top-bar window action. Top-bar actions, application menus, and remote catalog entries use the same policy filtering and destination dispatcher. A custom workspace may move Automation into an application menu or omit it. Account/sign-in and approvals keep their existing specialized flows and access checks.

Branding continues to use existing workspace app-name/icon metadata. Styling uses existing workspace style/theme support. `right.mode: existing` is the only supported value in version 1 and is reserved explicitly so the shell feature cannot silently change chat/workspace presentation. This proposal does not alter window resource assignments, conversation persistence, or Forge's content-layout schema.

Web is the first rendering target. Keep the contract platform-neutral and allow existing iOS/Android clients to ignore it until their shell renderers are updated. Native applications must use the same server-filtered layout and conditional tab rules when support is added.

## Implementation sequence

1. **Core schema and loader:** introduce proposed `protocol/ui/layout` types and `service/ui/layout` loader/compiler; add config parsing, reference validation, explicit override precedence, and embedded-provider injection. Keep these packages product-neutral.
2. **Core authorization and HTTP:** adapt the permittedview evaluator for layout nodes, batch destination policy checks, define expiry/error handling, add the endpoint and metadata capability, and connect its authenticated context through server registration.
3. **Agently default and renderer:** add the embedded YAML; wire its provider in `serve.go`; split Sidebar into shell composition and reusable history content; implement sticky search, continuous cursor loading, scroll restoration, accessible load/retry controls, and history virtualization; introduce role/feature-gated application tabs/menu rendering and a shared action dispatcher. Retain the existing right-hand subtree.
4. **Lifecycle and compatibility:** implement scoped preferences, compact drawer behavior, stale-response protection on identity changes, denied-open handling, old-server fallback, and consistent shortcut authorization. Live revocation is outside scope.
5. **Window providers:** wrap local/embedded loading with the provider interface; implement MCP catalog/definition adapters, catalog-to-menu generation, provider-qualified policy IDs/routes, inline datasource bindings, and provider-aware window identity/restoration. Integrate remote contracts with authenticated MCP execution and validate definitions before creating Forge contexts.
6. **Forge changes only where reusable:** use existing window authorization/preflight and condition types. Add generic lifecycle or accessibility fixes in Forge if needed; application tabs and conversation history remain Agently responsibilities.

## Acceptance and verification

Live visual parity check (2026-09-26): built a detached pre-change Agently checkout at `93b0cdf4` and the current checkout, ran each against a fresh temporary workspace, and inspected both in the browser at 1280×720. The default top bar was 1280×50 in both; the sidebar was 220×640 at (0,50); the chat pane was 1050×640 at (230,50); Automation was 32×32 at (180,9); and the search field was 195×24 at (12,104). The visible default controls and empty-history state matched. The new search field has an accessible label, so its accessibility tree is intentionally more descriptive. A custom override with two applications still showed tabs and the split layout in the same current binary.

- Default startup without workspace layout files serves the embedded YAML and visually matches the pre-change Agently shell: Automation in the top bar, history-only sidebar at the previous saved/default width, prior history controls expressed by separate settings, and the same right-hand chat/workspace behavior. Verify against a separately built pre-change checkout at the same viewport.
- Two workspaces served by separate processes (or successive fresh process launches) render their own application tabs and custom menus, even when they reuse layout/application/menu IDs. Configuration, authorization decisions, and selected-tab preferences cannot leak between instances. This test does not imply concurrent workspace switching within one process. A custom workspace can omit the Agently application entirely.
- A workspace replacement with two permitted applications shows tabs with independent menus. After filtering to one application the tab strip disappears; after filtering to zero, history fills the sidebar.
- Each window menu opens the intended registered window, preserves conversation drafts/attachments, and correctly distinguishes parameterized resource instances.
- A workspace-configured MCP provider dynamically exposes grouped windows inside an authorized application. Reload Navigation discovers catalog additions/removals with unchanged YAML; local and remote windows with identical keys remain distinct. Remote definitions render through Forge and inline datasource reads use authorized MCP execution.
- Test provider failure, denied application (no catalog call), denied destination (no definition call), malformed catalog/definition, cursor/revision instability, unapproved datasource service, and remote attempts to weaken application authorization. Direct opens and restored remote windows use the same provider-qualified policy identity and fresh authorization.
- A menu in an authorized application tab opens a workspace-defined app/window through its `windowKey` in the right-hand UI workspace. Verify workspace definition resolution, destination authorization, and that its widgets/datasources use the existing Forge runtime; built-in destination support is not a restriction on custom apps.
- Verify canonical parameter matching for reordered object keys, array order, scalar/array differences, numeric/string IDs, omitted versus null-valued fields, and unsafe numbers. Identical parameters in different conversations or host regions create distinct instances; identical destinations in different apps share only within the same full identity tuple. Edited form values do not change activation identity.
- Automation refreshes `schedules` once per successful activation after authorization/context readiness. Denied or unavailable contexts trigger no refresh; an explicitly requested reference removed by permission filtering is never initialized.
- Decode application/menu and local/remote window authorization using the shared Forge `AuthorizationSpec`. Test identical parameter bindings and snapshots for equivalent specs; resource-scope `read` denial; global capability requests with scope omitted; and rejection of layout-only `mode`, literal ID syntax, or `scope: global`. Keep existing window contracts unchanged.
- At default minimum heights, sidebar H=327 px uses one scroller and H=328 px supports the divider. Test nondefault minimums against the same formula, one-section layouts, and compact drawers. The 64 px rail contains app icons and history actions only, with selection/expansion and disabled states as specified.
- A browser containing only `agently.sidebarWidth` starts the new shell at configured defaults. Scoped preferences win when present, resetting uses configured defaults, and the legacy key remains unchanged.
- Editing the selected layout file yields a different revision and menu tree on the next Reload Navigation/page-load request without restart. No browser update occurs before that request. Unchanged bytes keep the revision; malformed edits fail without old-tree fallback; changing the startup reference requires restart.
- New Conversation, conversation ownership filtering, selection, and existing focus/split modes retain their behavior. History uses the specified continuous cursor list and server search rather than page replacement.
- In one instance/workspace, configure two applications with distinct role/feature gates and menus, without entity IDs. Check role-only, feature-only, combined all/any rules, missing roles/features, both permitted, one denied, both denied, inherited menu checks, and children that cannot override an app denial. Assert empty resource ID lists and inclusion of principal data in resolver requests. Separately test entity-bound windowForm selectors and read checks on windows that require them.
- Search finds an authorized match beyond the initially loaded page; a stale query response cannot replace current results. Test the 250 ms debounce, immediate Enter, clear-and-restore, both empty states, and query-preserving retry.
- Loading older pages preserves the first visible row/offset, prevents concurrent duplicate appends, deduplicates IDs, and stops at the final cursor. The labeled load/retry action works by keyboard without a scroll trigger; resizing a short list does not cause an automatic request loop.
- App switching, collapse/expand, conversation selection, and sidebar resizing preserve search and scroll state. Incoming activity while browsing older rows shows an indicator instead of moving the list. Verify sticky controls and virtualized keyboard focus in desktop and compact layouts.
- Invalid explicit references, malformed YAML, unsupported versions, unknown actions, duplicate IDs, and invalid conditions produce actionable diagnostics without permissive fallback.
- A `hiddenWhen` condition using `equal:` instead of `equals:` fails load-time validation and returns no menu tree. Validate all known operator operand types and recursively reject malformed or mixed `all`/`any`/`not` structures, unknown keys, and duplicate YAML keys.
- Layout discovery sends an empty conversation ID. Cover a policy that allows discovery but denies opening in a particular conversation: the UI shows the unavailable/access message, preserves the draft, and never retries with empty context. Also cover discovery denial despite a conversation-specific grant.
- The new shell uses only the layout menu tree; legacy `/navigation` retains its response contract. Shared filtering yields equivalent results for the same candidates/context, and a layout-fetch error never triggers fallback to `/navigation`.
- Tests cover ancestor denial, local authorization contexts, all four Forge conditions, empty-group pruning, destination denial, absent/expired resolvers, and the existing `allowedIds` semantics.
- Direct destination URLs and datasource/mutation calls remain protected even when no menu is involved. A denied menu must not initialize its protected window or datasource.
- Principal switching and late responses cannot restore a previous principal's navigation; invalid selected-app preferences recover predictably. Expired decisions are rejected on server-side resolution/reuse. There is no client expiry timer, permission polling, or automatic window closure for revocation.
- Keyboard and pointer resizing respect limits; small viewports retain usable navigation/history; multiple-app overflow remains accessible; desktop preferences survive compact transitions.
- Add focused Go loader/compiler/handler tests and React state/rendering tests, then exercise one browser scenario spanning app switching, menu activation, conversation creation, and a denied destination. Use parity fixtures for Forge condition behavior rather than copying the implementation into tests.

The design decisions above are the proposed version-1 baseline. Full arbitrary shell regions, embedded navigation widgets, app-scoped conversations, external URL actions, native renderers, and an in-app layout editor can build on this contract later.
