# Generic primitive cleanup and latency plan

Status: implementation in progress. The initial source review below is dated 2026-10-13; the execution record at the end distinguishes completed changes from pending verification. The overall Steward acceptance and latency targets remain unproven.

## Governing correction: resource authorization is owned by MCP

The user's subsequent clarification overrides any earlier migration proposal in this document that would install resource authorization in the Agently/Core client path:

- The managing MCP owns resource visibility, authorization, revision selection and execution permission. Its list responses are already filtered; its get/execute denials are authoritative.
- Agently/Core must not re-evaluate resource ACLs, account/entity/parent permissions, policy documents, or resource authorization leases before forwarding a request. Remove these duplicated checks; do not relocate them into `app/host`.
- Local UI rules may control the visibility of controls, tabs and menu items. They are presentation rules and do not grant resource access.
- Authentication, forwarding the current credential, private-cache isolation, browser/conversation ownership, input validation and response/reference integrity remain infrastructure responsibilities. They are not a second resource-policy engine.
- The in-memory MCP follows the same provider/client separation. Native definition loading is not a reason to invent application-side resource policies. Existing data-service authorization stays at its managing server.
- Generic authz libraries and private identity adapters remain available to managing servers; Agently's gateway must not depend on them to decide whether an external resource is allowed.

Accordingly, `authorization_delegated.go`, `authorization_selection.go`, `authorization_resolved_mapping.go` and the resource-policy portions of `authorization.go`, `authorization_reports.go`, `authorization_internal_visibility.go` and window admission wiring are **removal candidates**, not code to move into Core. Retain only the distinct UI-visibility portions where applicable. The earlier ownership/move tables are provisional and must be read subject to this correction.

Performance attribution follows this boundary: measure local authentication/credential forwarding and UI-visibility work locally; measure upstream resource authorization only when the MCP provides timing/trace data. Otherwise report upstream latency as unattributed rather than fabricating a client auth-check duration. Do not add policy checks for instrumentation.

## Outcome

Agently should assemble its runtime. Agently Core should implement reusable primitive discovery, registration, resolution, delivery and execution integration. Generic authorization policy belongs in `viant/authz`; Viant identity and entity mappings stay in the private adapter. Do not move the whole existing application package into an Auth library.

Use one shared gateway and producer infrastructure for windows, reports, datasources, skills, intents, templates, menus and toolfeeds. Use Go generics for typed definitions and common operations. Keep payload-specific compilation, validation and execution semantics explicit. A report and a window share delivery machinery; they are not the same document schema.

The operational target is at most **12 ms added local overhead** for a ready, cached primitive operation across the entire gateway path, not 12 ms per wrapper. Measure upstream MCP/IdP time, initialization, computation and UI rendering separately while also reporting the actual end-to-end latency. An expensive cold compilation cannot be made compliant by renaming it or omitting it from the trace.

## 1. Reviewed baseline and latest-main scan

| Repository | Feature HEAD reviewed | Latest fetched `origin/main` | Incoming commits absent from feature |
| --- | --- | --- | ---: |
| Forge | `24066d8402d8586fc2e2932974980e4f17086845` | `36079cd69f061a206c3af4222c65c3c0f2fdd0ad` | 3 |
| Agently Core | `6b4876e653fd37b67d476945ad52348a08ed37d8` | `9dc5cf5bada47cad90bb50e228e7925ba51d09db` | 31 |
| Agently | `a9c87dacbd54f442e294d76772477a39eea05287` | `7976157b0309887d4222ec7a4700e2811fa5cc96` | 23 |

Main was fetched and relevant source diffs inspected. **The feature branches are not synchronized with these newly fetched heads.** This scan is not a runtime regression pass. Earlier test results belong to earlier source graphs and cannot certify this combination.

Tracked/untracked local work overlaps incoming main paths: Forge 44 paths, Core 4, Agently 4. Overlap does not prove a conflict or equivalent content. Preserve and compare those edits before integration; do not stage them wholesale. Especially inspect Core `go.mod`, `protocol/tool/schema/legacy_body_args{,_test}.go`, `sdk/artifact_bridge.go`, and Agently `go.mod`, `go.sum`, `ui/src/components/chat/ChatFeedFromChatStore.jsx`, `ui/src/styles/shell.css`.

### Main changes that cleanup must preserve

| Area | Source evidence on latest main | Required preservation / regression check |
| --- | --- | --- |
| Compact history | Core `8703507d`, `5e677265`; `sdk/ts/src/aguiConversationCompactBootstrap.test.ts`, `aguiConversationCompactHistory.test.ts` | Routine bootstrap must not load raw model payloads. Details remain explicit/lazy. Measure bytes and restoration time. |
| Streaming completion | Core `084a5662`, `68f790a8`; `sdk/ts/src/aguiConversationTransport.ts`, `client.ts`, `aguiConversationStreamingCompletion.test.ts` | Fully streamed primary turns must not trigger redundant full transcript reconciliation. Preserve recovery for external/native and browser-tool turns. Integrate replacement-listener replay with this behavior rather than restoring unconditional refreshes. |
| Browser MCP | Core `2aff15e6`, `4f794ef4`; `service/browsermcp/registry.go`, `sdk/ts/src/browserMCP.ts` | Compact current-catalog validation, selected-server lookup, active lifetime refresh without rebinding, connection ownership and no server fallback for browser execution. |
| Existing performance work | Core `service/agent/tool_surface_cache.go`, `internal/tool/registry/tool_surface_cache.go`, `service/agent/binding_payload_reads.go` | Reuse and consolidate these caches and request-local immutable payload reads. Do not add a parallel primitive cache with a different scope policy. Broad tool patterns currently stay uncached; a public aggregate must be composed from independently classified partitions. |
| Conversation lifecycle | Core `9dc5cf5b`, `sdk/handler_conversations_delete_test.go`; Agently `7976157b` | Preserve scoped Datly flush and actual conversation deletion. A matching status/error classification must reach UI. |
| History viewport | Agently `ui/src/components/chat/historyScroll{,.test}.js` | Open at the correct position, follow late rendering, stop following on user scroll, and dispose observers on navigation. |
| Lookup/composer | Agently `NamedLookupInput.jsx`, `starterPromptMacros.js`, `moneyTokens.test.js`; Forge composer accessibility changes | Empty and populated pickers, retained selection, clear/reopen filters, slash entry, money/date chips, accessibility and original datasource bindings. |
| Window metadata recovery | Forge `WindowContent.jsx`; Agently `windowMetadataRecovery.test.js` | Preserve explicit retry of transient errors. Do not retry a 401/403 as a generic transient error. Canonical window OPEN admission is distinct from datasource business authorization. |
| Dataset denial and editing | Forge `0f88385`, `36079cd`; `DataSourceFetcher.denial.render.test.jsx`, `Basic.datasetDenial.render.test.jsx`, sparse mutation and editor tests | Dataset 403 remains scoped to the dataset, clears stale data appropriately, does not repeatedly auto-fetch, and does not become a window denial. Keep filters usable for empty results/custom search and preserve sparse edits. |
| Other main fixes | Core `42a50c51`, `44c2772b`, `33fc6960`, `7dc652cd`, `89ff40e4` | Preserve MySQL-compatible orphan SQL, linked SQLite OAuth discovery, nullable approval completion, original task through attachment continuations and task cancellation state. |

Latest main already documents browser timing in Core `doc/browser-mcp-performance.md`: two measured tool lifecycles of 233/438 ms, backend deferral of 35/32 ms and result persistence of 13/92 ms. Those reported samples are not new measurements from this review and are not a 12 ms overhead pass. They identify existing measurement boundaries and work still required; do not compare a handler-only benchmark with the whole lifecycle.

## 2. Duplication and misplaced responsibilities

In Agently's root, the `authorization*`, `native_*` and `component_*` Go families contain **51 files: 24 production files (4,237 lines) and 27 tests**, including a local untracked diagnostic test. File count alone is not the problem; the root package owns substantial reusable behavior.

| Finding | Evidence | Refactoring decision |
| --- | --- | --- |
| Application root owns generic provider contracts | `authorization.go`: `AuthorizationProviders`, `AuthorizationProviderFactory`, config decoding and runtime assembly | Core owns reusable contracts/composition. Agently resolves application config paths, supplies dependencies and starts the host. |
| Repeated source registration | `authorization_authored.go` and `authorization_toolfeeds.go` repeat directory binding, actor verification, resolver construction, provider assignment and snapshot replacement | One generic registration pipeline, with typed compiler/validator adapters. Remove copied resolver loops. |
| Repeated actor-context wrappers | `authorization_http.go`, `authorization_account_projection.go`, `authorization_read_sequence.go`, `component_authority.go`, plus wrappers in `authorization.go` | Remove resource-policy wrappers from the client path. Retain only authentication/credential and UI-visibility adaptation with explicit ownership. |
| Snapshot orchestration in application root | `authorization_internal_snapshot.go` combines menu, window, template, intent and toolfeed loading; `authorization_workspace_reports.go` separately builds report snapshots | A common immutable snapshot builder in Core with explicit compiler inputs and dependency graphs. Application asset selection remains configuration. |
| Duplicate producer state and reconstruction | Core `service/resource/local.go` stores its own bindings/configuration and `neutral()` constructs a `primitiveproducer.LocalProvider` again for delegated operations | One producer instance per published generation. Remove the duplicate state owner and per-call reconstruction; retain only a temporary compatibility facade if callers require it. |
| Generic infrastructure already exists | Core `service/resource/generic.go`: `List[T]`, `Get[T]`, `Located[T]`, `Result[T]`; `service/resource/catalog.go`: `ListPage[T]`; `protocol/primitive/catalog.go`: `Catalog[T]` | Extend these implementations. Do not introduce another gateway per kind or another independently cached generic gateway. |
| More than one cache layer owns related representations | Manager tool pages and visible catalogs; gateway capability cache; native snapshots; newer main's agent tool-surface caches | Define each layer's value, scope, generation and invalidation owner. Derived representations reference one authoritative generation. Consolidate redundant layers after measuring their actual purpose. |
| Warm lookup does avoidable preparation | `visible_catalog.go:CatalogScope` enumerates providers, gets clients, serializes config and may resolve credentials; `catalog.go` marshals rows and unmarshals selected results | Precompute stable provider descriptors/config fingerprints and typed immutable projections. Keep dynamic access identity correct. Measure before claiming these operations dominate latency. |
| Public tool metadata and visible resources are conflated in expectations | `tool_catalog.go` can share explicitly public `tools/list`; `visible_catalog.go` currently scopes visible resources privately and includes conversation/client identity | Model public/private/browser scope explicitly per surface. A public tool schema does not make its resource list, user projection or data result public. |
| Fragmented timing | Feature paths have useful timing/call-count tests; latest main has browser UserTiming, Server-Timing and phase logs | Adopt one correlated primitive-operation trace across these boundaries. Existing timers become children of that trace, not separate incompatible instrumentation. |

