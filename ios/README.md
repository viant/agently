# Agently iOS Foundation

This directory starts the iOS implementation described in
`/Users/awitas/go/src/github.com/viant/agently/ios-app.md`.

Current scope:

- initial SwiftUI app-shell/state scaffolding is in place
- the app layer now builds as a local Swift package: `AgentlyAppFoundation`
- an Xcode app project now exists: `AgentlyApp.xcodeproj`
- the project is generated from `project.yml` via `xcodegen generate`
- simulator launch has been validated
- launch now shows the active API base URL and exposes `Retry` and
  `Edit Settings` if bootstrap stalls
- the signed-in shell now distinguishes loading from empty state for
  conversation lists, selected conversation content, and artifact refreshes
- applying settings now rebuilds the active SDK client in-process
- settings now normalize pasted API URLs and strip accidental `/v1` or
  `/v1/api` suffixes
- settings now expose quick local connection presets for simulator testing
- the last active conversation is now persisted and restored when the
  workspace comes back
- the sign-in screen now supports in-app secure OAuth via
  `ASWebAuthenticationSession`, and the generated app target registers an
  `agently-ios://` callback scheme for OAuth redirects
- the sign-in flow is now OAuth-only and workspace-first; provider discovery
  comes from the connected Agently workspace and the app drives a single
  secure sign-in path through `ASWebAuthenticationSession`
- auth/bootstrap/live-state failures now emit app-side `Logger` entries to the
  simulator console, and conversation live-state load failures are surfaced
  instead of being silently swallowed
- workspace metadata now uses the richer backend contract, including version,
  capabilities, models, and agent starter-task metadata
- the shared iOS SDK now also exposes a generic `MCPHostClient` for any
  workspace-backed MCP HTTP host instead of hardcoding a Steward-specific flow
- the shell now exposes `New Chat` and `Refresh`, refreshes the conversation
  list after query-created conversations, and supports local conversation
  search on both phone and tablet layouts
- conversation rows now surface stage, agent, and friendlier activity metadata
  so larger histories are easier to scan on both phone and tablet
- the conversation list now sorts locally by most recent activity and exposes a
  one-tap clear action when search hides every row
- approval and elicitation actions now expose in-flight UI state instead of
  only showing failures after the request returns
- pending approval cards now show tool names, status, conversation/message
  context, and pretty-printed JSON payloads for arguments and metadata
- conversation artifacts now load into the signed-in workspace and can open in
  a lightweight preview sheet
- selecting an artifact now lazily downloads generated files or conversation
  files through the shared SDK, fills inline text preview when possible,
  renders downloaded images inline, uses richer markdown/JSON/CSV formatting
  for text artifacts, exposes a share action for downloaded files, and now
  offers Quick Look fallback for downloaded binary outputs on iPhone/iPad
- artifact preview presentation is now routed through a single shared shell
  path so phone and iPad use the same selection and dismissal behavior
- elicitation forms now seed schema default values and preserve basic boolean
  and numeric field types instead of flattening every submitted value to text
- elicitation forms also honor simple `required` schema fields and block empty
  submit paths with inline validation instead of only relying on server errors
- elicitation forms now render simple string `enum` schemas as picker-style
  choices instead of forcing every selectable value through free-form text
- elicitation forms now also respect nullable and multi-type schema hints, show
  field descriptions/examples inline, and switch to better email/URL/numeric
  input behavior when the schema provides those hints
- array and object schema fields now seed as pretty JSON and round-trip back as
  structured JSON payloads instead of being flattened into plain strings
- structured array/object fields now also block submit with inline validation
  when the edited value is not valid JSON or does not match the expected
  container type
- numeric and integer schema fields now also block submit with inline validation
  when the entered value does not parse as the expected numeric type
- email and URL schema fields now also block submit with inline validation when
  the entered value does not match a basic email or http/https URL shape
- date and date-time schema fields now also block submit with inline validation
  when the entered value does not match `YYYY-MM-DD` or ISO 8601 date-time
  expectations
- min/max string length and minimum/maximum numeric schema constraints now also
  block submit with inline validation when a field falls outside those bounds
- exclusive numeric bounds via `exclusiveMinimum` and `exclusiveMaximum` now
  also block submit with inline validation when a numeric value crosses those
  strict limits
- numeric schema `multipleOf` constraints now also block submit with inline
  validation when a numeric value is not an allowed multiple
- structured array and object schema fields now also honor `minItems`,
  `maxItems`, `minProperties`, and `maxProperties` before submit
