# Multi-Platform Metadata Plan

Last updated: 2026-04-13

## Goal

Separate Forge metadata by platform and form factor so mobile work cannot break
web behavior, while keeping a minimal shared layer for truly common definitions.

Target outcome:

- `web` metadata remains first-class and independently testable
- mobile metadata is split into explicit `phone` and `tablet` branches
- target selection happens server-side for metadata/window loading
- `$import(...)` resolves relative to the active target branch before falling
  back to shared content
- one shared target-context primitive is reused across:
  - metadata/window fetches
  - query/client context
  - runtime metadata resolution

## Current Findings

### What already exists

- Web Forge has target-aware metadata resolution in the frontend runtime:
  - `platform`
  - `formFactor`
  - `targetOverrides`
- Android already passes explicit Forge target context:
  - `platform = "android"`
  - `formFactor = "phone" | "tablet"`
- iOS Forge runtime supports target context and metadata resolution, but the app
  bootstrap still needs explicit `ios.phone` / `ios.tablet` wiring
- Mobile OAuth is already handled natively outside Forge metadata:
  - iOS uses `ASWebAuthenticationSession`
  - Android uses a dedicated WebView auth surface fed by `authWebUrl`

### What is still being refined

- Most key windows now have explicit `web`, `android/*`, and `ios/*` branch
  scaffolding, but many mobile branches are still copied from the current web
  or legacy layouts and have not been specialized yet
- Final end-to-end device smoke across real browser / simulator / emulator flows
  should continue as branches diverge further
- The target `capabilities` vocabulary should be documented and tightened over
  time so it stays a Forge-owned primitive instead of drifting per client

### Regressions already observed

- The `sdk improvements` cleanup removed metadata windows still used by web nav
- Web kept opening window keys like:
  - `chat/new`
  - `schedule`
  - `schedule/history`
- Missing metadata caused real runtime regressions

## Proposed Metadata Structure

Use explicit branch folders:

```text
metadata/window/<window-key>/
  shared/
  web/
  android/
    phone/
    tablet/
  ios/
    phone/
    tablet/
```

Resolution order:

1. exact platform + form factor
2. platform
3. shared
4. legacy root fallback during migration only

## Shared Target Primitive

Define one sharable target-context primitive in Forge for generic metadata/UI
targeting and reuse that same shape in Agently where matching client identity is
helpful.

Suggested shape:

```json
{
  "platform": "web|android|ios",
  "formFactor": "desktop|tablet|phone",
  "surface": "browser|app",
  "capabilities": ["markdown", "chart", "upload", "code", "diff"]
}
```

Rules:

1. Forge owns the canonical structure and matching semantics for metadata-driven
   UI targeting.
2. SDKs should construct and send the same target object for:
   - metadata/window fetches
   - Agently query context (`context.client`) when the app wants the same client
     identity contract
3. Backend metadata selection should consume that same structure.
4. Frontend/runtime fallback resolution should also consume that same structure.

This avoids drift between:
- query context saying one thing
- metadata calls saying another
- client-side metadata resolution inventing a third shape

Current vocabulary in use:

- `markdown`
- `chart`
- `upload`
- `code`
- `diff`
- `attachments`
- `camera`
- `voice`

Current platform usage:

- web currently advertises `markdown`, `chart`, `upload`, `code`, `diff`
- iOS currently advertises `markdown`, `chart`, `attachments`, `camera`, `voice`
- Android now reuses one shared helper so Forge runtime and metadata requests
  both advertise `markdown`, `chart`, `attachments`, `camera`, `voice`

OAuth note:

- `oauth` metadata still exists for web/admin-style metadata-driven views
- mobile sign-in itself should continue to delegate to the provider login flow
  through the native auth surfaces rather than depending on the Forge `oauth`
  window as the primary user-facing auth path

## Implementation Plan

### Phase 1: Forge Loader Support

Status: `completed`

Tasks:

1. Define the shared target-context primitive in Forge.
2. Add target context to backend metadata/window requests.
2. Update Forge backend `LoadWindow(...)` to resolve branch roots instead of
   hardcoding `key/main.yaml`.
3. Update Forge backend `$import(...)` resolution to search:
   - current target branch
   - platform branch
   - shared branch
   - legacy fallback
4. Add backend tests covering:
   - `web/desktop`
   - `android/phone`
   - `android/tablet`
   - `ios/phone`
   - `ios/tablet`