## 3. Explicit file removal / relocation inventory

“Remove” below means remove the current implementation file after its responsibilities and tests have been consolidated and callers migrated. **It is not authorization to delete behavior or discard working edits.** Proposed package names are deliberately few; do not replace every old prefix with a new subpackage.

### Agently production files to retire from the root implementation

| File | Destination / disposition |
| --- | --- |
| `authorization.go` | Split reusable configuration/composition into Core `app/host`; reduce Agently to small startup wiring and necessary exported compatibility aliases. Retire the 676-line root implementation. |
| `authorization_account_projection.go` | Core host authorization adapter; use generic account projection, with private mapping supplied externally. |
| `authorization_authored.go` | Core typed source registration and template/intent compiler adapters. |
| `authorization_bootstrap.go` | Delete the consumer window admission/bootstrap configuration. Resource authorization belongs to the managing MCP. |
| `authorization_delegated.go` | Remove application-side admission of resources already managed by external MCP. |
| `authorization_hosted_apps.go` | Core hosted-app registration adapter. |
| `authorization_http.go` | Core adaptation of the existing public authz HTTP provider. The actual generic HTTP authority remains in authz; no second IdP implementation. |
| `authorization_internal.go` | Core host composition over one primitive registry/producer/gateway. |
| `authorization_internal_snapshot.go` | Core snapshot composition over typed compilers. |
| `authorization_internal_visibility.go` | Retain UI control/tab/menu visibility only; remove client resource-policy evaluation. Owning MCP filters resource discovery. |
| `authorization_internal_window_catalog.go` | Core window codec/catalog adapter on shared resource infrastructure. |
| `authorization_menus.go` | Common registration/snapshot work to Core; keep the menu action/visibility projection in the menu adapter. |
| `authorization_native_templates.go` | Core template/intent adapters and registry. |
| `authorization_read_sequence.go` | Remove client authorization-decision propagation; retain only independently justified request correlation or data consistency in existing infrastructure. |
| `authorization_reports.go` | Core report source registration and existing report service integration. |
| `authorization_resolved_mapping.go` | Remove local resource-policy mapping from application/gateway; managing MCP owns its configuration. |
| `authorization_selection.go` | Remove client policy-selected revision implementation; forward requested references and preserve the server-selected revision. |
| `authorization_toolfeeds.go` | Same generic registration path as other definitions, plus toolfeed codec. |
| `authorization_window_sources.go` | Core source/bundle compiler infrastructure and window codec. Agently supplies embedded bytes/assets as inputs. |
| `authorization_workspace_reports.go` | Core native report compiler/snapshot adapter. |
| `component_authority.go` | Remove the extra client resource-authority refresh. Ordinary authentication and UI visibility have separate owners. |
| `native_window_delivery.go` | Core window delivery adapter, sharing source selection and request scope. |
| `native_window_host.go` | Core host composition; Agently retains invocation/config wiring only. |
| `native_window_selection.go` | Retain source/argument integrity plumbing; remove consumer bootstrap datasource/read wrappers. |

Delete `window_bootstrap.go`, `window_bootstrap_configured.go`, and their obsolete consumer-admission tests. `default_framework_source.go` supplies Agently's embedded framework assets; reusable loading/bundle logic belongs in Core. UI controls may use current roles/exposures; no consumer account/entity/resource preflight belongs in that projection.

Move the corresponding unit tests with their implementation. Keep true executable startup/HTTP composition tests in Agently. The untracked `authorization_framework_owned_diagnostic_test.go` is a local diagnostic candidate to archive outside the product package after preserving any unique regression coverage; do not silently commit or delete it.

### Core removal / consolidation candidates

| File / area | Decision and prerequisite |
| --- | --- |
| `service/resource/local.go` | Retire duplicate provider implementation/state in favor of `service/primitiveproducer.LocalProvider`; migrate direct private-state users and inject validation caching explicitly before removing the facade. |
| `service/resource/local_client.go` | Consolidate MCP client construction with the producer's existing in-process/standalone transport adapters. Keep a temporary public compatibility function only if necessary. |
| `service/resource/menu.go` | Move producer aliases/validator ownership to the producer or shared contract; migrate `GetMenu` callers to typed generic get with an explicit menu projection adapter. Remove this facade file only after all remaining helpers have an owner. Preserve distinct source/projection fingerprints. |
| `service/resource/native_framework_window.go` | Fold the small framework source binding DTO into the window adapter's shared registration contract. Do not create an additional generic hierarchy just to remove one file. |
| `service/resource/local_adapters.go` | Split its mixed window registration and window/report validators into their actual adapters; replace common registration with generic registration. Retire this mixed-responsibility file after migration. |
| `service/agent/tool_surface_cache.go` and `internal/tool/registry/tool_surface_cache.go` on latest main | **Not immediate deletions.** Consolidate common scope/generation computation into one manager-owned policy; keep the needed immutable LLM-schema projection until its replacement has equivalent performance and isolation tests. |
| `service/resource/authority.go`, `public_read.go`, `gateway_auth.go`, `read_sequence.go` | Audit context/freshness overlap, but no evidence supports wholesale deletion. Retain their safeguards until one equivalent boundary replaces them. |

Do **not** delete `generic.go`, `catalog.go`, `protocol/primitive/catalog.go`, native snapshot storage, browser catalog ownership, payload codecs, or window/report execution services merely because they are separate files. These represent distinct responsibilities. Do not remove the application’s original reporting implementation as a side effect of consolidating resource delivery.

## 4. Target ownership and dependency direction

| Owner | Responsibilities | Must not acquire |
| --- | --- | --- |
| `viant/authz` | Generic policy evaluation, selected revisions and authorization contracts | Agently startup, Forge widgets, window YAML, report builders, Viant-specific entity mapping |
| Private Auth adapter | Viant identity/protocol mapping and entity/capability evaluation, served through deployed HTTP IdP/provider endpoints | Public generic package ownership or application UI wiring |
| Core `protocol/primitive` / resource identity contracts | URIs, metadata, kinds, pagination/filter queries, typed result envelopes and revision identity | Filesystem scanning, network calls, application-specific policies |
| Core `service/resource` | One federation/gateway path, typed list/get, shared catalog indexes and definition/dependency resolution | Authoritative aggregate self-proxy, business ACL duplication, a gateway instance per resource type |
| Core `service/primitiveproducer` | Authoritative local provider, ordinary MCP tool/resource publication, injectable admission, in-process and standalone operation | Dependency on Agently executable or private company packages |
| Core `protocol/mcp/manager` | Connection/authentication integration, authoritative cache scope/generation and transport catalog lifecycle | Business entity/parent permission decisions or cached tool execution results |
| Core `app/host` (proposed) | Compose Core services, transport credentials, source registrations and UI-visibility inputs | Resource-policy evaluation, CLI/environment ownership or a new copy of resource services |
| Kind adapters | Typed compilation/validation, dependency extraction, rendering/execution contracts specific to a kind | Independent cache/scope/transport/authentication frameworks |
| Agently | CLI/options/environment, application assets, service construction and UI hosting | Repeated resource/policy/provider implementations |
| Forge | Payload schemas, widgets and rendering behavior | Auth providers, per-user backend catalogs, business ACL logic |

There is no justification for putting all these files in `viant/auth`. The local public checkout reviewed is `viant/authz`. Authentication adapters and generic authorization are not interchangeable with resource delivery. Dependency direction must remain: application → Core integration → generic contracts/authz; private providers are injected or accessed over HTTP.

## 5. Go generics design

Retain the existing shared `Gateway`, `List[T]`, `Get[T]`, `ListPage[T]` and immutable `Catalog[T]`. Do not instantiate a separate `Gateway[T]` with separate maps for every payload type. Go methods cannot introduce independent type parameters; use generic functions or a lightweight typed facade over the same gateway.

Illustrative contract, to be reconciled with existing types rather than added as a parallel API:

```go
type Kind[T any] struct {
    Name             string
    Versions         []int64
    Decode           func(json.RawMessage) (T, error)
    Validate         func(context.Context, T) error
    Dependencies     func(T) []ResourceRef
}

type Registration[T any] struct {
    Kind      Kind[T]
    Namespace string
    Source    DefinitionSource[T]
}

func Register[T any](r *Registry, in Registration[T]) error
func List[T any](ctx context.Context, g *Gateway, kind Kind[T], q Query) (Page[Located[T]], error)
func Get[T any](ctx context.Context, g *Gateway, kind Kind[T], ref ResourceRef) (Resolved[T], error)
```

`DefinitionSource[T]`, `Registry` and illustrative names above are design placeholders. Prefer adapting existing `ResourceSource`, `ExtensionDirectory`, compiler hooks and registry types where they already carry the required contract. Type erasure is confined to the heterogeneous registration boundary; decode into `T` once, not through repeated `map[string]any`/JSON conversions.

Common pipeline:

1. Resolve provider, kind, namespace and effective scope.
2. Locate immutable metadata/content in the correct cache partition.
3. Forward requested working/stamped references; preserve the managing MCP's selected revision.
4. Decode/validate through the registered kind adapter when a new content generation is published.
5. Resolve declared dependencies and select the requested target from an already compiled bundle.
6. Forward get/execute to the owning MCP with current transport identity and respect its result or denial; attach per-request delivery state separately from immutable definitions. Do not insert a local resource admission callback.
7. Return a typed result through the existing MCP tools/resources and SDK compatibility entry points.

