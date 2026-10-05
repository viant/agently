# Agently

Agently is an agentic application framework and ready-to-run server built on
[Agently Core](https://github.com/viant/agently-core). It combines durable agent
execution with a CLI, embedded web application, native mobile shells and
workspace-driven configuration.

Use it to build assistants that work with your tools and data, retain useful
conversation history, request approvals, coordinate linked agents and render
interactive results. Provider integrations and application metadata are
configurable; the framework is not tied to one business domain.

## How the pieces fit

| Layer | Responsibility |
| --- | --- |
| Agently Core | Model/tool execution, persistence, recovery, authentication, goals, scheduling and SDK contracts |
| Workspace | Agents, models, MCP clients, tools/policies, intents, templates, feeds and application metadata |
| Agently server | Assembled HTTP/BFF service, CLI, application integrations and embedded assets |
| Forge | Generic metadata-driven controls, windows, layouts, charts, tables and inline content |
| Web/iOS/Android shells | Navigation, conversation coordination, hosted workspaces, native interaction and presentation |

The server supports OpenAI, Vertex AI Gemini/Claude, Bedrock Converse/Claude,
Grok, InceptionLabs and Ollama through the configured core adapters. Actual
models, credentials, streaming and multimodal capabilities depend on the
selected provider. Optional MCP exposure and A2A endpoints let other clients
interoperate with the application.

## Build and start

Requires Go 1.25.8 or newer. Web builds also require Node.js/npm. For this
AG-UI branch, first arrange the compatible sibling dependencies described in
[local development](#local-development).

~~~bash
# From the repository root
go build -o ./bin/agently ./agently

# Serve the workspace, API and embedded application
./bin/agently serve -a :8080 -w /path/to/workspace

# Submit a prompt or use the interactive CLI
./bin/agently query -q "Summarize the project documentation"
./bin/agently query
./bin/agently list-tools
~~~

Configure the workspace's selected model and its provider credentials before
making a model request. A web build embeds application assets into the server;
use the safe [UI build workflow](#web-application-development) after editing them.

Useful server options:

| Option | Purpose |
| --- | --- |
| -a / --addr | Listen address; default :8080 |
| -w / --workspace | Workspace path |
| -p / --policy | Coarse tool policy: auto, ask or deny |
| --expose-mcp | Optional MCP tool exposure, with configured port/patterns |
| --ui-dist | Explicit local UI asset override |
| -d / --debug | Debug diagnostics |

The CLI can connect to a remote server with --api and its supported bearer,
session or OOB authentication options. See the command's help before selecting
a deployment-specific credential method.

## Configure a workspace

Agently's application workspace defaults to ~/.agently; AGENTLY_WORKSPACE or
the server's --workspace option selects another location.

~~~text
workspace/
  config.yaml
  agents/
  models/
  embedders/
  mcp/
  tools/bundles/
  intents/
  templates/
  workflows/
  feeds/
~~~

A basic application configuration selects resources by their workspace IDs:

~~~yaml
default:
  agent: assistant
  model: configured-model

auth:
  enabled: true
  cookieName: agently_session
  local:
    enabled: true
~~~

Define an agent under agents/:

~~~yaml
id: assistant
name: Project Assistant
temperature: 0
parallelToolCalls: true
prompt:
  text: "{{.Task.Prompt}}"
  engine: go
~~~

Configure the selected model/provider in models/, then choose tool bundles,
knowledge/resources and intent profiles for the agent. Imported YAML fragments
support shared, keyed and scoped parameterized configuration. Exact parameters
retain their YAML types; overrides stay within their intended scope.

Workspace resources are editable through the application's APIs or as authored
files. For the configuration contracts, start with the core
[workspace guide](../agently-core-ag-ui/doc/workspace-system.md),
[agents and prompts](../agently-core-ag-ui/doc/prompts.md),
[providers](../agently-core-ag-ui/doc/llm-providers.md),
[tools](../agently-core-ag-ui/doc/tool-system.md) and
[templates](../agently-core-ag-ui/doc/templates.md).

## Authentication and tool policy

The framework supports local sessions, JWT RSA/HMAC and OAuth BFF, SPA, bearer
and mixed modes. The BFF keeps the configured session/current cookies and
headers in use across chat, metadata, fonts, uploads, reporting and MCP calls.
Credential reuse for MCP is scoped by user, origin and audience rather than
copied into arbitrary tool requests. Distributed token refresh supports
multi-instance deployments.

Local development may use a default development user. Remove development
auto-login and select the deployment's identity provider for a protected service.
See the core [authentication](../agently-core-ag-ui/doc/auth-system.md) and
[MCP integration](../agently-core-ag-ui/doc/mcp-integration.md) guides.

Tool policy has two layers: --policy supplies coarse auto/ask/deny behavior;
bundle match rules supply none/prompt/queue approval. Denial happens before
approval. An approval is not permission to bypass the underlying tool policy.

~~~yaml
match:
  - name: "project:*"
    approval:
      mode: queue
~~~

Approval editors can use selectors to extract and write back editable data.
Built-in checkbox_list and radio_list editors support collection selection.
Their configured callback and receipt remain associated with the original
native tool operation; changing views or replaying the outcome does not create
a second tool invocation.

## Conversations, AG-UI and SDKs

The outward Go HTTP, TypeScript, Swift and Kotlin SDKs use **AG-UI**. Submission,
canonical bootstrap, observation, attachment, cancellation and continuation use
the configured BFF client. Supporting native application APIs remain available.
Host-side Go Backend.Query and internal executor calls remain native and do not
loop through the public AG-UI endpoint. Go HTTP Query/RunAGUI and the CLI use
the run SSE directly; the CLI answers interrupts with standard resume entries.
Scoped application events remain a separate supporting API.

Standard POST /v1/ag-ui/run requests and SSE events carry runs, messages,
frontend tools/results, state, interrupts, resumes and subagent attribution.
Versioned **Agently extensions** carry native presentation, goals, approvals,
queued-turn control, workspace/datasource commands, feeds and MCP Apps.
Applications integrating a generic AG-UI client should keep that distinction
explicit.

The coordinator owns submitted work independently of a mounted view. Reopening
History or a pane attaches to its durable journal rather than sending another
prompt. Native conversation IDs and opaque wire thread IDs remain separate;
an authenticated aguiThreadId reference lets the SDK reopen the original wire
thread while UI, history and application hints use native identity. Account
changes invalidate previous transport state and bindings.

Conversation SDKs provide only the AG-UI interaction path. There is no legacy
mode, query resubmission or unscoped conversation-stream fallback. Supporting
BFF APIs remain available, including dedicated readConversationHistory and
readApplicationState helpers for authorized native/read-only history.
A shared reader denied access to an owner's private journal does not acquire
that journal or initiate execution.

See the core [SDK guide](../agently-core-ag-ui/doc/sdk.md),
[TypeScript](../agently-core-ag-ui/sdk/ts/AG-UI.md),
[Swift](../agently-core-ag-ui/sdk/ios/AG-UI.md),
[Kotlin](../agently-core-ag-ui/sdk/android/AG-UI.md) and
[operation matrix](../agently-core-ag-ui/doc/ag-ui-operation-matrix.md).
Passing SDK/source checks is not a claim of universal protocol or product-shell
parity.

## Workspaces and visual results

The application distinguishes navigation, conversation-owned hosted workspaces
and message/turn-owned inline content. Metadata defines controls, dialogs,
window placement, chart/table content and actions. Mobile targeting has explicit
platform/form-factor branches so a mobile change does not remove web metadata.

Named theme manifests, scoped application/workspace CSS and authenticated
native font assets support shared appearance across shell and hosted content.
Layouts and authoring data remain owned by the workspace. Tool feeds retain
their actual persisted operation identity and can appear inline or detached.
Developer execution detail and user-facing progress are separate surfaces.

Reports capture the authored document and exact scoped dataset requests before
execution. Begin, compile, complete and activate are distinct phases; saved
completion is distinct from active context. Verified frozen results can restore
a completed report without an implicit dataset rerun. Export/publication are
explicit actions. Application-specific forecast evidence binding remains
disabled at startup while its evidence gates are pending.

Read [workspace ownership/appearance](doc/workspace-ui.md),
[platform architecture](multi-platform.md),
[core UI scopes](../agently-core-ag-ui/doc/ui-ownership-model.md),
[feeds](../agently-core-ag-ui/doc/feed-system.md) and
[MCP UI](../agently-core-ag-ui/doc/mcp-ui.md).

## Goals, scheduling and recovery

Goals retain objectives, budgets/accounting, pause/resume and scheduler wakeups.
Cron, interval and adhoc schedules use distributed leases. Queued turns,
elicitation, approvals and continuation retain native ownership and admitted
tool identity. Management commands use existing domain services rather than
requiring a model round trip.

Async start/status/cancel tools and linked agents retain parent/child invocation
attribution. Disconnecting an observer is distinct from canceling the native
execution. Durable receipts and canonical history let recovery reconcile what
actually happened, rather than assume success or blindly repeat a tool.

Use AGENTLY_SCHEDULER_API and AGENTLY_SCHEDULER_RUNNER to separate API and runner
deployments. See [scheduler](../agently-core-ag-ui/doc/scheduler.md),
[async operations](../agently-core-ag-ui/doc/async.md),
[goals](../agently-core-ag-ui/doc/autonomous.md) and
[approval coordination](../agently-core-ag-ui/doc/ag-ui-approval-coordination.md).

## Optional proactive context compaction

Proactive compaction is off until an agent explicitly sets:

~~~yaml
contextCompactionPercent: 80
~~~

The selected model must separately declare its actual positive
options.contextWindow capacity. At the percentage threshold, the runtime
compacts eligible completed history, rebuilds/recounts the request and resumes.
Latest-user, pending-operation and completed-tool identity protections remain.
Durable full-history barriers cover failures/restarts. The threshold is a
trigger, not a hard context cap.

Exact prepared-input counting is implemented for OpenAI Responses API models
through their normal authenticated HTTP client. Chat Completions, the ChatGPT
backend and unsupported counters do not silently become character estimates.
Omitting the setting adds no proactive count/compaction calls; ordinary reactive
provider-limit recovery remains available.

See [configuration and verification](../agently-core-ag-ui/doc/proactive-context-compaction.md).
Live provider tests require explicit setup; ordinary unit/HTTP failure fixtures
do not make provider business calls.

## Persistence and deployment

SQLite is the workspace default; MySQL is available through the configured
connection. Apply the versioned MySQL schema for deployment. AG-UI reuses
existing conversation, run and call_payload tables: protocol projection,
admission/lease and ordered journal payloads are isolated from native execution
rows and ordinary payload classes. Native APIs cannot overwrite protocol
identity/state. Cleanup follows owned references in the existing transaction.

| Environment setting | Purpose |
| --- | --- |
| AGENTLY_WORKSPACE | Workspace root |
| AGENTLY_ADDR | Listen address |
| AGENTLY_DB_DRIVER / AGENTLY_DB_DSN | Persistence driver/connection |
| AGENTLY_UI_DIST | Explicit UI asset directory |
| AGENTLY_DEBUG | Global diagnostics |
| AGENTLY_SCHEDULER_API / AGENTLY_SCHEDULER_RUNNER | Scheduler deployment roles |
| AGENTLY_CLEANUP_ENABLED | Enable periodic cleanup |
| AGENTLY_CLEANUP_INTERACTIVE_MODE / SCHEDULED_MODE / ORPHAN_MODE | off, dry-run or execute policies |

Keep credentials in the configured provider/identity resources, provision
schema and assets, and select API/runner roles for the deployment. Request-scoped
SDK SessionDebug settings and X-Agently-Debug headers allow diagnostics without
turning on global debug. See [cleanup policies](doc/database-cleanup.md) and the
core [storage/schema guide](../agently-core-ag-ui/doc/ag-ui-storage-reuse.md).

Persistence readers/writers are authored in the core's dql/ and adjacent SQL.
The stock Endly task in its e2e/datly directory runs endly -t=transcribe and
overwrites generated component artifacts. Application lifecycle hooks remain
authored. There is no separate regeneration workflow; see the
[transcription task](../agently-core-ag-ui/e2e/datly/transcribe.yaml).

## Local development

This branch's Go module intentionally selects compatible sibling checkouts:

~~~text
agently-ag-ui/
agently-core-ag-ui/
forge-ag-ui/
mcp-ag-ui/
mcp-protocol-ag-ui/
~~~

Read go.mod for their replacements and the pinned Datly dependency graph.
An optional parent go.work may support experiments, but it should not silently
select incompatible revisions.

Web dependencies are declared separately in ui/package.json: the core TypeScript
SDK points to the AG-UI core fork, while the current Forge npm source points to
the sibling forge checkout. A Go replacement does not change that npm source.
iOS package links select the core SDK/Forge forks; AGENTLY_IOS_SDK_PACKAGE_PATH
provides an explicit SDK override. Android normally uses pinned android/deps;
local AG-UI changes require the explicit sibling-source flag.

~~~bash
# From this repository root
go build -o ./bin/agently ./agently
(cd ui && npm ci && npm test && npm run build)
swift test --package-path ios
(cd android && ./gradlew -Pagently.android.useSiblingSources=true   :app:testDebugUnitTest :app:assembleDebug)
~~~

Android requires JDK 17 and the configured Android SDK. Device signing,
installation and service-connected acceptance are separate:
[Android workflow](doc/android.md), [iOS workflow](doc/ios.md).

### Web application development

~~~bash
# Safe build and sync to the embedded deployment bundle
(cd ui && npm run build:embed)
# Or use the repository wrapper
./e2e/build-ui-embed.sh

# Include the updated assets in the server
go build -o ./bin/agently ./agently

# Vite development server
(cd ui && npm run dev)
~~~

The safe sync workflow preserves deployment/ui/init.go. Avoid replacing that
directory with a raw destructive copy of ui/dist.

### CLI tools and extension points

~~~bash
./bin/agently query --api https://agent.example -q "Summarize the README"
./bin/agently list-tools --api https://agent.example
./bin/agently mcp list --api https://agent.example
./bin/agently mcp run -n project/read -a @args.json --api https://agent.example
./bin/agently chatgpt-login --clientURL "scy://configured-client"
~~~

Select authenticated CLI options appropriate for the deployment. Optional
MCP tool exposure uses --expose-mcp plus configured tool patterns; A2A services
publish /.well-known/agent.json and /v1/api/a2a endpoints. Custom applications can
reuse the core runtime and SDKs, add their own tools and metadata, or compose a
different shell without replacing the execution/persistence contracts.

## Repository map and further reading

| Path | Contents |
| --- | --- |
| agently/ | Binary entry point |
| cmd/agently/ | CLI commands |
| serve.go, runtime/, bootstrap/ | Server assembly and workspace configuration |
| metadata/, deployment/ui/, ui/ | Authored UI metadata, embedded assets and web source |
| ios/, android/ | Native application shells and their dependency selection |
| doc/, e2e/, preview/ | Guides, acceptance workflows and development previews |

Start with the [core documentation index](../agently-core-ag-ui/doc/README.md)
for framework internals. Keep application-specific acceptance and integration
evidence in their dedicated guides rather than treating it as a generic
framework guarantee.