- structured array schema fields now also honor `uniqueItems` before submit
- structured array schema fields now also validate simple `items` type and
  basic item format constraints before submit
- structured object schema fields now also validate simple nested `properties`
  and nested `required` constraints before submit
- nested structured arrays and objects now recurse through those same `items`
  and `properties`/`required` rules instead of stopping at one level deep
- structured object schema fields now also honor `additionalProperties: false`
  at the top level and inside nested objects before submit
- structured object schema fields now also honor schema-valued
  `additionalProperties` rules at the top level and inside nested objects,
  validating unknown keys instead of only allowing or rejecting them wholesale
- top-level fields and nested structured values now also honor simple `oneOf`
  and `anyOf` schema alternatives, accepting a value when any declared branch
  validates successfully
- top-level fields and nested structured values now also honor simple `allOf`
  schema branches, requiring a value to satisfy every declared branch
- nested array items and nested object properties now also honor string length,
  numeric range, `multipleOf`, and container size constraints instead of only
  validating type/format/shape
- structured array schema fields now also honor tuple-style `prefixItems`
  validation, so indexed array elements can each enforce their own schema
  before trailing `items` validation applies
- structured array schema fields now also honor `contains` plus
  `minContains`/`maxContains`, so arrays can require a minimum or maximum
  number of elements that match a nested schema
- top-level fields and nested structured values now also honor exact `const`
  values, so schema-defined constants are enforced before submit instead of
  only after a server round-trip
- top-level fields and nested structured values now also honor `not`
  disallowed-schema branches, rejecting values that match a forbidden nested
  constraint before submit
- string schema `pattern` constraints now also block submit with inline
  validation when the entered value does not match the expected regex shape
- transcript rows now show saved-turn timestamps, live assistant streaming
  status, auto-scroll toward the latest message, and richer markdown rendering
  for assistant responses
- transcript bubbles now expose a quick copy action so message content is
  easier to reuse directly from the iOS shell
- user transcript bubbles now expose a prompt-reuse action that pushes an older
  request back into the composer for quick iteration
- user transcript bubbles can also replay an older request immediately through a
  single reuse-and-send action when the composer is idle
- sending a prompt now creates an immediate optimistic user row and assistant
  placeholder so the transcript responds before the backend stream fully binds
- while a turn is still running, the composer stays disabled and the
  workspace exposes a real `Stop` action backed by the shared cancel-turn API
- if live updates disconnect, the workspace now exposes a `Retry Live` action
  to reconnect the active conversation stream without a full app reload
- compact-width devices now route through a dedicated phone workspace flow with
  conversation push navigation instead of reusing the tablet split-detail path
- approval and artifact sections can now collapse down to count-labeled headers
  so compact and rotated layouts stay easier to scan during longer sessions
- the workspace header no longer duplicates the composer input and now renders
  a denser metadata summary with agent, model, embedder, and available-agent
  context
- the workspace header now also lets the user select the active agent directly
  from discovered workspace metadata instead of relying on a manual settings
  text field
- composer attachments can now be imported from files, removed before send, and
  uploaded into the query path
- photo-library image picking is now wired alongside file import in the
  composer
- camera capture is now wired in the iOS composer, and the generated app target
  now carries the camera usage description
- voice dictation is now wired into the composer with speech and microphone
  permission prompts and live transcript preview while recording
- the composer action strip now adapts more gracefully on narrow and rotated
  layouts by collapsing into a vertically stacked variant when needed
- the shared iOS foundations that are already buildable live in:
  - `../../agently-core/sdk/ios`
  - `../../forge/ios`
- this package is a bridge to the real destination, which is still an Xcode
  iOS app target under `agently`

Next implementation steps:

- polish iPad and phone shell behavior beyond the current compact-width
  conversation push flow
- validate OAuth sign-in against real Agently workspace configurations
- keep agent selection aligned with workspace-discovered metadata
- wire discovered MCP host endpoints into a real workspace-driven UI once the
  backend exposes them through a stable contract
- connect live transcript and composer flows to a real backend session
- finish the remaining Forge runtime wiring: data sources and signals, now
  that dashboard runtime, targeting resolution, and baseline multi-field schema
  form rendering are in place

Useful commands:

- `xcodegen generate`
- `swift build`
- `xcodebuild -project AgentlyApp.xcodeproj -scheme AgentlyApp -sdk iphonesimulator CODE_SIGNING_ALLOWED=NO build`
