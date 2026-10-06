# Forecast evidence binding (proposed writer integration)

The `ac0973a1-9c3e-4c96-bd5b-cfeddac8b610` audit found correct persisted and provider-visible pairs for all 20 cube calls and a correct nine-response continuation chain. The provider's final `forge-data` itself swapped October 2/3 overall availability. Thirteen other timeline cells and the aggregate were correct. Renderer relabeling or another prompt cannot enforce the association.

## Minimal profile

A forecast daily `forge-data` envelope carries `sourceBindings` alongside its dataset ID:

```json
{
  "planId": "<saved immutable plan id>",
  "profile": "forecast-daily-v1",
  "columns": [
    {"key": "overall", "calls": [
      {"opId": "call_example", "requestHash": "<64 lowercase hex>", "responsePointer": "/data/0/avails"}
    ]}
  ]
}
```

Version 1 binds only single-row aggregate availability (`avails`) by day. Each declared column must cover every trusted requested day exactly once. Column names are presentation aliases matched to an independently supplied trusted policy, never inferred from numeric values or call order. Dates are generated from the bound request's `filters.date`, not supplied by the model. Arbitrary JSON pointers, expressions, aggregation, uniqueness summation, dimensioned responses, foreign calls, or new queries are unsupported.

The server supplies an immutable `Scope` (owner, conversation, turn), a trusted `Policy` (requested dates and exact per-column request templates without `filters.date`), and completed `Call` records from existing authorized native/Datly conversation APIs. Policy comes from the scoped targeting-conversion/forecast plan and authoritative category mapping; it is not accepted from the model's binding object. All non-date request fields—including exclusions, targeting selectors, measures, dimensions, and pagination—must match the policy exactly. The implementation's hash is SHA-256 over canonical object-key-sorted JSON (`encoding/json`, decoded with `UseNumber`); number lexemes are preserved, so `1` and `1.0` have distinct identities. The model receives hashes from the server; it does not invent them.

The pure materializer resolves each call ID, verifies scope/status/tool/hash and exact expected request, then emits ordinary date-keyed JSON rows and cell provenance. It performs no storage, network access, tool dispatch, or persistence. A separate validation entry point compares an already supplied data array with the deterministic result and rejects disagreement. Thus the Oct2/3 swapped array is rejected; materialization produces the correct values directly without altering a saved artifact.

## Date, duplicate, and failure behavior

`filters.date` must be a supported UTC midnight timestamp for one of the policy's explicit calendar dates. Nullable or absent response `eventDate` remains nullable in the original response: the derived row date comes from the request. A non-null response date must agree; otherwise fail. Each response must contain exactly one object row and a finite nonnegative numeric `avails`. A referenced call must be completed and successful. Missing/duplicate call IDs, duplicate column/date bindings (even identical values), conflicting records, invalid hashes/pointers, extra or missing columns/days, and unsupported shapes reject materialization. Additional unreferenced completed calls may exist; the actual fixture has 20 calls but the timeline binds 15 cells. Failed evidence never becomes an empty successful dataset.

## Enforcement and writer paths (not wired by the pure package)

A shared server-side content gate must resolve and validate bindings before a new forecast data block can become a committed report. The gate applies to:

1. Model final responses: `service/agent/run_turn.go: persistFinalAssistantMessage`, with the same gate applied before successful canonical report publication from the streamed response. Tentative numeric data must not be presented as verified before this gate. Patching only the final message after exposing unverified committed data is insufficient.
2. Explicit assistant writes: `protocol/tool/service/message/add.go: Service.add`, before `AddMessage` and final SSE publication. Interim content containing a forecast report commit follows the same rule; `interim` cannot bypass enforcement.
3. Report compilation/commit: `service/reporting/compiler_fenced.go` and the report store/complete validation boundary in `service/reporting/service.go`. The host resolves records under authenticated current scope, supplies the trusted policy, materializes/validates before compilation and hashing, and persists the verified ordinary canonical rows/provenance. The pure Forge compiler must not independently query storage.