5. Add tests proving the same target primitive is accepted by:
   - metadata/window loading
   - frontend runtime resolution

### Phase 2: Web-First Migration

Status: `completed`

Tasks:

1. Wire local module development for `agently`:
   - `github.com/viant/agently-core => ../agently-core`
   - `github.com/viant/forge => ../forge`
2. Send the shared Forge target-context object on web metadata calls and query
   context.
3. Restore and stabilize current web metadata windows so web is not broken while
   migration begins.
4. Move web-owned metadata into explicit `web/` branches first:
   - `chat/new`
   - `chat/conversations`
   - `schedule`
   - `schedule/history`
   - `agent`
   - `model`
   - `oauth`
   - `mcp`
   - `preferences`
   - `tool`
   - `workflow`
5. Add tests to ensure web-opened window keys resolve.

### Phase 3: iOS Migration

Status: `completed`

Tasks:

1. Pass explicit `ForgeTargetContext(platform: "ios", formFactor: ...)` from the
   iOS app layer.
2. Reuse the shared Forge target-context primitive for:
   - metadata requests
   - query context
3. Split iOS metadata into:
   - `ios/phone`
   - `ios/tablet`
4. Verify iOS runtime loads the expected branch.
5. Verify on iPhone and iPad simulators as branch-specific UI diverges.

### Phase 4: Android Migration

Status: `completed`

Tasks:

1. Keep Android target context as the source of truth.
2. Reuse the shared Forge target-context primitive for:
   - metadata requests
   - query context
3. Split Android metadata into:
   - `android/phone`
   - `android/tablet`
4. Verify Android runtime loads the expected branch.
5. Verify on phone and tablet emulator targets as branch-specific UI diverges.

### Phase 5: Final Verification

Status: `in_progress`

Tasks:

1. Web verification:
   - all nav-opened windows resolve
   - chat, scheduler, and runs work
2. iOS verification:
   - phone and tablet both load intended branches
3. Android verification:
   - phone and tablet both load intended branches
4. Add permanent regression coverage:
   - caller-to-window resolution tests
   - target-matrix metadata loader tests
5. Continue replacing copied mobile scaffolds with true platform/form-factor
   specific metadata where behavior or layout differs

## Progress Log

### Completed

- Restored deleted web metadata windows under `metadata/window/...`
- Fixed the restored `chat/conversations` metadata import mismatch
- Confirmed scheduler JS handlers still exist and match restored metadata
- Verified that the missing architectural piece is Forge backend loader support,
  not app-side runtime targeting support
- Added a shared target-context section and made Forge the canonical owner of
  the shape
- Added local web target-context reuse:
  - `buildWebTargetContext()`
  - query context reuses the same fields
- Wired Forge web metadata fetches to append:
  - `platform`
  - `formFactor`
  - `surface`
  - `capabilities`
- Wired `agently-core` SDK workspace metadata calls to accept the shared target
  context shape in:
  - Go HTTP SDK
  - TypeScript
  - Android
  - iOS
- Implemented Forge backend target-aware window selection candidate order:
  - exact platform + form factor
  - platform
  - shared
  - legacy fallback
- Implemented Forge backend target-aware `$import(...)` fallback order for
  branch-aware metadata loading
- Implemented Forge backend target-aware navigation loading so menu metadata
  follows the same target-context contract as window metadata
- Added regression tests for:
  - embedded window metadata loading
  - discovery-based loading of every explicit embedded `web`, `ios/*`, and
    `android/*` branch `main.yaml`
  - exact branch selection for every discovered explicit embedded target branch
  - embedded HTTP UI handler responses for target-aware navigation and window
    metadata selection
  - web target context
  - Go SDK workspace metadata target query params
  - TS SDK workspace metadata target query params
  - Forge backend target branch selection
  - Forge backend target-aware import fallback
  - Forge backend navigation target branch selection
- Added local module replaces in `agently/go.mod` for:
  - `../agently-core`
  - `../forge`
- Added local module replace in `agently-core/go.mod` for:
  - `../forge`
- Added explicit web branch roots for the currently restored top-level web
  windows by introducing `web/main.yaml` / `web/main.js` copies where needed
- Migrated the first deeper web-owned metadata subtrees into explicit `web/`
  folders for key windows such as:
  - `schedule`
  - `chat/new`
  - `chat/conversations`
