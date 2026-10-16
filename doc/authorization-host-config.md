# Unified authorization host configuration

`ServeOptions.ConfigureBuilder` installs application-owned providers and mappings
before Core builds its workspace authorization runtimes. It can construct a
prepared authorization adapter, bind a protected Forge catalog and install
custom policy stores, requirements, entity or entitlement providers.

An embedding host may supply an immutable file through `--authz-config PATH`
or `AGENTLY_AUTHZ_CONFIG` together with
`ServeOptions.AuthorizationProviderFactory`. The factory interprets only the
operator-owned opaque `identity` and `entityEvaluation` settings and returns
verified identity, policy namespace validation, selected permission checking
and optional permission leases. The public executable requires this injection
and fails startup when provider settings have no binding.

The outer file requires `schemaVersion: 1`, provider/mapping refs, policy version,
catalog path and exact policy/resource/capability mappings. It rejects unknown
outer fields and multiple JSON objects. Relative catalog paths resolve against
the file directory. Provider-specific fields and endpoint schemas are validated
by the injected host binding; public Agently interprets no numeric user/account
claims or user-info envelopes. Exact `selectedScopeBindings` map canonical ACL
resource/action pairs to named scope admission permissions. Mandatory gates
continue to enforce action-specific requirements.

The workspace must select `mode: authz` and matching refs for both UI and
policy authorization. An explicit file paired with omitted, legacy or mismatched
workspace settings fails during startup. Every mapped resource/action must have
both an ACL policy and requirements document. Without a file,
the embedding callback remains available; without either, existing workspace
configuration continues to apply. A selected missing or malformed file never
silently disables authorization.

The file alternative does not configure writable storage or editor workflows.
Those application integrations use `ConfigureBuilder`. Validate the actual
host seam and CLI parser with:

```sh
go test . ./cmd/agently -run 'TestHostAuthorization|TestServeCmd_AuthorizationConfig' -count=1
```

For operator preflight, run `agently serve --validate-authz --workspace PATH
--authz-config FILE`. An embedding host with its provider factory loads the authority configuration and registers
the protected catalog/mappings without creating execution stores or listeners.
It does not grant access, probe datasource services or prove policy parity for a
particular user.

Authenticated `/auth/me` responses include an opaque `sessionBinding` lifecycle
fence. The browser clears mounted protocol/cache state when it changes, including
same-subject credential/account changes. Credential rotation conservatively
changes this fence; durable conversations can then be restored with fresh
authorization. The fence contains no raw cookie or token and grants no authority.
