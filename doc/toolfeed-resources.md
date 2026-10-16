# Toolfeed definitions as resources

A toolfeed definition has a canonical URI such as
`toolfeed://team/nested/terminal`. The namespace is a logical name, not a
network destination. Providers advertise `toolfeeds/list` and `toolfeeds/get`
as ordinary MCP tools and declare support in namespace capabilities.

Definitions and live activity are separate. The definition retains the native
FeedSpec fields and legacy `id`; live activation and content updates continue
through the existing `agently.feed` AG-UI activity contract. Reading a definition
does not activate a feed or execute its datasource.

## Register a workspace source

The trusted host authorization configuration can contain:

```json
{
  "toolfeedSources": [
    {"namespace": "team", "directory": "feeds"}
  ]
}
```

With an explicitly configured internal resource provider, this maps
`feeds/nested/terminal.yaml` to `toolfeed://team/nested/terminal`. Paths remain
confined to the workspace. There is no inferred namespace or default grant.
The `users` namespace requires a trusted ownership projection and is rejected
by this host mapping.

Each indexed URI also needs its existing authorization configuration:

- Exact `resourceRevisionBindings` for `resource.list` and `resource.get`, with
  resource kind `toolfeed`, the complete URI as resource ID, and an explicit tenant.
- `resourceSelections` and role/feature policy that authorize the working
  candidate. An omitted selector is selected by policy, not by choosing latest.

These YAML sources expose a mutable working candidate; they do not invent
stamped history. Reads return the authorized content fingerprint and authority
binding. Reusing a pin after content changes, expiry, or revocation fails.
Unknown namespace/kind, foreign ownership, collisions, and unconstrained paths
are rejected. Unconfigured providers do not gain toolfeed support implicitly.

## Feature dependency state

The implementation spans the separate `protocol/primitive` module and Core.
Feature verification currently uses explicit temporary module replacements to
select the matching local module sources. Publication and normal dependency pins
must be completed before claiming a remotely consumable release; no unpublished
module version is assumed to exist.
