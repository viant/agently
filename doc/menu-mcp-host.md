# Optional bounded navigation menus

The host keeps its existing `/v1/workspace/layout` endpoint. Optional trusted
`menuSources` in the authorization configuration replace only its `applications`
array with visible MCP menu projections. Topbar, history, sidebar sizing and other
layout chrome retain the existing layout processing. No browser-side menu
transport, special Datly server or producer URL inference is required.

A current Steward publication can be configured explicitly:

```json
{
  "menuSources": [
    {
      "uri": "menu://steward/navigation",
      "localLayoutPath": "ui/layout.yaml"
    }
  ]
}
```

`localLayoutPath` is resolved relative to the trusted authorization file, not
relative to an MCP request. It publishes the source's existing applications
through the configured `InternalResourceProviderIdentity`. An optional
`providerIdentity` must match that internal owner. The logical namespace is
chosen in `uri`; it never denotes a server URL. Source YAML remains unchanged.
This local source supports the exact working candidate.

The menu's resource family needs the same explicit revision authority as other
resources. Its operation is `menu.view`; there is no implicit allow or invented
ACL. For example, add a family mapping and an authorized selection:

```json
{
  "resourceRevisionBindings": [
    {
      "operation": "menu.view",
      "uri": "menu://steward/navigation",
      "resource": {
        "kind": "menu",
        "id": "menu://steward/navigation",
        "tenant": "YOUR_VERIFIED_TENANT"
      },
      "action": "read"
    }
  ],
  "resourceSelections": [
    {
      "resource": {
        "kind": "menu",
        "id": "menu://steward/navigation",
        "tenant": "YOUR_VERIFIED_TENANT"
      },
      "revision": 1,
      "defaultVersion": "working"
    }
  ]
}
```

The corresponding `policies` and `requirements` must explicitly authorize the
chosen version/action. The host uses `PreparedAuthorization.ResourceResolver`
for producer selection and consumer admission. Existing window namespace
bindings remain required. Menu publication grants no window execution authority.

Local action visibility resolves `windowKey` only through the declared workspace
or framework window bindings. It applies their namespace admission and existing
catalog role/feature visibility checks. Missing window mappings disappear;
roleless and featureless metadata does not trigger entity permission lookups.
Nested groups retain the existing `children` structure and disappear when all
children are denied. Window opening continues through the existing backend guards.

An arbitrary external MCP source uses an explicit owner and logical URI:

```json
{
  "menuSources": [
    {
      "providerIdentity": "YOUR_CONFIGURED_MCP_PROVIDER_IDENTITY",
      "uri": "menu://platform/navigation",
      "revision": "release-7"
    }
  ]
}
```

Its connection is resolved by registered provider identity. Omit `revision` to
allow source and host policy selection; an explicit revision is exact and must
be authorized by both. External working/stamped menus use the same neutral
contract and may use a provider-owned users namespace when explicitly authorized.
Local users publication requires a trusted ownership adapter and is deliberately
not supported by this platform layout adapter.

The producer exposes ordinary registered `menus/list` and `menus/get` tools only
when menu bindings are installed. Source metadata/pin and filtered projection
bytes have distinct fingerprints; unfiltered source bytes are never sent.
Consumer `Gateway.GetMenu` validates that separation and forces fresh reads.

An aggregate layout read holds one verified actor and gateway read association
across every source. It retains each original provider/source pin and host
admission decision, constrains all leases, and rereads the exact source and
projection before release. Changes to account, credentials, owner, revision,
content, visibility or authority reject the response. The optional injected
`LayoutMenuRead.Close` releases its scope on all layout failure paths.

With no `menuSources`, legacy layout behavior remains. Once configured, a failed,
expired, revoked, changed or empty menu never resurrects raw layout applications.
An empty permitted menu returns `applications: []`; failures return a generic
navigation-unavailable response. The composed layout revision includes the source
pins and visible fingerprints, without representing a projection as source bytes.

Authoring or enabling an AI Studio publish flag does not install these host
bindings. Operators must configure publication and consumer authority explicitly.
