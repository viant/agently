## Tool Feed details layer
Tool execution details layer fits Forge’s existing metadata style and enables two flavors: data selection for visualization
and control actions (commit/rollback, etc.). No code, just the model.

Goals

- Add Forge-level visualization of tool results via selectors on the last call output.
- Expose companion control actions for certain services (commit/rollback, rerun).
- Keep everything declarative: metadata-driven activation with minimal new primitives.
- Reuse existing patterns: handlers, parameter mapping, dot-path selectors, DataSource signals.

Baseline (Forge Declarative Fit)

- Handlers: resolved via Context.lookupHandler(name) and compatible with actions.
- Parameter mapping: flexible from/to/name/location/direction, already used in dialogs/windows.
- Selectors: dot-path via resolveSelector(holder, path) — fast and simple.
- Signals: form, collection, input, control per DataSource; window control exists.

Execution Envelope

- Shape (runtime, not metadata): execution = { id, service, method, args, startedAt, durationMs, ok, error, output, meta, sessionId, correlationId }.
- History: session-scoped ring buffer (configurable size). Exposed via a “tools history” provider for UI.

Metadata YAML (Top-level)

- instrumentation.tools: declaratively describes what to extract and which controls to attach.
- Matching by service/method with optional wildcarding and ordering when multiple rules apply.

Example structure (shape, not code):

- instrumentation.tools:
    - - match: { service: "core", method: "updatePlan" }
    - `capture`: map output into DS or view
    - `visualize`: component/placement/data selector
- - match: { service: "system_patch", method: "*" }
    - `capture`: changed files, diff summaries
    - `controls`: commit, rollback handlers + guards
- - match: { service: "system_exec", method: "execute" }
    - `history`: enabled, limit, grouping
    - `visualize`: history list + detail log viewer
    - `controls`: rerun, open output

Flavor A — Visualization (Data Selection)

- capture: define how to map the last execution.output fields into a DS store or transient view model.
    - Uses the existing parameter mapping style:
    - `from: ":output"` (execution payload)
    - `to: ":form" | ":collection" | "<ds>:form" | "<ds>:metrics"`
    - `name: "<dest.selector>"`, `location: "<source.selector>"`
    - `direction`: usually implicit (uses `:output`)
- visualize: describe how UI should render the captured values.
    - component: canonical key or widget key (e.g., planView, fileDiffList, execHistory)
    - data: from/to + selector or simple selector: "output.plan"
    - placement: panel | sidebar | overlay | tab:Tooling
    - showWhen: expression on execution or captured state to toggle visibility

Core idea: you don’t push UI logic into the service; you declare what to show and where to read it from.

Flavor B — Controls (Additional Actions)

- controls: declarative buttons or menu actions attached to a matched execution.
    - id/label/icon: UI affordances.
    - handler: handler name (resolved via context) or service op.
    - parameters: optional parameter mapping to pass context values.
    - enabledWhen: expression against execution, captured state or DS data.
    - confirm: optional confirmation with template text.
    - sideEffects: optional follow-up captures (re-query changes after commit).

This enables service “capabilities” (e.g., system_patch) and tool-specific management (e.g., rerun last exec).

Selectors & Transforms

- selector: dot-path on a well-defined root:
    - For capture: root is execution with output nested: e.g., output.plan, output.diff.files.
    - For visualize.data: can be execution or a DS store.
- template: reuse resolveTemplate(${path}) when needed for strings.
- compute: optional light transforms (pick, map, count) can be supported later; keep v1 to raw selectors.

UI Integration

- Discovery: Window-level components (e.g., Chat, Editor, Tool Inspector) subscribe to:
    - Last execution that matches instrumentation rules for the current scope.
    - Or a configured toolsDs for execution history.
- Rendering: A small catalog of common components keyed by visualize.component:
    - planView: renders array of steps with status.
    - fileDiffList: changed files with status; can expand to unified diffs.
    - execHistory: list with status/exitCode/time; click-through detail to logViewer.
    - logViewer: stdout/stderr with search + copy.
- Controls: Render buttons defined in controls next to the visualization; wire to handlers.

Worked Examples

- core:updatePlan
    - capture: { from: ":output", to: ":form", name: "plan", location: "plan" }
    - visualize: { component: "planView", data: { from: ":form", selector: "plan" }, placement: "sidebar" }
- system_patch:* (no-arg companion methods, per your assumption)
    - capture: files: { from: ":output", to: ":form", name: "patch.changed", location: "changedFiles" }
    - controls:
    - commit: `{ handler: "system_patch.commit", confirm: "Commit ${:form.patch.changed.length} files?" }`
    - rollback: `{ handler: "system_patch.rollback", enabledWhen: ":form.patch.changed.length > 0" }`
- visualize: { component: "fileDiffList", data: { from: ":form", selector: "patch.changed" }, controls: ["commit","rollback"] }
- system_exec:execute
    - history: { enabled: true, limit: 100, groupBy: "sessionId" }
    - capture: { from: ":output", to: "tools:collection", name: "...", location: "..." } or implicit auto-log
    - visualize:
    - list: `{ component: "execHistory", data: { source: "history" }, placement: "panel" }`
    - detail: `{ component: "logViewer", data: { selector: "output.stdout" } }`
- controls:
    - rerun: `{ handler: "system_exec.execute", parameters: [{ from: "caller:form", to: ":input", name: "args" }]}`
    - openOutput: `{ handler: "files.open", parameters: [{ from: ":output", location: "artifacts[0].uri", to: "caller:form", name: "uri" }] }`

Lifecycle & Storage

- Scope: Per window/session by default; optional global.
- History: In-memory ring buffer with opt-in persistence (if backend available).
- Redaction: Per-rule masking of fields (stdout, secrets). Support redact: ["output.env", "args[1]"].
- Sampling: sample: rate|predicate to limit volume for noisy execs.
- Error handling: Visualization still shows envelope with ok=false, error.message.

Timeouts

- Tool execution enforces a bounded timeout to prevent a single stuck call from blocking the run.
- Configure via env var `AGENTLY_TOOLCALL_TIMEOUT` (e.g., `45s`, `2m`). Default is `3m`.
- On timeout the tool call is marked `canceled` and the error text is captured as the response payload so the model can reason about it.

Activation & Precedence

- Activation points: Window metadata under instrumentation.tools; optional global defaults merged with window’s rules.
- Matching order: First match wins; priority optional for ordering.
- Namespacing: service and method are explicit; wildcards allowed (method: "*").

Security & Safety

- Opt-in for controls that mutate state (commit/rollback); confirm prompts recommended.
- Role-based visibility: visibleWhen with a user/role source if available.
- Sandbox-aware: controls can be disabledWhen sandbox blocks an operation.

Migration & Implementation Steps

- Add an execution bus: capture envelopes for all tool calls.