The model-delta mapping is `service/core/stream.go` (typed `StreamEventTextDelta` is appended to output events); the review must also locate its event bus/publication subscriber. The content gate must consume `sourceBindings` before client parsing: current `sdk/report_inline.go:inlineDataEnvelopeFields` deliberately rejects unknown envelope fields. Emit only the existing canonical envelope plus materialized data, and keep evidence provenance in the approved report metadata/audit path; do not silently relax every SDK parser. Preserve the raw provider response for audit separately from new validated canonical content. The next integration patch must identify the common stream/canonical publication hook in addition to these persistence entry points. No shared writer is changed in this initial pure implementation. This document is a concrete contract, not a claim that enforcement has shipped.

## New writes versus historical compatibility

New forecast evidence-backed daily datasets require this profile at the server write boundary when the server's trusted forecast plan establishes that source class. Missing bindings fail with an actionable diagnostic; a model cannot bypass by omitting the profile or choosing another dataset ID. Generic manually authored tables and unrelated report fences retain their existing semantics. This discrimination must use trusted run/tool-plan context, not arbitrary dataset-name matching.

Historical saved reports remain readable as historical unverified artifacts. Do not rewrite their bodies, hashes, or rows, relabel dates on clients, or issue fresh business reads to repair them. Existing known mismatches should remain explicitly identified by audit evidence. New validated data can be published only through the authorized writer lifecycle; no implicit repair or retroactive verification is claimed.

## Required tests

Use the captured 20 paired calls to build all 15 timeline cells. Reordering records/bindings must not change output. The October 2/3 swap must fail validation. Wrong owner, conversation, turn, request hash, tool, request scope, response pointer/date, duplicated/conflicting evidence, failed calls, and missing cells must fail closed. Tests must also prove the raw call responses and proposed historical body are unchanged. Writer integration will additionally require model-final, message/add, stream-commit, and report-commit bypass tests before any live release.

## Concrete trusted-plan lifecycle and source receipts

### What exists, and what does not

For the audited turn, the accepted `steward/AdTargetingProfile` request was `AudienceId:[7366798]`. Its completed response identifies `data[0].audienceId=7366798`, parent order `2685245`, and the audience's original targeting/exclusion expressions. `data[0].audience.inclusionSignals` supplies the authoritative groups:

- Inventory: `ad.site.type` (`app`, `web`).
- Location: `location` (`US`) and `location.metrocode` (`807`).
- Data: `viant.taxonomy` (`689452`).
- Demographics: `user.demographic.age` (`5` through `9`).
- Shared exclusion: `site.lists.v2` (`11548`), retained for every series.

The actual response has no `forecastHandoff`. That skill-documented compact surface must not be assumed to exist. The completed `ForecastingTargetingConvert` response supplies `includedFeatures`, `excludedFeatures`, `body`, `query`, and `queryString`. It confirms the normalized full filter set. Its request's inclusion/exclusion must match the selected profile's original expressions; caller-supplied conversion strings alone are not independent scope authority.

Authoritative implementation is in `viant-internal/steward/pkg/steward/inventory/targeting/forecasting/convert.go` (`Service.Convert`) and `service.go` (`BuildForecastingRequest`, `BuildForecastingInputFromAdProfile`), backed by `converter/Converter.ToForecastingInput`, strict expression parsing, and its embedded feature mapping. `converter.WithIgnoredIncludeFeatures` can construct a group-only inclusion while keeping every exclusion. **Do not reuse `BuildForecastingCategoryRuns` unchanged**: its current loop uses only generic/data/context/supply categories and also ignores out-of-category exclusions. That is different from this workflow's profile-produced Inventory/Location/Data/Demographics matrix and shared-exclusion contract.

The current core/assembly module does not import this private Steward converter. Therefore a real policy producer must be added before enabling enforcement. Preferred boundary: a pure forecast-plan projection at the Steward conversion/profile producer, using that same converter implementation, returns an exact supported one-day request matrix and its source-profile identity. The BFF independently binds the projection to its authorized profile and conversion call records, verifies original expressions/entity IDs, and wraps it with owner/conversation/turn and hashes. This is an extension of existing producer output, not a new cube, query, database table, or model-authored template. An injected host `ForecastPolicyBuilder` backed by the same converter is the alternative if the assembly accepts that package dependency; do not copy the mapping logic into core.