- Added initial explicit mobile branch scaffolding for key windows:
  - `chat/new`
  - `chat/conversations`
  - `schedule`
  - `schedule/history`
  across:
  - `android/phone`
  - `android/tablet`
  - `ios/phone`
  - `ios/tablet`
- Expanded explicit branch scaffolding beyond chat/schedule to additional
  web-facing windows such as:
  - `agent`
  - `agent/pick`
  - `mcp`
  - `model`
  - `oauth`
  - `preferences`
  - `tool`
  - `workflow`
  - `workflow/conversation`
- Added the first real mobile form-factor specialization on scheduler metadata:
  - phone schedule tables now use a more compact column set
  - phone/tablet run-history tables now restore quick filter, pagination, and
    open-chat affordances
  - phone/tablet schedule-history windows now use branch-local run-history
    tables instead of inheriting the desktop table
  - phone/tablet schedule-history windows now use different paging density
  - tablet run-history keeps a richer column set than phone
  - phone/tablet scheduler datasources now use different page sizes instead of
    sharing desktop-sized defaults
- Added the first real mobile chat-list specialization:
  - phone conversation tables now show a compact title-only view
  - tablet/web keep the richer title + id layout
- Added the first real mobile chat/conversations specialization:
  - phone chat/conversations windows now use a shorter conversation viewport
  - tablet/web retain the roomier split-view chat height
- Added the first real mobile single-chat specialization:
  - phone chat windows now disable command-center mode
  - tablet/web retain the richer command-center composer
- Added the first real mobile chat-dialog specialization:
  - phone chat settings/queue/file-browser dialogs now use phone-sized modal dimensions
  - phone approval-editor dialogs now use phone-sized modal dimensions
  - tablet chat settings dialogs now use roomy touch sizing without inheriting desktop min-width constraints
  - tablet chat settings panels now use tablet-specific form spacing instead of
    mirroring the desktop panel layout
  - tablet approval-editor dialogs now also diverge from web sizing
  - tablet queued-turn tables now use tablet-specific column sizing and expose a
    quick filter instead of mirroring the desktop queue table
  - tablet queued-turn datasources now back that quick filter with a
    tablet-specific contains-style filter set
  - tablet queue/file-browser dialogs now also diverge from web sizing
  - web retains the desktop-sized settings and approval-editor dialog sizing
- Added the first real mobile workflow specialization:
  - phone workflow tables now use a title-only list
  - tablet workflow tables now use explicit touch-oriented column sizing instead
    of mirroring the web table
  - tablet workflow browsing now exposes a quick filter, with a tablet-specific
    workflow datasource filter set
  - phone run-workflow dialogs now use phone-sized modal dimensions
  - tablet/web keep the richer table and roomier dialog sizing
- Added the first real mobile tool-window specialization:
  - phone tool windows now use a single-column detail layout
  - phone run-tool dialogs now use phone-sized modal dimensions
  - tablet tool tables now use explicit touch-oriented column sizing instead of
    mirroring the web table
  - tablet tool browsing now uses contains-style quick filtering instead of the
    desktop exact-match filter behavior
  - tablet tool windows now use a different list/detail split balance than web
  - web keeps the desktop side-by-side detail balance
- Added the first real mobile model-window specialization:
  - phone model windows now use a single-column layout
  - phone model tables are more compact than web/tablet
  - tablet model tables now use explicit touch-oriented column sizing instead of
    mirroring the web table
  - tablet model browsing now exposes a quick filter, with a tablet-specific
    model datasource filter set
  - phone/tablet model datasources now use different page sizes
  - tablet model windows now use a different list/detail split balance than web
- Added the first real mobile preferences specialization:
  - phone preferences now use a single-column form layout
  - tablet preferences now use roomier form spacing
  - web keeps the denser two-column preferences form
- Added the first real mobile OAuth specialization:
  - phone OAuth windows now use a single-column layout
  - phone OAuth tables now use a more compact credential list than tablet/web
  - tablet OAuth tables now use explicit touch-oriented column sizing instead of
    mirroring the web table
  - tablet OAuth browsing now exposes a quick filter, with a tablet-specific
    OAuth datasource filter set
  - tablet OAuth windows now use a different list/detail split balance than web