Keep window targets, report datasets/exports, menu visibility projections, skill packages and intent/template semantics explicit. Do not add one mega-interface forcing every kind to implement every operation. A server exposing only skills, or only reports and datasources, is valid. Missing capability is not an error for unrelated kinds.

## 6. Cache scope and storage plan

One scope classifier and invalidation authority serves all representations. Implement a generic immutable snapshot store, e.g. `SnapshotStore[K comparable, V any]`, only if existing stores cannot be consolidated directly. Use explicit typed scope variants rather than string concatenation scattered through callers.

| Scope | Shared with | Key dimensions | Never shared with |
| --- | --- | --- | --- |
| Public | All authorized consumers of the same declared public surface in this deployment/workspace access domain | Provider identity + canonical endpoint/transport config generation + namespace/kind + source generation/revision + format/codec version | Authenticated projections, private catalogs, arbitrary cookies or browser session state |
| Private | Same authenticated user and effective authorization context, across that user's conversations when semantics allow | Public provider dimensions + issuer/subject + tenant/account + relevant verified authorization/credential generation; session dimension only when actually session-dependent | Another user/account/credential context; anonymous requests |
| Browser MCP | Only originating user's conversation and browser connection | Provider dimensions + user + conversation/native thread + browser connection/session + catalog generation | Another conversation, tab/connection or backend execution fallback, even for the same user |

“Public vertical” means the complete reusable public chain: advertised capabilities/schema → metadata catalog → immutable definition → compiled bundle/dependency graph → read-only presentation template. It does not mean that every operation on a server with public `tools/list` is public. Classification must be explicit per surface; unknown/context-dependent data stays scoped or uncached. Internal MCP definitions may be public while visibility or execution is private.

Compose the aggregate from independent public, private and browser partitions. Do not copy the entire public catalog into every user's cache. A private merged result should hold immutable references/indexes plus its own visibility selection. Preserve provider ownership and reject ambiguous collisions rather than selecting whichever result finishes first.

### Storage and population

- Preserve `Catalog[T]`'s sorted compact indexes and namespace partitions; normalize searchable text once. Keep alphabetical name order with deterministic URI/provider tie breakers.
- Page/filter cached metadata without sorting, parsing definitions or copying the full catalog on each request. Return only the requested page. Make cursors depend on scope, source generations and query.
- Namespace-filtered requests touch only relevant partitions. Cross-namespace requests use bounded concurrency where scanning merits it, followed by deterministic merge; do not spawn a goroutine for every item.
- Add inverted/trigram indexes only if measured scans exceed the budget at supported catalog sizes. Preserve literal contains semantics for name/description/terms.
- Precompute config fingerprints, source manifests and pure schema projections once per generation. Avoid scanning/serializing every server just to determine whether a cache hit is usable.
- Coalesce concurrent population for the same key. Record wait time independently from actual population work. Publish atomically only if the source generation and effective scope still match.
- Bound memory by bytes and entries, not just a fixed page count. Expose occupancy, evictions, rebuilds and hit ratios. Never evict an in-flight population in a way that strands its waiters.
- Capture/compile native YAML assets at startup before readiness; reuse immutable bytes and target bundles. Public remote discovery prewarms after connection setup. Private metadata warms after user authentication. Browser metadata warms after conversation/connection registration.
- Compile bundles once per content/dependency/codec generation. Include target and any genuine locale/theme/feature inputs in derived keys; request authority, target proof and mutable form values are not shared bundle content.
- Avoid metadata-list hydration of every definition. Fetch/compile the selected definition; batch only genuinely required dependencies. A warm get/open must not refetch every related resource or rediscover every connected server.

### Invalidation and authority

Provider configuration/catalog changes invalidate affected generations; source revisions/content fingerprints invalidate dependent bundles; principal/account or credential changes invalidate affected private scope; browser detach/rebind invalidates that connection's execution binding. Existing server authorization and expiry/revocation rules remain authoritative. No additional business ACL or revocation polling is introduced in the proxy.

Do not let a completed cache fill republish a superseded generation. Do not promote private resource bytes or names to public storage based on successful access by one user. Do not cache permission to execute together with a compiled definition. A valid metadata hit never means a mutation or data fetch is authorized. Do not cache tool results or merge distinct user calls merely because arguments match.

Unknown upstream pagination/filtering is supported: follow advertised cursors when populating, use ordinary results for unpaginated servers, and never invent request fields for a provider. Fully cached metadata can filter locally; huge or incomplete upstream catalogs may still require provider filtering/paging through an explicit adapter. Report completeness rather than presenting a partial cache as the complete catalog.

## 7. Instrument the complete primitive path

Use one root operation trace with child spans around actual work, queues and I/O. Instrument the existing primitive methods; no new MCP protocol requirement, Datly-specific `_meta` binding or custom wire policy contract is needed. Reuse latest main's browser UserTiming, Server-Timing and phase timings.

| Stage/span | Measure separately |
| --- | --- |
| `primitive.operation` | Request arrival → response completed; kind/operation/source/scope and outcome |
| `scope.resolve` | Local identity lookup, credential selection, scope-key construction, refresh wait |
| `cache.lookup` | Cache layer/key-class hit, miss, bypass reason, stale generation, lock wait |
| `cache.wait` | Waiting behind population; distinguish leader and follower |
| `cache.populate` | One source generation fetch, metadata decode, normalization, index build and publication |
| `provider.discover` | Connection queue, initialization, capability negotiation, each list page; server count and page count |
| `source.capture` | File enumeration/stat/read/hash, bytes/files and snapshot creation |
| `definition.decode` / `definition.validate` | Decode count, bytes, codec/version and pure validation cost |
| `bundle.dependencies` / `bundle.compile` | Dependency cache hits, DAG traversal, cycle checks, compilation, target selection; compilation count |
| `catalog.filter` / `catalog.page` | Partitions/entries examined, terms, sort/merge count, results; do not record search text |
| `auth.resolve` / `ui.visibility` | Local authentication/credential handling and UI visibility only; remote IdP wait separately. Resource auth-check timing comes from the managing MCP when available. |
| `revision.response` / `delivery.bind` | Requested versus server-selected revision, response/reference integrity and per-request context binding; no client policy selection |
| `mcp.dispatch` | Serialize, transport queue, upstream roundtrip, decode, result projection; execution time belongs to the provider child span |
| `view.resurface` | Cache read/restore, canonical hydration, retained projection replay, subscription gap/attach and reconciliation |
| `view.render` | First usable content, highlights, report pending state, final report; browser render/commit timing |
| `operation.cancel` / `operation.error` | Phase, bounded reason code, elapsed work and whether any dispatch occurred |

“Resurfacing” includes reopening a window/report, restoring a conversation and replacing a view subscription. Measure it as its own operation; do not repeat discovery, compilation or tool dispatch merely to rebuild the view. Replay retained view state without replaying host actions or mutations. Preserve explicit client/native message identities and the original user scroll intent.

### Metrics and correlation

- Histograms: operation wall time, stage exclusive time, upstream wait, local queue/lock time, bytes copied, allocations and bundle/index sizes. Report p50/p95/p99 and observed maximum with sample count.
- Counters: cache hit/miss/bypass/fill/join/evict/invalidate; metadata pages; actual MCP requests; auth checks; bundle compiles; JSON round trips; listener replacements; reconciliation fetches; duplicate/abandoned populations.
- Gauges: cached bytes/entries by scope class, active populations, active browser catalogs and queue depth.
- Bound metric labels to enums and configured low-cardinality identifiers. No user IDs, conversation IDs, resource URIs, tokens, raw arguments, document text, binary payloads or secrets as metric labels.
- Correlate authenticated diagnostic traces using existing request/run/tool IDs and transport correlation. Use standard HTTP trace context where supported; arbitrary MCP servers need not implement instrumentation or a new metadata extension.
- Every node supplies an exclusive duration or explicit upstream child boundary. In concurrent fan-out, record wall critical path and per-provider durations; do not sum overlapping spans to invent end-to-end latency.
- Instrument all operations with cheap in-memory counters/timers; sample detailed traces and retain bounded slow/error examples. Export asynchronously. No synchronous logging, network export or unbounded trace allocation on a hit path.
- Expose operator diagnostics through existing telemetry; optionally register ordinary read-only MCP diagnostic tools such as `primitives/diagnostics` and `primitives/trace` via `tools/list`/`tools/call`. These are application tools, not protocol extensions or another resource kind. Return bounded metadata-only timing trees, scope-safe for the caller; aggregates across users require existing operator authorization.

## 8. Latency budget and proof

Measure two totals: (1) user-observed end-to-end latency and (2) local overhead introduced by our gateway/adapters/cache/presentation plumbing. Both must be visible in the same trace. Provider execution, actual network latency, required remote authorization and model generation cannot honestly be guaranteed below 12 ms by a local refactor. Their local preparation/serialization/queue costs still count against our overhead budget.

Provisional warm-path budget to validate, not a measured result:

| Local work | Budget |
| --- | ---: |
| Scope lookup and local auth verification | 2 ms |
| Cache lookup, contention and generation validation | 1 ms |
| Catalog filter/page or definition lookup | 2 ms |
| Cached bundle selection/dependency references | 1 ms |
| Decode/copy/serialize, delivery and local dispatch bookkeeping | 2 ms |
| Resurfacing/projection bookkeeping for the requested view | 3 ms |
| Instrumentation/export enqueue | 0.5 ms |
| Headroom | 0.5 ms |
| **Total added local overhead** | **12 ms** |

These are additive allocations for the applicable sequential critical path, not independently summed percentiles. Require warm p99 ≤12 ms and log every observed >12 ms case for investigation. This is an engineering SLO, not a hard real-time guarantee from a garbage-collected process. Do not quietly widen it to seconds or exempt local lock waits, hashing, copies or repeated auth wrappers.

Heavy cache population and bundle computation belong before readiness or in controlled background refresh. Measure them in full. A synchronous cold miss remains a slow miss and an optimization defect to address for normal interactive paths; returning a fast empty catalog is not a solution. On a genuinely unavailable first-time provider, return explicit loading/unavailable state and truthful timings rather than claiming a warm-path pass. Never cache a grant or bypass an upstream check to satisfy the budget.