### Admission and dates

1. At the accepted profile completion, bind the exact selected entity IDs from its request to the IDs in the returned profile. Pin owner, conversation, turn, profile op ID, request hash, and response hash. A profile for another entity cannot satisfy this plan.
2. Establish an immutable clock and zone from the admitted turn, not the later rendering clock. The existing forecast skill's default is current day plus the preceding two days (`steward-proof/workspace/skills/forecast/SKILL.md`, “Default horizon”). The actual turn's Oct4 clock gives Oct2–4. Structured user-origin dates are enforced exactly. For plaintext custom windows, freeze the exact executed converter window with `dateOrigin:"tool-evidence"`; this proves actual source-request dates, not verified user intent. Never silently substitute the default for a custom converter window. The historical artifact does not itself prove a structured date receipt exists.
3. After the selected converter completes, require its input expressions to match the selected profile. For `user-window` or `workspace-default`, require its dates to match the admitted window; for `tool-evidence`, freeze the exact converter request/response window before accepting cube receipts. Produce the full/category exact request templates through the authoritative converter and profile groups. Retain typed shared exclusions and shared non-category constraints; reject unsupported/group-ambiguous selectors. Fix the aggregate measure/dimension/pagination profile in server policy; do not learn templates from the first cube call or from the fixture.
4. Canonically hash a plan record containing scope, immutable dates/clock/zone, producer version, profile/conversion source hashes, category membership, and exact request templates. The resulting `planId`/`policyHash` is a stable identity. New profile, dates, exclusions, or mapping version creates a new plan, never mutates the admitted one.
5. Persist/resolve that record through existing native authorized tool-result/turn metadata APIs. A restart reconstructs it from the same immutable profile/conversion receipts and recorded clock; if required evidence is absent, fail closed. An in-memory cache may accelerate resolution but cannot be the sole authority. No new schema is required.

### Receipts visible to the model

The model currently receives no request hashes. A new server-owned result projection must supply them; expecting the model to compute them is not an implementation.

For a completed conversion/profile plan, advertise a reserved `_agentlyForecastPlan` object containing `profile`, `planId`, `policyHash`, `dates`, `columns`, and the expected per-column/per-date request hashes (plus exact supported requests if needed for execution). On each successful matching `ForecastingCube` result, preserve its existing `status` and `data`, and add `_agentlyEvidence`:

```json
{
  "profile": "forecast-daily-v1",
  "planId": "<server plan identity>",
  "policyHash": "<server policy hash>",
  "column": "overall",
  "date": "2026-10-02",
  "opId": "call_example",
  "requestHash": "<server computed request hash>",
  "responseHash": "<hash of original response excluding receipt>",
  "responsePointer": "/data/0/avails"
}
```

The model may refer to this receipt in `sourceBindings`; the writer resolves the original stored call and plan again. Receipt text is not authority. Reserve these result keys and reject a tool-supplied spoofed collision. Advertise a completed receipt only after successful durable completion; persistence failures cannot yield a usable receipt. Failed, asynchronous-pending, overflow-only, or scope-mismatching results do not satisfy evidence bindings. Tool overflow handling must retain a compact receipt while preserving the original result through its existing payload link.

Concrete core boundary: `service/shared/toolexec/tool_executor.go` currently executes the tool, optionally wraps overflow around lines 448–453, normalizes result, writes the response payload/body around 588–596, and finalizes the tool call around 603–614. Introduce an injected evidence-result projector on the successful raw result **before overflow projection**, keep request/response bytes for hash derivation, and ensure the returned model-visible receipt is released only after terminal persistence succeeds. The existing `completeToolCall` / native payload links provide the immutable record reference. The same projector must cover coalesced results (`tool_executor_messages.go:persistCoalescedToolResult`) and resumed results, each with its own op ID; a duplicate physical response is not permission to substitute another op's receipt.