- Added the first real mobile MCP specialization:
  - phone MCP windows now use a single-column layout
  - phone OAuth-picker dialogs for MCP now use phone-sized modal dimensions
  - phone OAuth-picker tables now use a compact name-only list
  - tablet MCP tables now use explicit touch-oriented column sizing instead of
    mirroring the web table
  - tablet OAuth-picker tables now also use explicit touch-oriented column
    sizing instead of mirroring the web picker table
  - tablet MCP browsing now exposes a quick filter, with a tablet-specific MCP
    datasource filter set
  - branch-local OAuth picker imports were corrected for target folder nesting
  - tablet MCP windows now use a different list/detail split balance than web
- Added tablet-specific dialog divergence so tablet no longer blindly mirrors web on:
  - MCP OAuth picker dialogs
  - run-tool dialogs
  - run-workflow dialogs
- Added the first real mobile agent specialization:
  - phone agent windows now use a single-column layout
  - phone agent tables now use a more compact list than tablet/web
  - tablet agent tables now use explicit touch-oriented column sizing instead
    of mirroring the web table
  - tablet agent secondary tables (chains/tools/knowledge) now also use
    explicit touch-oriented sizing instead of mirroring web
  - tablet agent nested datasources now auto-select detail items so the
    split-view editor opens populated
  - tablet chain editors now use denser 2-column tab layouts and a roomier
    agent-lookup dialog instead of the desktop editor defaults
  - phone/tablet agent datasources now use different page sizes
  - tablet agent windows now use a different list/detail split balance than web
- Added the first real mobile agent-picker specialization:
  - phone agent pickers now use roomier touch padding in the sticky action bar
  - tablet agent pickers now use intermediate touch padding
  - web retains the densest picker footer spacing
- Added the first real mobile workflow-conversation specialization:
  - phone workflow conversation windows now use a shorter read-only chat height
  - tablet workflow conversation windows now use an intermediate viewport height
  - web retains the roomier conversation viewport
- Wired iOS app bootstrap to pass explicit `ios` + `phone|tablet` target context
  into:
  - Forge runtime metadata loading
  - workspace metadata SDK requests
- Unified iOS target capability construction so metadata requests and Forge
  runtime now reuse the same capability helper
- Wired Android app runtime to pass explicit `android` + `phone|tablet` target
  context into workspace metadata SDK requests
- Unified Android capability construction so Forge runtime target context and
  metadata request target context now reuse the same capability helper
- Added Gradle wrapper files for:
  - `agently/android`
  - `agently-core/sdk/android`
- Verified Android Gradle builds under JDK 17 with:
  - `gradle :app:testDebugUnitTest`
  - `gradle test` in `agently-core/sdk/android`
- Added a workspace-level `/Users/awitas/go/src/github.com/viant/go.work` for
  local multi-repo development instead of relying on committed module-level
  `replace` directives
- Added request-scoped SDK debug/logging support across:
  - Go HTTP SDK
  - TypeScript SDK
  - iOS SDK
  - Android SDK

### In Progress

- Replacing copied mobile scaffolds with true platform/form-factor-specific
  metadata as UX diverges
- Keeping the migration document and status current

### Deprioritized

- Deeper mobile specialization of the Forge `oauth` window itself.
  Mobile authentication already flows through native auth surfaces, so this
  metadata is secondary compared with chat/workflow/tool/model/preferences
  branches.

### Not Started Yet

- Nothing foundational remains blocked; remaining work is deeper branch
  specialization and repeated device/browser smoke as those branches change

## Verification Status

Verified in this pass:

- web target-context helper and query context tests pass
- web UI tests pass for:
  - client target context
  - submit/query context propagation
  - chatter prompt contract
  - detail panel payload hydration
  - top-nav main-pane replacement behavior for `Automation` / `Runs`
  - chat chrome selection logic tied to the active main window
  - non-chat main-window header/close visibility rules
- live web metadata endpoints return `ok` for:
  - `chat/new`
  - `schedule`
  - `schedule/history`
- terminal Playwright browser smoke confirms:
  - the web shell loads successfully
  - `Automation` opens the scheduler window
  - `Runs` opens the run-history window
  - scheduler/runs main-pane windows expose a visible header with a close action
  - closing the active scheduler/runs window returns the main pane to chat