Define supported benchmark sizes before accepting the target: 20/1,000/10,000 metadata entries, 1/10/50 namespaces, 1/10/25 providers, 1/16/64 concurrent callers, and small/large real window/report bundles with recorded bytes and dependency counts. Use synthetic data for high-volume tests. Record hardware, Go/Node versions, build mode, dataset, occupancy, hit rate, GC and contention profiles. Test uninstrumented and instrumented paths to prove instrumentation cost fits its allocation.

Required warm invariants: zero filesystem scans/full reads, zero bundle compiles, zero full-catalog sort, zero schema retransmission and zero upstream rediscovery unless mandated by a documented invalidation. For public metadata, warming user A must benefit user B. For private metadata, A must never expose or prime B's projection. For browser MCP, conversation A must never execute on B's connection.

## 9. Implementation sequence and acceptance gates

1. **Integrate latest main in clean reviewable snapshots.** Inventory WIP and compare overlaps. Retain newer compact-bootstrap, completion, browser-cache, scrolling, picker and denial behavior. Apply generic regression fixes to main too when independently reproduced there; never merge the boundary migration into main as a backport.
2. **Add baseline instrumentation before optimizing.** Trace list/get/open/restore/report/skill activation and ordinary/browser tool dispatch. Publish timing trees and actual call counts for cold, warm and refreshed cases.
3. **Remove duplicate producer state.** Build one neutral producer per source generation. Migrate `service/resource/local.go` facade callers; prove same definitions, visibility, errors and standalone/in-process MCP behavior.
4. **Consolidate generic registration.** Reuse the existing gateway and generic catalog. Replace authored/toolfeed/menu/report common binding loops with typed registrations; keep kind adapters and operation-specific checks.
5. **Remove redundant resource authorization and move reusable host behavior out of Agently.** Remove application/client resource policy, mapping and admission callbacks. Extract Core composition, credential forwarding and UI-visibility adaptation only; move unit tests with retained behavior and replace obsolete client-policy tests with unchanged-forwarding/upstream-denial tests. Keep startup and HTTP composition tests in Agently. Avoid a permanent layer of wrappers with the old prefixes.
6. **Implement one scope classifier and cache-generation policy.** Reconcile latest main's caches. Add public aggregate partitions, per-user private projections and strictly conversation-bound browser catalogs. Prove isolation and invalidation before enabling broader reuse.
7. **Move capture/index/bundle work out of the hit path.** Prewarm, compile immutable definitions, reuse target bundles, batch/parallelize independent metadata population within bounded limits, and remove measured repeated serialization and scope construction.
8. **Run final acceptance on committed source graphs.** Go unit/race tests, TypeScript/UI tests, stock native generation with explained/committed output, original full Endly suite and live UI/provider acceptance. Preserve original test cases/timeouts; qualify results by exact source graph. Rebuild embedded UI and prove it uses the tested SDK/Forge revisions.

Acceptance matrix:

- Every primitive uses the common gateway/producer; partial-kind MCP servers and unsupported optional methods work without disabling other kinds.
- Original Steward YAML, paths, relative datasource references, titles and existing menu actions work unchanged except separately approved fixes. No namespace/provider title prefixes.
- Public/private/browser isolation holds under concurrency, user/account switching, reconnect, changed source generations and retained stale cursors. No late population resurrection or cross-environment bindings.
- Managing MCP alone performs resource and business permission checks. Its list is filtered and its direct get/execute denial is authoritative. Prove there is no local client resource-policy evaluation, no duplicated admission check and no altered request scope. Preserve upstream denial classification and server-selected revision/cache semantics. UI visibility remains presentation-only.
- Inline and external reports render/export without window-resource errors; datasets, downloadable artifacts and artifact macros retain their authorization and no-binary-in-model guarantees.
- Early submit before history loads, view unmount/remount, stream completion and restoration preserve one user message, public highlights, pending report state and final report. No raw transcript/JSON leakage, replayed mutations or redundant completion fetches. Test user and developer modes.
- Browser MCP compact status validation and instrumentation from main remain; schema is registered once per generation, connection ownership is preserved and browser tools never fall back to server execution.
- Advertiser/campaign/order pickers, window-to-window navigation, menus, skill/intake discovery, report builder and media-plan UI work through the shared delivery path.
- Current-user and operator diagnostics expose only authorized timing metadata. Slow/error paths still produce correlated measurements without payload leakage.
- Benchmark p99 meets the 12 ms local-overhead target for the declared ready-cache envelope, with slow samples and all cold/upstream costs disclosed. Current historical timing anecdotes and isolated handler measurements are insufficient.

## 10. Deliverables and stop conditions

Deliver a smaller application root, a single generic registration/delivery path, one documented cache scope/generation policy, correlated primitive traces, a scoped diagnostic MCP surface if enabled, reproducible performance fixtures, exact commit/dependency records and a passing regression matrix.

Do not call cleanup complete merely because files moved. Do not call the branch stable based on the prior day's tests, a compiled binary alone, or a fast list endpoint while bundles, authorization, restoration or execution are still slow. The final review must identify what was removed, where each responsibility now lives, which main fixes were retained, measured overhead by phase, and which acceptance gates remain open.


## 2026-10-13 execution record and clarified provider model

Work is isolated in `/Users/awitas/.codex/work/generic-cleanup-20261013/{agently-core,agently,forge,datly}` on `ENG-54517-generic-cleanup`. The original dirty feature checkouts have not been reset. The reviewed main revisions above were merged into these clean integration worktrees; that does not mean the original worktrees have advanced.

### Governing provider model

- One existing generic gateway consumes internal in-memory MCPs and external MCPs through the same ordinary tool/resource contracts. It does not authorize provider resources.
- Reusable provider infrastructure is now `service/resource/provider` (renamed from `primitiveproducer`), with current-user ownership/storage under `service/resource/provider/user`. YAML and stored content are source adapters, not separate gateways.
- Reports have one list/get interface across namespaces. Save targets the authenticated canonical user's `report://users/<canonical-user>/<path>` namespace. The managing user provider enforces writes; the client does not evaluate a local resource ACL.
- User namespaces can support windows and other kinds using the same generic infrastructure. That extensibility is not a claim that user-window authoring is implemented.
- Existing generic `List[T]`, `Get[T]`, `ListPage[T]` and `primitive.Catalog[T]` are reused. Shared pagination should coordinate loading, cache ownership, coalescing, cancellation, limits, filtering and sorting. It must not create a second cache per resource kind.
- Partial providers are valid. Supported kinds and operations come from namespace discovery; generic MCP servers need not implement all application primitives or optional MCP features.

### Completed commits and current evidence

- Core `03144da5`: removed window/report consumer admission callbacks and obsolete callback APIs/tests, preserved managing MCP denial and UI visibility. Includes the actual-Builder window compatibility fix and the three skill activation fixes. Focused and race checks passed; this is not full release certification.
- Core `32c24e9c`: reusable static/dynamic catalog and explicitly declared authoring operations, package move to `resource/provider`. Same in-memory and HTTP MCP handler, no per-call provider reconstruction. Renamed provider race tests passed.
- User source now passes ownership, exact-byte persistence, filesystem reopen and native SQLite tests. Its private persistence context uses canonical identity for new rows and the verified legacy alias only for existing rows; caller credentials/context are unchanged.
- Actual Builder integration proves save through user MCP, common get/list, plain update, cache invalidation, foreign-user denial and rejection of writes outside the user namespace. The primitive report list migration to the shared cached paginator is still being integrated.
- Timing instrumentation records bounded metadata only. Provider observer microbenchmarks show approximately 0.77 microseconds added to a dynamic Get with unchanged allocations. These are microbenchmarks, not proof of the 12 ms end-to-end local-overhead target.
- No implementation has been pushed or deployed. Main received only the separately verified Core stream-completion backport `6e664c44` earlier; resource-provider changes remain on the integration feature branch.

### Benchmark requirements now being measured

Use the real `viant/mcp` client/server protocol, not hand-written JSON-RPC simulations:

1. Five servers, 100 tools each; a documented subset has 200 input properties and 200 output properties per tool.
2. Compare server page sizes 10, 20 and 100; one versus four workers over independent servers. Opaque `nextCursor` chains remain serial.
3. Namespace benchmark: each server has three namespaces with 30 resources each, distributed across the ten currently supported kinds (window, report, datasource, intent, skill, template, prompt, eval, menu, toolfeed). Providers implement different subsets.
4. Compare namespace-first discovery with bounded independent namespace enumeration. Standard `tools/list` must not be misrepresented as namespace-filterable; application namespace catalogs are ordinary registered MCP tools.
5. Record cold versus warm timing, allocations, payload bytes and actual RPC counts. Distinguish cached list snapshots, fresh external get, preloaded internal definitions, and in-memory tools metadata cache bypass.
6. Include partial-capability and unpaginated providers, protected/public scopes, concurrent cold callers, mutation invalidation, slow/error/canceled providers and invalid cursors. Report percentile evidence with sufficient samples; a three-repeat microbenchmark is not a p99 claim.
7. Measure on a quiet machine window; label injected network latency separately from real loopback measurements. Decide whether concurrency is useful before changing production defaults.

### Still required

Finish report save/update/export and original-workspace compatibility, move remaining list consumers onto the generic paginator, complete timing coverage and benchmark interpretation, resolve remaining broader integration failures, and run full regular Steward/UI acceptance on the final source graph. History, streaming user/dev modes, reports/visuals, pickers, window navigation, intake/skills, media plan and later Studio parity are not certified by the new narrow tests. Recheck current main before final integration and preserve concurrent work.

### Integration verification update — October 13