## Actual prepublication hook and new unbound behavior

`service/core/stream.go:consumeEvents` calls the provider handler before appending its own canonical event. The immediate text publisher is actually `service/core/modelcall/recorder_observer_helpers.go:publishStreamDeltaNow`, reached via the coalesced `publishStreamDelta` buffer; it calls `StreamPublisherFromContext(ctx).Publish`. Gating only `appendStreamEvent` would be too late. Wrap the injected `modelcall.StreamPublisher` (`service/core/stream.go` and `generate.go` inject it into context) with a per-message, per-admitted-plan content gate. That wrapper must see split fence delimiters and UTF-8 boundaries across arbitrary chunks.

The wrapper passes ordinary prose, reasoning, and tool progress through. Once a forecast report group begins, it buffers numeric `forge-data` and the associated report commit. It can expose a non-committed layout shell, but cannot release data as verified before bindings and the plan are available. At commit it resolves the complete evidence matrix, materializes/validates, removes writer-only `sourceBindings`, and releases standard canonical data plus commit. Output offsets must be based on released canonical text, while raw provider bytes remain in the model-call audit recorder. Cancellation or invalid/incomplete binding discards the uncommitted numeric projection and emits an actionable diagnostic; it does not silently fill missing cells or expose the wrong data first.

The same content gate is invoked for `message/add`, final assistant persistence, compile-only, and saved report completion, so non-streaming and explicit writes cannot bypass it. Idempotency is keyed by admitted plan plus report scope/id and source-binding digest. Authorizing one response does not authorize another modified binding.

For a **new** forecast run after rollout, an accepted scoped profile/conversion plan marks the run as requiring evidence binding. Missing `sourceBindings` (the current legacy model output style), altered dataset IDs, or omission of `_agentlyForecastPlan` does not disable the gate: the data block fails completion with a concrete missing-binding diagnostic. The model can repair within the same run using advertised receipts, without new cube reads. Unsupported plan modes return an unsupported-plan diagnostic rather than falling back to unchecked forecast tables. Generic unrelated fences remain unaffected. Historical saved bodies are read-only and never retroactively rewritten or claimed verified.

Before writer changes, integration tests must prove: actual profile/category conversion produces the 15-request policy independently of cube requests; dates are pinned across midnight/restart; receipts survive overflow/coalescing/resume with correct op IDs; every chunk split of a report/data/commit sequence withholds wrong rows; explicit message/add/final/compile/complete writes all reject missing or conflicting bindings; generic and historical reads remain unchanged. This lifecycle is proposed, not implemented by the pure binder tests.

## Optional user-origin selection admission and compatibility

Source inspection confirms the current gap rather than assuming a picker proof:

- Web `ui/src/services/chatService.js` builds submission `context` from `buildWebQueryContext`, workspace hints, and extra context. `clientContext.js` provides platform/capabilities/client ID. `workspaceQueryContext.js` explicitly says its view hints are not authorization or resource grants. No immutable starter selection receipt is currently emitted.
- iOS `AppRuntime.sendCurrentQuery` calls `ComposerRuntime.resolvedQuery()` and passes `buildAppleClientQueryContext`; that helper contains platform/form factor/capabilities/UI client ID only. The selected token becomes prompt text.
- Android `QueryRuntime` passes its supplied `queryContext` to `QueryInput`; the future composer producer must capture selection before draft reset, not reconstruct it from the resolved prompt.
- Core `service/agent/query.go:QueryInput.Context` already supports an additive object. `intake_query.go` explicitly describes normalized `Scope.Values` as hints, not authoritative data. Do not promote those hints or parse free prose into a verified selection.

Proposed optional user request field:

```json
{"client":{"forecastIntent":{
  "version":1,
  "selectedEntities":[{"kind":"audience","id":"7366798"}],
  "window":{"mode":"workspaceDefault"}
}}}
```