- Go HTTP SDK workspace metadata target query params test passes
- Go HTTP SDK request-scoped session debug headers are verified
- TypeScript SDK workspace metadata tests pass with target-context query params
- TypeScript SDK request-scoped session debug headers are verified
- embedded metadata window regression test passes for the restored web window set
- Forge backend metadata tests pass for:
  - target branch selection
  - target-aware import fallback
- Forge backend navigation tests pass for target-aware branch selection
- iOS Swift package tests pass after explicit target-context app bootstrap wiring
- iOS SDK workspace metadata request test passes with target-context query items
- iOS SDK request-scoped session debug headers are verified
- iOS app package tests pass for:
  - shared metadata target context seeding
  - Forge runtime explicit iOS target wiring
- iOS app package tests continue to pass after unifying target capability helper
- embedded metadata regression tests pass after the first explicit `web/`
  subtree migration
- Android SDK Gradle tests pass under JDK 17
- Android SDK request-scoped session debug headers are verified
- Android app unit tests pass under JDK 17 after updating the target-context
  call sites
- Android app unit tests continue to pass after unifying Forge/runtime and
  metadata-request capability sets
- Android wrapper files were generated successfully
- embedded metadata regression tests pass for the first explicit mobile branch
  roots
- embedded metadata regression tests now pass for the full discovered explicit
  target branch tree, not only hand-picked branches
- embedded metadata regression tests now also verify exact branch resolution for
  every discovered explicit target branch
- embedded metadata regression tests now verify mobile scheduler tables are
  intentionally specialized rather than accidental copies
- embedded metadata regression tests now verify phone/tablet scheduler paging
  density differs intentionally
- embedded metadata regression tests now verify mobile conversation tables are
  intentionally more compact on phone than tablet/web
- embedded metadata regression tests now verify chat/conversations phone height
  differs intentionally from tablet/web
- embedded metadata regression tests now verify phone chat windows use a simpler
  composer mode than tablet/web
- embedded metadata regression tests now verify phone chat dialog sizing differs
  intentionally from tablet/web
- embedded metadata regression tests now verify mobile workflow metadata is more
  compact on phone than tablet/web
- embedded metadata regression tests now verify mobile tool metadata is more
  compact on phone than tablet/web
- embedded metadata regression tests now verify mobile model metadata is more
  compact on phone than tablet/web
- embedded metadata regression tests now verify mobile preferences metadata is
  simpler on phone than tablet/web
- embedded metadata regression tests now verify mobile OAuth metadata is simpler
  on phone than tablet/web
- embedded metadata regression tests now verify mobile MCP metadata is simpler
  on phone than tablet/web
- embedded metadata regression tests now verify mobile agent metadata is simpler
  on phone than tablet/web
- embedded metadata regression tests now verify agent-picker phone/tablet
  padding differs intentionally
- embedded metadata regression tests now verify workflow/conversation phone
  height differs intentionally from tablet/web
- local `agently` module wiring now resolves to:
  - workspace-level `go.work`
- `agently-core` and `forge` local wiring now resolves via the same workspace
  file instead of committed `go.mod` replace directives
- Go HTTP SDK request-scoped debug headers are verified
- TypeScript SDK request-scoped debug headers are verified
- iOS SDK request-scoped debug headers are verified
- Android SDK request-scoped debug headers are verified
- `agently-core` embedded UI handler tests now verify target-aware window and
  navigation responses at the served HTTP boundary
- wrapper-based Gradle execution is currently blocked in this environment by a
  local TLS trust issue when downloading the Gradle distribution, even though
  global `gradle` execution and all Android verification passed

Still worth repeating as follow-up smoke:

- real browser interaction after future metadata branch specialization
- iPhone/iPad simulator interaction after future iOS-specific metadata changes
- Android emulator interaction after future Android-specific metadata changes

Current unrelated blockers observed during broad local builds:

- `forge` full `go build ./...` is blocked by missing `go.sum` entries in MCP
  packages
- `agently` full `go build ./...` with local replaces is blocked by unrelated
  existing `debugf` compile errors in local `agently-core` packages outside this
  migration scope

## Rules Going Forward

1. Mobile work must not delete shared web metadata.
2. `web` is a first-class target, not an implied default.
3. `shared/` should stay small.
4. Platform-specific behavior belongs in platform branches.
5. Form-factor-specific layout belongs in phone/tablet branches.
6. A window key may only be removed after all callers are removed or migrated.