- Latest integration merges include Core main `958f76671f1cf1828ff41c46a6d6c6023fa5c7e2`, Agently main `1005e6bbaf4a2a8d54a820ed6f28844ed2ba4098`, and Forge main `0597fac893974348cff3f5f3228a3a472b2f3276`. Source composition remains the isolated `go.work`; these are not published dependency pins.
- Core `7cc9ea6a` integrates the user report source, shared report list pagination, exact definition persistence and observer/cache work. Quiet full reporting/user race tests passed. Core `19c88b0a` scopes resource notifications to provider and current private principal, preserves other users/providers, separates resource/tool generations, and fences in-flight publication. Real MCP event and race receipts are in the integration work directory.
- Namespace-first real MCP benchmark (five servers, three namespaces each) reduces discovery from 247.825 ms to 4.519 ms with four workers; response bytes 4,796,271 to 13,110. Refreshed event-aware run measured 5.095 ms discovery, 23.690 ms cold list and 1.252 ms warm list with zero warm RPCs. These sample medians do not prove the p99 overhead SLO.
- Core `52c28941` preserves conversation-owned inline report events without resolving a nonexistent window. Events/registry races passed. A full Core compile-only test sweep passed after removing an obsolete fixture callback; this does not mean all tests ran.
- Actual Agently executable built. First launch with all 1,399 original tracked Steward workspace files unchanged failed on the datasource authoring `template` key. Loader compatibility is being fixed using existing template/import semantics, not by changing the workspace YAML.
- The expanded datasource test run exposed export worker fixtures missing canonical authenticated user context. The managing MCP pagination paths succeed; worker restore rejects the incomplete pinned-job identity. The integration fixture and precise acceptance coverage are being corrected without relaxing that boundary.
- Full UI suite initially had 1,227 passes and 12 failures across five files. Current investigation identified stale test imports/temporary fixtures and duplicate React resolution through linked packages; the full green run and subsequent embedded rebuild remain required.
- Audit found a remaining optional consumer datasource policy callback (`WithPrimitiveDatasourceAdmission`) with no production callers. It is being removed; generic content/argument integrity checks and managing MCP denial remain. Other remaining host adapters still require responsibility/file-hygiene review.

Live authenticated Steward acceptance has not yet passed. Do not infer it from backend compilation, narrow races, fixture corrections or benchmark numbers.

### Unified MCP definition loading — proposed final integration

The eleventh primitive is `mcp://<namespace>/<name>`. Its definition is the existing Agently `config.MCPClient` model, available through ordinary `mcps/list`, `mcps/get`, and YAML `resources/read`. Preserve complete authored OAuth/client configuration and secret references; never resolve credentials into resource responses. Namespace capabilities advertise this kind independently of other primitives.

All execution configuration should follow one path:

`internal captured YAML or external MCP catalog -> generic resource gateway -> definition reader -> existing manager provider -> existing connection manager`

- Keep the existing provider's transport normalization, environment handling and per-user credential-store attachment. Replace its direct workspace repository reads with a definition reader. Workspace files become internal provider inputs, not a parallel runtime configuration path.
- Bind discovery to explicit catalog roots. A small set of root connection seeds breaks the bootstrap dependency cycle; root enumeration must not call the aggregated manager name listing recursively. Seeds are connectivity inputs and must never become fallback definitions after an owning catalog denies or removes a resource.
- Preserve original service names as aliases for canonical MCP references. Duplicate aliases across roots fail explicitly; do not pick a catalog by iteration order. Reuse the generic cached list/get machinery for alias inventory and definitions.
- Standard resource notifications invalidate the relevant source generation. Reconcile additions, updates and removals lazily outside notification callbacks. Compare effective configuration fingerprints before reusing a pooled client; changed definitions retire old clients and removed definitions cannot dispatch. Do not automatically replay mutations.
- Scope public metadata globally, private metadata by current authenticated user and effective credential generation, and browser metadata by conversation and connection. Provider authorization remains authoritative. Runtime transport and credential handling remain host responsibilities.
- A private catalog's discovered registration remains private even if its definition requests public caching. A child definition cannot widen the scope of the source that supplied it or overwrite another user's OAuth client/provider registration. Public sharing requires an explicitly public root and context-neutral metadata.
- One lifecycle serves both internal and external definitions. Do not add a second registration primitive, per-kind gateway, bespoke notification protocol or per-HTTP provider read.

Acceptance: unchanged original aliases/YAML, two external roots, partial capabilities, resource add/update/remove, endpoint/config changes, bootstrap cycles, alias collisions, terminal provider denials, user isolation, OAuth rotation, and actual tool dispatch against the selected revised endpoint. Publishing the `mcp` resource alone does not satisfy this lifecycle.

### Current measured state — October 13, cache candidate

- Asset-manifest polling has been removed and committed. Captured source generations change through explicit reload/notification/restart; ordinary Gets perform no filesystem scan.
- Full resource and MCP-manager race suites passed in the composed integration tree (21.987 s and 17.292 s). HTTP UI and primitive-provider suites also passed (4.128 s and 3.524 s). These are source-composition checks, not release acceptance.
- Live authenticated chat in the cache candidate rendered correctly. Its first measured window metadata request was 42.663 ms; its next warm request was 11.475 ms with zero provider Gets and approximately 0.0015 ms of cached definition-decoder overhead. Two samples are not percentile evidence.
- Live testing caught a remaining sidebar/layout 503 and missing slash lookups associated with startup resource-revision denial. This candidate is not stable until that cause is fixed and menus, pickers and reports are retested.
- Production-style private HTTP caching still bypasses metadata reuse when opaque cookie/OAuth transport hooks lack an effective credential witness. The next change must supply a reliable witness and rotation tests, not remove isolation or invent a metadata lease.
- Dynamic MCP definition publication is implemented and narrowly tested; convergence of actual runtime loading onto the definition reader remains unfinished.

Follow-up source verification: zero-metadata-expiry layout regression now has a failing-before HTTP 503 and passing-after HTTP 200 test through authenticated default Builder composition. Menu visibility remains cached until the managing provider's standard resource notification. Production-style cookie/OAuth private-cache tests pass, including credential rotation and user isolation; unsupported custom credential stores remain uncached. The Agently host window/embedded native HTTP regression group passes (15.950 s). Browser confirmation after the final rebuild is pending renewed Viant sign-in, because the earlier local session expired. This remains an open acceptance gate.

Dependency implementation commits on isolated `ENG-54517-generic-cleanup`: JSON-RPC `4f2f913` (final session cleanup), MCP `df0db79` (session-close hook), MCP `18d964c` (effective credential fingerprint with detached public token/client fields). They are not pushed and consumer module pins are not yet published; test composition uses the isolated `go.work`.

### Confirmed additions: lookup primitive and external namespace indexes

Lookups were missing from the primitive inventory. They are a first-class `lookup://<namespace>/<path>` resource, using ordinary `lookups/list` and `lookups/get` through the same gateway, paginator and scoped cache. Existing `extension/forge/lookups/*.yaml` overlays remain authoritative and unchanged. Preserve named tokens, target matching, priorities, picker references and input/output mappings. Inline window lookup metadata remains compatible. Primitive-mode SDK lookup discovery must not fall back to scanning the workspace.

External lookup datasource and picker references retain their owning namespace/provider. A foreign dialog must never resolve through an unrelated local workspace dialog. Existing dialog/model assets are window-bundle dependencies, not automatically new primitive kinds. Verify empty and populated picker flows in the live app after the registry adapter is complete.

For external MCP discovery, prefer a complete, paged namespace resource index when the provider advertises one. The gateway can answer its existing `windows/list`, `lookups/list`, and other kind-specific methods by filtering the same cached namespace index. Definitions still use the owning advertised Get method when absent from the definition cache. A namespace response containing only supported kinds is not a complete resource index: such providers continue through their advertised kind-specific List methods. Do not change standard MCP request schemas, guess an optional method, or create separate caches for these paths.

Actual runtime loading now has the common definition-reader seam and Builder binding, with a configuration-only in-memory MCP as bootstrap. Canonical references and original aliases share transport/token identity; SDK handshake names remain separate. `mcpCatalogs` optionally lists canonical references to configuration catalogs; it contains no duplicate endpoint/OAuth settings. Captured-file-removal and empty-catalog tests pass. Remaining acceptance includes complete lookup delivery, latest-protocol notification subscriptions, scoped inline OAuth registration, full composed regression checks, and live Steward/Studio verification. Narrow passing tests do not certify the current integration branch as stable.


### Corrected responsibility boundary — October 13

The managing MCP alone decides resource availability and data access. Agently/Core must not add resource/entity/account/parent/action authorization preflight. They retain authentication and credential forwarding, current-user/session cache isolation, selected-source and argument integrity, upstream errors, and UI control/tab visibility based on current roles/exposures. Exposure is a rendering input, not a newly invented role.

Consumer window admission/bootstrap and report/datasource policy hooks are being deleted, including obsolete APIs rather than inert stubs. The real SDK-to-gateway-to-WebSocket regression proves arbitrary selected IDs and arguments forward unchanged, role changes only affect rendering, and upstream Get denial remains terminal. Final composed tests and rebuilt live Steward verification remain required.

The shared namespace contract now lives in `github.com/viant/mcp-protocol/extension/namespaces`, local commit `cc52f4b7738cc870dfaa1f6c19c7d0330622830d` (not pushed). Studio and Core share ordinary namespace catalog tools and generic resource-get, without changing MCP wire methods. Namespace IDs are opaque and separate from display names. The owning MCP filters before pagination; Core caches a complete paged index and derives kind-specific gateway lists. Legacy declared-kind discovery remains supported. Package race tests pass; Core consumer adaptation is still in progress.


### Verification update — October 13, projection candidate

- Shared namespace contract agreed with Studio and locally committed in MCP-protocol (`cc52f4b`). Studio is adopting the exact DTOs via an explicit temporary go.work; publication/pins remain outstanding. The root Studio catalog should publish current running namespace MCP configurations, exclude its own connection, and emit lifecycle resource notifications. MCP configuration responses contain configuration only, never editing file/folder snapshots.
- Full Agently UI suite: 162 files, 1,245 tests passed. Embedded UI rebuilt.
- Consumer window bootstrap and report/datasource policy hooks are removed in the integration source. Live advertiser verification exposed a remaining projection defect: ordinary `windowForm` conditions were rejected by an authorization-only validator. Mixed form/authorization condition regressions now pass; the corrected test server is starting on 30423. Live confirmation is still required.
- Scoped lookup discovery previously enumerated unrelated MCPs before filtering by owner. It now uses the selected provider through the existing bounded generic catalog. Full live lookup verification remains pending.
- Report profiling identified repeated full-builder cloning per dataset-backed block. Forge commit `b8699ea` caches dataset preparation within a lowering invocation; Core `1e4075d6` embeds the regenerated official lowerer and benchmarks. Initialized lowering benchmark: 181.6ms/138.9MB to 135.8ms/104.1MB. These figures exclude VM initialization and are not HTTP or end-to-end latency.
- Former report timeout regressions now pass without raising deadlines: actual authored datasource execution and pinned compilation (47.684s test group), exact-resource export (8.950s). Full materializer race/parity suite passes (85.630s). All ten existing report golden outputs retain parity; read-only regeneration verifies bundle bytes.
- Configured host framework windows now register their exact host-owned native HTTP bindings; real runtime/canonical+alias HTTP regression passes (6.361s). External providers cannot register those host transports through resource payloads.