An explicit window uses `mode:"explicit"`, `from`, `to`, and a supported IANA `timeZone`. Version 1 supports a single selected audience/line. IDs are canonical positive decimal strings; reject duplicate/conflicting IDs, unknown kinds/fields, invalid windows, and unsupported versions. Do not accept client `owner`, `turnId`, `policyHash`, `authoritative`, or `selectionOrigin` fields. Web, iOS, and Android capture this object directly from the accepted lookup token values and date controls at Send, alongside the resolved text. They do not reconstruct it from display labels, current workspace selection after Send, or a later async callback. Existing SDK context maps remain backward compatible; CLI callers may supply the same optional field without any UI dependency.

Core freezes the validated value before intake in a separate server-owned `forecastbinding.Admission`, supplies owner from authenticated principal and conversation/turn from the actual accepted turn, and assigns `selectionOrigin:"user-selection"`. Later model context, tools, sidecar hints, profile requests, and retries cannot replace it. A profile request or returned entity that differs from this explicit selection fails; it must not downgrade to fallback. The admission is a scope constraint, not an authorization grant: existing resource authorization still applies. Persist it through the existing native run checkpoint/payload path under a versioned reserved entry, preserving other checkpoint data, before intake can proceed; resume must load the same admission rather than current client context. The exact checkpoint adapter must be tested alongside existing resume writers before installation.

When this optional field is **absent**, retain plaintext and CLI forecasting behavior with `selectionOrigin:"tool-evidence"`: source identity is the exact authorized completed profile request plus matching returned profile. This verifies the data's source entity, not that a model interpreted the user's free-text entity correctly. Do not claim user-selection proof or parse the prompt heuristically. The same strict converter, exclusions, scope checks, call receipts, and deterministic binding still apply. Bindings cannot substitute an arbitrary entity/body because the host loads and revalidates the original scoped profile and conversion records. Multiple profiles require distinct plan identities; a report cannot silently merge plans.

Date compatibility is approved as follows: structured user-origin dates use `dateOrigin:"user-window"` and enforce exact match with no fallback. An actual workspace-default workflow may use `dateOrigin:"workspace-default"` and pin its three-day window from the admitted clock/zone. Plaintext/custom-window callers use `dateOrigin:"tool-evidence"`: freeze and display the exact executed converter window, without claiming the user's natural-language date intent was verified. Never replace that custom window with the default. Preserve existing forecast reads; no prose parser or permission prompt is added. Cube receipt dates must belong to the frozen plan regardless of origin, and each cell still derives its date from the exact paired request. These provenance labels are assigned by the server, not accepted from model bindings or client owner/proof fields.

The core SPI is now declared in `service/reporting/forecastbinding/producer.go`: `Admission` (including `SelectionOrigin` and `DateOrigin`), `PlanSources`, `ProducedPolicy`, `PolicyProducer`, `SourceStore`, and startup-injected `Dependencies`. It imports no private Steward code and installs no global mutable registry. The composition root must provide the authoritative producer/projection adapter and native authorized store adapter before enabling gates; missing dependencies are explicit unsupported-policy errors, not permission to accept unchecked data.

The authoritative private-converter regression is `viant-internal/steward/pkg/steward/inventory/targeting/forecasting/agui_evidence_policy_test.go`, with the captured audience-profile fixture. It independently builds overall plus the four actual profile groups, checks audience 7366798, rejects 7366799, retains `exclude_sitelist2:[11548]` on all five templates, checks dates, and proves the profile is unchanged. It calls the existing strict converter with include-only ignores; it does not use cube requests as template input. This test-only adapter is not a claim that the runtime producer or admission writer is installed.

## Immutable evidence storage (approved replacement for checkpoints)

The forecast admission and plans are JSON documents attached to the existing
native execution run in `call_payload`: `kind=attachment`, typed
`subtype=agently.forecast.admission.v1` or `agently.forecast.plan.v1`, and matching
versioned `schema_ref`. `run_id` owns the document; `sequence` stays NULL (it is
not an AG-UI journal event). No new table, column, enum, transcript message, or
checkpoint namespace is required. The admission ID is a length-framed SHA-256
identity of owner/conversation/turn/run/type; a plan additionally includes its
validated immutable plan ID. Exact document digest and canonical JSON must match
on idempotent duplicate writes; differing bytes fail.