These are integration receipts, not a release sign-off. Full composed backend checks, authenticated window/picker/report/history flows, source/package cleanup, dependency publication/pinning, and Studio UI parity remain open.

### Shared resource and skill integration — October 14

The namespace index advertises the primitive resources available from each owning MCP. Agently's generic gateway derives kind-specific list/get views from a complete cached index and reads definitions from the selected owner. This is a delivery contract, not an authorization layer: the owning MCP decides which entries and definitions the caller may see.

Skills use the public Agent Skills `SKILL.md` format and the native MCP Skills extension (`skills/list`, `skills/get`, and `resources/read` for files). The namespace index can advertise `skill://.../SKILL.md` for cross-primitive discovery, but it does not replace those native methods or introduce an Agently-only skill format. Datly's isolated v1 integration branch removed its duplicate ordinary-tool skills bridge; the native extension and full MCP race suite pass. The Core in-memory publisher and consumer are being aligned to the same single source.

The shared namespace-contract commit `cc52f4b` has now been pushed and pinned at `v0.19.1-0.20261014011736-cc52f4b7738c` in the isolated consumers. Datly's generic handler guard, package-preload APIs and native-skill cleanup are published on feature branch `ENG-54517-generic-cleanup` at `59523a9a` (`v1.1.1-0.20261014035505-59523a9a6c21`); Core and Agently pin that revision, while Studio's final dependency pin remains pending. The affected Core resource/primitive-provider/HTTP-UI race suites and Agently root tests pass against the local integration graph. Datly's focused native Skills and embedded-binary tests pass; the full transcribe race package runs generated child modules and exceeded a 20-minute timeout, so it is not a full-suite pass. Authenticated local testing opens the picker frame, but both Steward MCP endpoints and the creative MCP endpoint currently time out from this host; live data acceptance cannot be inferred from the local fixture tests. The final UI suite passes 163 files and 1,248 tests after the floating-window and close fixes.

### Authenticated integration update — October 13 evening / October 14 UTC

- VPN connectivity returned. The deployed Steward and creative MCP URLs answered quickly with HTTP 405 to GET, and authenticated local Steward on port 30424 displayed real campaign and order picker rows. Exact campaign 555307 and order 2702608 selection, reopening, and chip retention were verified. Forge `3f12790` preserves the existing resource query when appending target parameters; it fixed the earlier picker 404.
- A fresh report turn exposed `catalog cursor does not match current snapshot` while resolving the internal `workspace_ui` intent. An unrelated MCP `tools/list` refresh was clearing all visible catalogs mid-read. Core now scopes tool-catalog invalidation to dependent sources and scopes category snapshots to providers that actually advertise that kind. The manager regression and Core manager/resource/executor race suites pass. The category-gateway change in `service/resource/catalog.go` remains part of the larger uncommitted integration set and must be committed with its catalog-root dependencies.
- Core `ca58509e` lets a provider-admitted intent retain its own declared bundle while a separately selected runtime bundle comes from the captured toolfeed. This removed the `advanced-reporting-tools` unavailable failure without making Core authorize the provider's tools. It also avoids rediscovering an MCP for ordinary tools with no output-artifact policy.
- Steward advertises private `tools/list` with `ttlMs: 0`. Repeated bundle/skill matching had fetched its large schema catalog many times in a single binding. Core `477210bd` reuses an authenticated, generation-keyed read snapshot only during one binding; independent requests still fetch again. The BFF transport's existing credential fingerprint now participates in that scope. Full manager and agent race suites passed, with a separate zero-TTL/token/generation regression.
- The authenticated report library showed **0 saved reports** and **6 presets**. “Performance Delivery Command Center” is a preset, so asking for a *saved* “Performance Delivery” report cannot resolve an ID. The preset workspace opened and rendered spend/impressions and detail rows. The campaign picker selected 555307 (WHOOP), the selection survived a browser-tab restart, and the seven-day leading spend changed from $1,054,806 unfiltered to $32,746 with the campaign filter. The requested October 1–8 date range was not exercised; the preset remained on its default October 7–13 range. Do not report the exact requested run as accepted.
- A standalone preset emitted `report.run_start` with a local window ID but no admitted window key, causing a nonfatal 400 telemetry warning. Agently `839da67e` suppresses only those unpaired durable chat events; paired bridge-window events remain. Targeted UI tests and embedded UI build passed. No data or durable-run authority is inferred from this telemetry fix.
- The local request reached the report workspace in roughly 49 seconds including model/tool work. This is **not** a 12 ms gateway measurement or release sign-off. Local metadata, model, upstream MCP, report SQL, and browser rendering timings still need separate final-source attribution. AI Studio's native Datly MCP/UI publication remains under its separate agent; do not wire management tabs until its descriptor and concurrent-session tests pass. That agent found a likely cross-session JSON-RPC request-ID cancellation in `viant/mcp` and is fixing it upstream.

### Five-server real-MCP benchmark receipt

The explicit `TestPrimitiveGatewayBenchmarkFixtureSmoke` and `TestPrimitiveGatewayBenchmarkMatrix` passed on the current integration source graph. The fixture uses five actual `viant/mcp` HTTP servers, 100 tools per server (20 with 200 input and 200 output properties), three namespaces per server, partial kind support, and 450 visible resources across ten kinds. The matrix ran three repetitions per page-size/worker setting with no injected RTT. For page size 20 and four workers, medians were 4.04 ms for cold namespace discovery (25 RPCs), 26.22 ms for cold list (30 RPCs), 1.48 ms for warm list (zero RPCs), 96.91 ms for 450 first Get calls (450 RPCs), and 8.80 ms for 450 repeated cached Get calls (zero RPCs). One-worker warm list median was 4.74 ms. These are aggregate fixture timings, not per-item times, WAN latency, production p99, or proof that every UI operation meets the 12 ms local-overhead target. The exact run log is outside the repository at `/Users/awitas/.codex/work/generic-cleanup-20261013/primitive-bench-current.log`.

The separate 20 ms-per-request injected-latency run also passed. At page size 20, one versus four workers reduced median cold namespace discovery from 552.1 to 219.2 ms (25 RPCs), cold list from 702.8 to 342.0 ms (30 RPCs), and a 30-item fresh-Get sample from 674.5 to 178.1 ms (30 RPCs). Warm list remained 4.8 versus 1.5 ms with zero RPCs. This demonstrates bounded concurrency benefit in the synthetic fixture; it is not measured WAN latency. The exact log is `/Users/awitas/.codex/work/generic-cleanup-20261013/primitive-bench-rtt-current.log`.

### Standalone build and publication receipt

The integration branches were published through IntelliJ without changing any `main` branch: MCP `4cd46ac`, Forge `711acf4`, Authz `7814ee9`, Core `9e04bd0e`, and Agently `bb3a0aa3`. Core publishes the nested Resource module at `664c53d5`, Primitive at `1f9a5816`, and Window at `bbbdb0ee`; its root and Agently pin those published revisions. Core's full package compilation now passes with `GOWORK=off`. Agently's standalone binary build and full package compilation also pass with `GOWORK=off`. This resolves the earlier local-workspace-only imports of `protocol/primitive/observe`, `protocol/resource/user`, `protocol/window.PresentationInput`, and `authz/oauth` HTTP providers. Focused MCP server/client race tests, Core manager/resource/executor/agent/reporting/datasource/skill/SDK/UI race suites, Forge window tests, and Agently's non-e2e Go suite passed on the integration graph. The broad Agently suite was stopped when it entered live-LLM e2e; it is not claimed as a complete pass.

The local report library opened with 0 saved reports and 6 presets. “Performance Delivery Command Center” is the matching preset. After selecting campaign 555307 through the real picker, its completed dashboard showed $32,746 leading seven-day spend and 1.5M impressions; the unfiltered dashboard had shown $1,054,806 and 47.6M. The UI rebuild also removed the standalone report's nonfatal unpaired `report.run_start` 400 warning. The preset still used October 7–13; October 1–8 remains an explicit acceptance gap.

The published graph was then tested without `go.work`: nested Resource, Primitive, and Window modules passed their standalone tests; Core compiled every package with `GOWORK=off`; Agently built its binary and compiled every package with `GOWORK=off`. A separate standalone binary on local port 30425 loaded the authenticated Steward navigation and a real Campaigns window with 45 pages of rows. Its test MCP listener is on 5003; the existing 30424 integration server was left untouched. The full live-LLM e2e suite remains outside this standalone smoke.

AI Studio and Datly Studio remain separate authoring applications. The Studio agent has verified basic MySQL component publication and cross-database lookup through both native Datly and AI Studio MCP surfaces, but its larger UI/policy matrix and finalized hosted-app descriptors are pending. Management-tab integration into the DSP shell must use the published `window://`/`ui://` and menu/MCP primitives after those gates, with no hardcoded fixture URL or Studio code copied into Steward.

### Local workspace follow-up — October 14

- The Performance Delivery Command Center preset deliberately declares Today, Yesterday, and trailing-seven-day dataset overrides in Steward's unchanged `performance/builder.yaml`. A temporary Forge menu control appeared to accept October 1–8 while the detail table still showed October 11–14, so it was reverted on the isolated feature branch (`a169165` followed by `4308792`). The final source keeps that override hidden; the exact October 1–8 report request remains unverified.
- The published standalone Agently build on 30425 restored a saved conversation to visible content in 3.1 seconds and opened its Reports workspace. This is a single local sample, not a p99 claim.
- Focus-mode workspaces intentionally cover the global sidebar (`.app-summary-surface-shell` is fixed at z-index 100). Browser automation initially clicked an obscured sidebar button and appeared to show a navigation failure. The actual visible workspace tabs switch among Reports, Advertisers and Campaigns; returning to chat exposes the global menu. Diagnostic edits were removed and the Agently checkout is clean. The fullscreen menu visibility question is a UX decision, not evidence of an MCP dispatch regression.

### Navigation identity, standard MCP resources and history latency — October 14

- `navigation://<namespace>/<path>` is the canonical navigation URI; `app://` is reserved for an application descriptor. The user explicitly chose standard MCP Resources as the required interoperable read path: `resources/list` and `resources/read` with `params.uri`. `resource/get` and `resources/get` are custom names, not MCP resource methods. Optional namespace and per-kind tools remain ordinary tools, never prerequisites for reading a standard MCP resource.
- Core feature commits `9b6d9dcc` (navigation kind/URI) and `2b181db7` (nested primitive pin) were published. The later correction `1840734e` changes `Plural("navigation")` to `navigations`; Core performance commit `2c0ea6a3` and Agently commits `5c4d82cd`/`ec4f7661` remain local pending IntelliJ publication and repinning. The AI Studio and Datly Studio agents were notified of the standard method and navigation identity.
- A local server profile found 4–7 ms for native conversation reads but 8–10 seconds for synchronous toolfeed catalog discovery on a settled history read. The initial history request now sets `includeFeeds:false` and skips dynamic feed lookup during compact tool-payload projection; the existing native observer hydrates feeds separately. On the isolated 30426 build, saved chat content appeared in 175–269 ms and another report-related saved chat in 368 ms. These are local click-to-visible samples, not p99 or WAN measurements. Focused UI tests (111) and Core SDK transcript tests pass.
- The isolated Core source now publishes provider-owned definitions through standard `resources/list/read`, and the gateway consumes resource-only MCP servers without requiring namespace or kind tools. A real MCP wire test covers `navigation://`, `window://`, YAML `mcp://`, owner denial, and `ttlMs:0` re-listing; namespace-index resources/read is also tested. Existing Steward provider paths retain their established tool contract during transition. Core resource/workspace races and focused report/skill executor races pass. This standard-resource implementation is not yet committed/pushed or verified in the deployed environment.

### Boundary branch integration receipt — October 14

`primitive-mcp-boundaries` is the active branch. The earlier generic-cleanup checkouts were fast-forward staging copies; their tested commits have been brought onto the active Forge, Core and Agently branches. Current published remote heads: Forge `904778e4`, Core `af32c41b`, Agently `15b971ac`. No `main` branch was changed. The original uncommitted boundary work was saved in recoverable Git stashes before the fast-forwards, then non-conflicting changes were reapplied. Dirty hosted-app and source-authoring work remains in the active checkouts and is not included in those published commits.

The standard MCP read path now uses `resources/list` and `resources/read`; custom namespace and kind tools are optional. Core separates a zero list-cache TTL from an actor's read lease. A real resource-only MCP test covers list/read, owner denial and immediate re-listing; an explicit multi-page standard-resource fixture is still needed. The local authenticated Steward run on 30426 loaded the unified report catalog, Advertisers and Campaigns data, and Campaigns list-to-detail navigation. Saved history opened visibly in a few hundred milliseconds in local samples. A standalone binary built with `GOWORK=off` from published Core and Forge revisions served the matching embedded `deployment/ui` asset hash and restored a campaign detail without a window error.

The deployed campaign-spend conversation `dc1b5dd2-9064-455f-a823-3eea1052cac9` exposed a separate steering delay: an old runtime turn hint could steer a follow-up after the canonical chat turn was terminal. Agently now selects steering only from the canonical active turn, otherwise uses the normal query/queue path; rejected steering clears the optimistic row. Core bounds synchronous steering acceptance to 15 seconds. The focused UI tests (37), SDK steering race test, UI production build and standalone Agently build pass. These commits are published to the feature branch, not deployed to production.

Startup no longer probes user-scoped window/tool catalogs with a background identity. The previous `resource revision denied` startup warnings disappeared on the local restarted binary; authorized window and report requests still work. Remaining proof for the full objective includes latest-binary report execution, lookup and toolfeed flows, hosted Datly Studio/AI Studio end-to-end parity, source-manifest publication and package pins across Studio, cache/event benchmarks, and broad regression checks. The feature branch is not certified release-ready yet.

### Standard MCP pagination correction — October 14

A three-page real MCP resource fixture exposed non-deterministic `resources/list` ordering in the shared default resource registry. MCP-protocol now sorts resource and template URIs before listing, returns complete/private result metadata, and uses JSON-RPC invalid-parameters for a missing `resources/read` URI. Protocol server/namespace race tests and Core's paged resource race test pass. Published feature heads: MCP-protocol `af92f34`, Core `8014dff2`, Agently `7e952a58`; Forge remains `904778e4`. Agently's standalone Go build passes with these published pins. The embedded UI hash was verified from the built binary and its campaign detail restored locally. Production deployment remains separate.

### Linked Datly v1 and broad UI verification — October 14

Core now pins Datly v1 `a220d43f` and removes the deleted `PreloadPackages` selector, retaining linked-only package indexing. Datly owns eager warmup selection from authored warmup/async/cache configuration. Core's native-store package passed its full suite and race run; Agently built standalone with published Core and Datly revisions. Published heads: Core `0a00f427`, Agently `71704650`, MCP-protocol `af92f34`, Forge `904778e4`. A fresh local embedded binary with these pins started on 30426, restored campaign detail, and showed no unauthenticated catalog warmup denial. The clean published Agently UI source passed 163 files / 1,260 tests. These receipts do not establish production deployment or Studio parity.

### Authenticated report discovery and measured gateway cost — October 14

The local Performance Delivery Command Center initially created a fresh synthetic MCP discovery scope for each conversation-less datasource request. The pre-fix server log recorded 102 such fallbacks while the report remained empty. Core now reuses a scope keyed to the authenticated user and credential for private catalogs, and a distinct public scope for public catalogs. Different users and rotated credentials stay isolated. The Core registry and manager suites pass under the race detector. Feature heads Core `7cb43d88` and Agently `1835db4c` are published; the latter pins the former. A local build using the fix returned campaign-filtered report rows, including spend and impressions, and its subsequent report refresh completed in approximately 3.2 seconds. The post-fix log had no synthetic-scope fallbacks. This is a local functional result, not a production deployment or a claim that the full refresh meets the 12 ms local-overhead target.

The explicit real-SDK fixture validated 5 HTTP MCP servers, 100 tools per server, 3 namespaces per server, 450 resource entries across 10 kinds, and roughly 1.09 MB of definition payload. Three repeats of the zero-delay matrix produced median warm list times of 2.39 ms at page 10 and 1.47 ms at page 20 with four workers; warm fresh-get across all 450 entries took 8.67 ms at page 20. Cold discovery took 4.19 ms and cold list 24.09 ms at page 20 with four workers. A separate simulated 20 ms per-request delay yielded 344 ms cold list versus 2.03 ms warm list, and 176 ms for 30 fresh gets versus 1.12 ms repeated gets, with four workers. These figures are synthetic fixture measurements, not WAN or browser end-to-end latency. Raw results: `/tmp/ENG-54517-gateway-benchmark.log` and `/tmp/ENG-54517-gateway-rtt.log`.

The real-SDK `notifications/resources/list_changed` fixture also passed add, update, remove, and 10-event burst cases. Cache invalidation took 1.18–1.52 ms and the next visible catalog took 5.03–6.02 ms in this local fixture. Both notified clients rejected their old cursors; an unrelated provider retained its generation and cursor. The burst did not cause 10 separate provider refreshes. Raw results: `/tmp/ENG-54517-gateway-events.log`.

### Notification parameter regression and dependency pins — October 14

A full Core resource race run exposed two first-list timeouts on servers advertising standard change notifications. The server received `subscriptions/listen` and emitted `notifications/subscriptions/acknowledged`, but `jsonrpc.Notification.UnmarshalJSON` discarded the notification `params`. The manager therefore could not read the accepted filter and waited for its five-second deadline before any resource list. JSON-RPC `5f5e9b1` preserves raw notification parameters and has a regression test for payload retention, absent parameters, and invalid notification shapes. Its complete Go suite passed. With this revision, both formerly failing real-SDK tests and the full Core resource and MCP manager race suites pass. MCP `4ede589` pins JSON-RPC and MCP-protocol `af92f34`; Core `b1813540` pins MCP and JSON-RPC; Agently `c43f1fec` pins the resulting Core/MCP/JSON-RPC graph and builds standalone. These changes are on `primitive-mcp-boundaries`, not production.

### Latest main sync and clean embedded UI — October 14

Core `6daeb70a` merges the four newer main continuation commits. The merged SDK, AG-UI store, Datly AG-UI manager, resource gateway and MCP manager Go suites pass under the race detector; the SDK TypeScript suite passed 681 tests with 5 skipped. Agently `ba90072a` merges the newer main embedded-UI commit, pins merged Core and replaces generated assets with a build from clean, committed Agently, Forge and Core checkouts. The clean UI suite passed 1,260 tests across 163 files; standalone Go build and focused native-window/toolfeed tests passed. The bundle's referenced entry assets exist and includes the report data-view progress string. Uncommitted hosted-window work in the active checkouts was not included in that asset build. Local authenticated report timing and Studio parity are still unverified; no production deployment is claimed.

### Hosted report preservation during window drilldown — October 14

An authenticated local run on 30426 exposed a navigation defect while an eight-view report was loading: opening Campaigns and then a campaign detail removed the report tab. Reopening the report through its chat attachment restarted progress at 0/8. Forge's `openTarget` requested hosted-region replacement, but `addWindow` selected the first window with that conversation, parent and region, which could be an unrelated report. Forge `343de5d` now passes the source window ID and replaces only that matching window; legacy callers without a source ID retain their existing region-replacement behavior. A focused regression test and the existing window and UI-command tests pass.

Agently `31538042` pins Forge `343de5d` and embeds a clean UI build. The remote feature heads match those commits. In the fresh authenticated build on 30427, opening Campaigns and drilling into campaign 563637 left Performance Metrics as an open tab. Returning to it showed 3/8 views ready rather than a fresh 0/8 request, and the report finished with the scoped $601 spend headline and detail rows. This verifies the local window-to-window/report case; it does not certify production deployment, Studio parity, or the remaining report-latency target.