The private Datly orchestration locks the owned execution run in a serializable
transaction before writing, checks exact conversation/turn/owner/run identity,
running status, and the supplied unexpired native execution lease, then invokes
a restricted transcribed insert-only payload writer. Read authorization joins
that same execution run and checks the exact document subtype and schema. Normal
checkpoint updates can run concurrently without touching these documents.
Existing run-owned payload retention protects them through continuation. The
native SQLite regression now verifies guarded GC retains live run documents and
native run deletion immediately cascades them. This requires foreign keys on
every opened connection, supplied by the provisioned DSN (a one-time schema
connection PRAGMA was insufficient).
The earlier checkpoint namespace prototype must
be removed before runtime installation; no dual storage or fallback is kept.


Current implementation checkpoint: private DQL readers/writer and orchestration
compile and pass native scope, immutable duplicate/conflict, restart, active-lease,
concurrent ordinary checkpoint/document-write, and guarded-GC tests. The pure
20-call fixture still proves all 15 cells and rejects the swapped dates. Combining
that fixture with real persisted evidence across restart,
startup injection, and shared streaming/writer publication gates remains unfinished.
No live feature gate or provider query is enabled by this storage change.


## Production source checkpoint (not startup-enabled)

The optional `runtime/evidence.Factory` boundary now captures the original
`context.client.forecastIntent` before routing/intake, saves and confirms its
immutable admission after the durable starter, and restores the original
admission on continuation. A new clock or model-generated nested context cannot
replace it. Startup registration is now available through the explicit workspace
opt-in below; existing workspaces remain disabled by default.
Historical reads do not pass through this write gate.

The turn controller resolves a unique matching completed profile, or an explicit
`sourceProfileOpId`, under the authenticated owner/conversation/turn. The effective
converter request is enriched **before** coalescing, request payload capture and
dispatch. It records a server-generated `evidenceSourceOpId`; model-supplied trusted
body/reference fields are rejected. Request and response payloads remain immutable.
A completed conversion saves/reloads the immutable plan and publishes its receipt.
A general cube result receives an `_agentlySource` receipt describing the actual
operation, hashes and request date. It does not claim policy/category/user intent;
those are independently checked when the report names `sourceBindings.planId`.
Grouped/range cube reads remain supported and do not gain a scalar pointer unless
the response satisfies the supported aggregate shape.

For version 1, after a trusted forecast plan is admitted, **every newly authored
structured dataset in that turn requires trusted bindings**. Renaming a dataset,
labeling it unrelated, or supplying inline rows, numeric KPI values or chart arrays
cannot exempt it. Mixed-source tables need an independently trusted source
classification before they can be supported; model labels are insufficient.
Canonical `forge-report` layout/prose remain supported. `forge-config` is an
unsupported alternate runtime/data path within this admitted proof boundary.
Unrelated turns retain existing report behavior. Dataset mutation modes are
restricted to complete `replace` transactions, avoiding append/patch duplication.

The common guard is connected to immediate model-call delta publication, model
assistant projection, agent final content/persistence, `message/add` (including
interim messages), and fenced compiler content plus explicit/string-encoded
fences. Structured fences are buffered across byte/UTF-8 boundaries and validated
before release. Raw provider audit payloads remain unchanged. Rejections propagate
as evidence errors rather than successful empty data. Native restart tests cover
all twenty captured calls, fifteen correct cells, immutable plan/admission,
printable receipts and unrelated checkpoint coexistence.

The standalone HTTP boundary now has an optional server-issued command receipt.
The UI dispatcher persists an immutable attachment bound to the exact running
native operation, owner/conversation, canonical persisted workspace creator and
activation, builder, revision, and admitted plan IDs. It issues a stable request
UUID plus opaque `reportAdmissionRef`. Begin resolves the receipt and stores only
server-verified `_agentlyForecastCommand:{version:1,ref,requestId}` linkage in
requestedParams; caller namespace injection and request/builder substitution fail.
The linked compiler consumes the same reference, validates bindings, and stores
an immutable spec/fill/print hash proof. Complete rereads that proof and exact
scope before its lifecycle CAS. Hashing normalizes decimal spelling without
float64 rounding, so large integer changes remain detectable. Existing unlinked
manual reports remain available and are not guessed to be forecast reports from
an active conversation alone.

The native integration test covers this receipt/compiler/verification path and
its restart, including wrong workspace revision, request/builder/conversation
substitution, missing bindings, artifact tampering and idempotent compilation.
Startup remains disabled for the current Steward workspace pending verified
authoritative producer deployment, explicit picker-origin context binding, and
live acceptance. The composition-root registration is implemented as an opt-in.
Do not claim live forecast parity or mixed-source report support from these unit
and native-persistence proofs.


## Explicit workspace rollout configuration

The assembly's `Serve` and headless scheduler startup read this optional workspace
configuration before accepting requests:

```yaml
features:
  forecastEvidence:
    enabled: true
    profile: forecast-daily-v1
    timeZone: America/Los_Angeles
    rolloutCutoff: '2026-10-05T18:00:00-07:00'
```

This is an example, not an enabled deployment. Missing configuration or
`enabled: false` captures no new admissions and performs no registry preflight,
while restoring existing recorded authority. Enabled configuration requires
an exact supported profile, a valid IANA zone and a fixed RFC3339 cutoff;
unknown fields and invalid values fail startup. An enabled cutoff must be at or
before the activation clock; the clock validates the value without deriving or
changing it. The cutoff comes from deployment
configuration and is never derived from boot time, prompt text or a missing
admission document.

Enabled startup first waits, bounded and cancelable, for the existing runtime-owned
registry warmup completion. It does not start a duplicate initializer or replace
the registry refresh lifetime with a temporary wait context. Startup then checks
the converter's declared
`Request.evidenceProfile` object, `Request.evidenceAudienceId` integer,
`Request.evidenceSourceOpId` string and `forecastEvidence` object output. Local
schema references are supported; absent, cyclic or external references do not
establish capability. The authenticated deployed catalog currently lacks all
three inputs and declares only a generic object output, so the current workspace
must remain disabled. No converter or cube business call is needed for that
capability check.

A single native source store and immutable-document store are shared by the
agent factory and report-command backend. The backend is attached to both report
compilation and run admission/completion before serving. Ordinary unlinked
reports retain their existing behavior. New turns in the enabled workspace
capture an admission; their publication guard remains dormant until an actual
trusted forecast plan is admitted.

On resume, a present admission is restored and validated strictly. Corrupt
admissions and storage errors never become legacy eligibility. A typed,
scoped missing-document result is eligible for passthrough only after checking
native run/conversation ownership and proving the server-owned run creation time
is strictly before the configured cutoff. Equality, later times and unknown
creation times fail closed. Resume never recreates intent or changes the clock;
the same enabled cutoff remains required after restart. Disabling or removing
the feature installs restore-only enforcement: present recorded admissions and
command proofs remain strict under their stored clock/policy, missing ordinary
admissions resume only after native ownership verification, and new queries do
not capture evidence. This path does not wait for or consult current producer
capability. Saved historical report bodies,
rows and hashes are never rewritten by this registration.

Current web, iOS and Android starter submissions do not emit structured
`client.forecastIntent`. Their lookup selections currently become resolved
prompt tokens. The factory can therefore support `tool-evidence` provenance but
cannot claim that those selected entities were captured as explicit user-origin
intent. A future declarative starter binding must name the accepted lookup token
and supported window mode, capture those values at Send before clearing the draft,
and pass them through the existing context map. Prompt parsing, lookup-name
heuristics and later producer arguments must not establish user-selection proof.

The saved `ac0973a1-9c3e-4c96-bd5b-cfeddac8b610` artifact remains erroneous:
October 2/3 overall values were swapped in provider-authored `forge-data` before
persistence. The recorded twenty paired request/results and nine continuation
links are correct. Formatter fixes do not repair those stored cells. Source
materialization, publication and native restart regressions reject that exact
swap; a new enabled, verified live workflow is still required for acceptance.