### Hosted app and opening-admission integration — October 14

Agently `07878a13` registers hosted `mcpApp` windows, reads their installed `ui://` resource with the current window pin and selected target, and embeds the matching clean UI bundle. Core's ordinary POST resource reader validates that context before the read. A clean source graph passed 1,268 UI tests; an authenticated local Steward run kept the report mounted through Campaigns drilldown and returned with 4/8 views ready. No live Studio hosted-app descriptor was available in that workspace, so this is not Datly Studio or AI Studio parity proof.

Agently `f5c3b15d` forwards a complete signed admission frame from the active metadata signal when its window store contains navigation only. The clean bundle passed 1,269 UI tests. Forge `b8a3c8f` scopes opening-only UI behavior to protected windows, retains local business-form validation, delegates resource authorization to the managing MCP, and keeps 401 as an authentication failure. Its clean window render suite passed 57 tests; command and datasource authorization contracts passed. Agently `d6212fd1` pins that Forge revision and corrects Vite's reporting-chunk matcher to use the configured Forge root. Its clean UI suite passed 1,269 tests across 165 files, and a standalone binary built with published Core `ca3c50dc` and Forge `b8a3c8f`. The local authenticated run on 30430 loaded Campaigns page 1 of 45, opened campaign 563763, and reported no browser errors. The test listener's secondary MCP port was already occupied by another local server, so this run verifies the app's existing external MCP reads rather than its own listener.

Remaining release gates include a real installed Datly Studio and AI Studio hosted window in Agently, end-to-end menu and datasource navigation through those providers, the current report-latency target, and the remaining root-package consolidation. The UI and fixture tests above do not establish those gates.

The same authenticated 30430 build retained Performance Metrics while opening campaign 563763. Returning to the report tab displayed its existing scoped $601 spend and 27.9K impressions without a new request. A manual Run on October 14 showed progressive dataset status (0/8 initially, 5/8 at about 42 seconds, 6/8 at about 55 seconds), then cleared the progress notice and retained the result by about 65 seconds. The report path is functionally complete in this local sample but still far too slow. Forge currently schedules at most two dataset requests from one report (four globally) so other reports and ordinary windows retain capacity. These observations do not isolate warehouse execution from gateway, HTTP, or browser costs; preserve that fairness limit until correlated request timings show where the minute was spent.

Opt-in API timing on the identical workspace isolated the delay before datasource service execution. With Core `ca3c50dc`, the eight report datasource HTTP requests took roughly 11–20 seconds each while the service/backend span was usually 0.1–0.7 seconds. The generic alias resolver scanned 398 visible definitions in four local pages; every page rebuilt capability discovery, and MCP connection options repeatedly re-read `mcp://` definitions. Core `ff4a7602` adds request-scoped, user/credential-keyed name and authored-definition reuse, an owner-notification/connection-generation fence, one bounded 1,000-entry alias page, and four-way capability discovery. Selected definition Get and provider tool dispatch remain fresh. The published revision's local 30442 report still rendered $601 spend / 27.9K impressions; its eight report datasource HTTP requests were about 2.6–11.3 seconds each. This is a material improvement but remains above the local-overhead target. The 450-resource real-SDK fixture passed, with a 1.54 ms median warm list and 27.56 ms median cold list at four workers/page 20 (three repeats). Raw local traces: `/tmp/ENG-54517-opening-trace-core-v8.log` through `v11.log`; fixture: `/tmp/ENG-54517-catalog-fanout-matrix.log`. These are one-user local measurements, not p99 or production evidence. Remaining work is to eliminate repeated authenticated capability checks on the report path without weakening owner Get/dispatch authority, then remeasure the complete report.

### Shared public contract adoption — October 14

Core continues on `primitive-mcp-boundaries`; the temporary extraction branch/check-out was removed at the user's request. Core `7b5d8731` replaces the resource and primitive implementations (including generic catalog and observer context) with compatibility aliases/forwarders to published Forge `38bb1cf`. Core `90685b43` similarly reuses MCP `1d6bc8f` client configuration and async descriptors; async operation lifecycle remains in Core. These commits remove over 2,000 lines of duplicate implementation. The individual resource/primitive modules pass their tests; MCP configuration and async packages pass with `GOWORK=off`. The dependent Core runtime packages compile, and Agently builds in the explicit integration workspace. The nested-module publication/pins and a published-only Agently build remain pending.

Core `a800496f` adopts the shared freshness correction: persistent definition caching bypasses working and unspecified revision selectors. A regression test changes owning source bytes without a catalog notification and requires a fresh result. Immutable cache fixtures now select explicit stamps; working projection and terminal-check tests require owning reads. Original exact-byte, user/credential isolation, mutation/revocation and same-phase selected-resource checks remain. Earlier warm-Get figures obtained with persistent working reuse do not prove the corrected path's latency.

The integration graph uses published Forge `38bb1cf` and MCP `1d6bc8f` plus local Core resource/primitive modules. Resource/provider, MCP manager and datasource race suites pass. The combined reporting race run hit existing five-second lowerer deadlines; an isolated four-test rerun passed three, while `TestPinnedAuthoredReportCompilesThroughOfficialLowerer` still timed out. The full SDK race process was still active at this record. No timeout was increased. This is incomplete acceptance.

The retired `generic-cleanup-20261013` runtime was preserved in `/tmp/ENG-54517-steward-runtime`; obsolete local listeners using the retired path were stopped. The previously verified baseline binary is serving that preserved workspace at `127.0.0.1:30443` while the new shared source graph is validated. It is not evidence of the new integration binary's live acceptance.

#### Remaining extraction replacements

| Core implementation | Shared owner | Retained Core responsibility |
|---|---|---|
| `protocol/mcp/manager` generic pool/catalog/session files | Forge `backend/clientpool` | Explicit workspace config repository and Core identity/context composition |
| `service/resource` generic gateway, catalog, producer and snapshot files | Forge `backend/resourcehost`, `backend/provider` | Workspace/agent-specific readers, user persistence and host registration |
| `service/reporting` synchronous resource compile/execute, paging and parameter logic | Forge `backend/reporting/engine` | Export jobs, conversation/run persistence, scratchpad delivery and tool registration |
| `service/reportdefinition/materializer` bundled lowerer/runtime | Forge `backend/reporting/materializer` | Compatibility entry point only while callers migrate |
| Generic window/datasource/lookup/feed models | Forge contract packages | Application-specific storage, publication and identity bindings |

These replacements are still required. Compatibility aliases alone do not establish that Core uses the shared gateway or reporting engine. The newly published contracts must first be pinned in the root module with no local replaces, and actual report/window behavior must then be retested.

### Shared execution adoption follow-up — October 14

Core `b0cd64dd` adapts the shared MCP metadata contract at its Embedius host boundary. The final Agently shared-contract binary builds successfully. The corrected SDK startup-snapshot and working-read race regressions pass, as do MCP host adapter tests. The full reporting race suite passes when run without competing test suites (68.9 seconds); no execution timeout was increased.

Core `005a1042` switches report lowering and materialization directly to published Forge `backend/reporting/materializer` and removes the obsolete nested Core materializer module, bundle and duplicate fixtures. Before removal the trusted bundle hashes and every fixture matched the published Forge package; its Go implementation differed only in the resource import owner. Forge's complete materializer race suite passes (86.0 seconds), and Core's full reporting race suite passes against that shared implementation (66.8 seconds). These tests now live with their actual Forge implementation rather than a duplicate Core runtime. Core-owned compiler, datasource, revision and export integration checks remain in Core.

Window and datasource compatibility contracts are being changed to aliases of the same published Forge types. The window module's standalone race suite passes, and the datasource contract tests pass; the datasource runtime and dependent integration compilation are still being checked. The generic gateway, client pool and synchronous reporting engine remain follow-up work. No claim of complete extraction, published-only acceptance or new live UI acceptance is made.

Core `9013752e` completes the window and datasource compatibility aliases; window standalone race and datasource runtime race suites pass. Dependent resource, report and SDK packages compile. SDK native toolfeed and shared skill activation/supporting-file tests pass under race (`/tmp/ENG-54517-shared-sdk-acceptance.log`, 6.3 seconds).

The rebuilt integration binary `/tmp/ENG-54517-shared-contract-agently-v2` is serving the preserved workspace on port 30443 with primitive timing enabled. Authenticated UI verification restored the chat, opened the empty advertiser picker and displayed records, then returned to Performance Metrics without losing the completed report. All eight report views completed with the existing $601 spend / 27.9K impressions result. The older baseline's refresh-failure banner did not reappear after the new build completed. This is a local behavior check, not a release certification.

Cold report datasource HTTP requests still took approximately 2.2–9.35 seconds in this sample. A slow request spent 6.85 seconds resolving a datasource and 6.82 seconds inside the catalog snapshot; backend execution dominated other calls. Cold advertiser window resolution took 4.56 seconds while its subsequent datasource requests took about 165 ms. Warm report-tab return retained its completed result without restarting the eight requests. The active latency gate therefore still fails; shared type adoption has not fixed repeated external discovery. Timing source: `/tmp/ENG-54517-steward-shared-contract-v2.log`. Next consolidation must preserve owner reads while eliminating repeated namespace/catalog discovery and must be measured again.

Core public feature head `e84334ca` was pushed through IntelliJ and verified against the remote. Follow-up `f37abfaf` pins the three nested resource/primitive/window modules to that published revision. With these pins, the complete Core `GOWORK=off go build -mod=readonly ./...` passes, as do selected runtime package compilation and the resource cache race acceptance (4.3 seconds). No local replacements are required for these Core checks. Final dependency-pin publication and Agently's corresponding published Core pin/build are still pending.

The final Core feature head `f37abfaf` was also pushed via IntelliJ and independently verified. Agently now pins that published Core, the published nested modules, Forge38bb and MCP1d6; its `GOWORK=off go build -mod=readonly` succeeds. A dependency traversal confirms only Forge's materializer is linked, and the retired Core materializer requirement is removed from Agently. The local interactive verification above uses the same shared implementation; production deployment and the remaining latency/Studio parity gates are not certified.
