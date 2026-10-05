# AG-UI assembly development

## Current milestone and scope

The assembled backend now includes all 31 pinned AG-UI 1.0 wire-event schemas
(`@ag-ui/core`/`@ag-ui/client` 1.0.1), a durable Datly 1.0 journal, scoped
public tool IDs, frontend/root/nested client-tool handoff, detached-run
`parentRunId`, goal/state resource commands, workspace/datasource/lookup/feed
operations, and `run.get`, `run.events.list`, and `run.cancel`. Forge fence
extraction is performed by the backend before legacy TypeScript client parsing.
The official CopilotKit shell is an interoperability harness; production web,
mobile, and CLI shells are later work.

The pinned backend/SDK milestone is implemented and verified; later application
route parity remains separate. Approval and graph recovery have native and
assembled protocol evidence. Interactive MCP Apps
automatic app-scope resolution was approved and implemented on 2026-10-03;
scoped aliases, native policy/approvals, private receipts, replay and restart
were verified with the builtin renderer. Preserve host result `_meta` and keep
the full host result out of model history. The `../mcp-protocol-ag-ui` sibling
worktree replacement is needed for local builds until its wire fix is published
and pinned. No commits have been made. MySQL 8.4 checks cover `newAGUI` table
two-runtime lease/resume races and exact JSON. The full legacy schema bootstrap
hit a preexisting invalid `op_id` index and was not validated.

This worktree's `go.mod` replaces `github.com/viant/agently-core` with the sibling `../agently-core-ag-ui`. The existing assembly routes `/v1/` requests through the core API handler, including `POST /v1/ag-ui/run`; no additional assembly routing code or production UI replacement is needed.

The fixture uses a real assembled backend and its normal OpenAI provider, with a deterministic loopback mock model. It requires no external provider credentials. All conversation databases, state, indexes and snapshots belong to the new temporary workspace. The tool fixture exposes only `system/os:getEnv`, and the mock asks only for `AGENTLY_AGUI_FIXTURE_VALUE`.

From this worktree, build and create a fresh workspace:

```sh
go build -o /tmp/agently-ag-ui ./agently
python3 dev/ag-ui/init_workspace.py /tmp/agently-ag-ui-demo
```

Run the mock in one terminal:

```sh
python3 dev/ag-ui/mock_model.py --port 18082
```

Run the assembled backend in another terminal:

```sh
AGENTLY_AGUI_MOCK_KEY=fixture-only AGENTLY_AGUI_FIXTURE_VALUE=fixture-value \
  /tmp/agently-ag-ui serve --addr 127.0.0.1:18081 \
  --workspace /tmp/agently-ag-ui-demo --policy auto
```

The backend and mock bind only loopback. They fail on an occupied port instead of replacing another process. Use `--model-port` when generating the workspace to change the mock port; use `--addr` to change the backend port. The workspace generator refuses any nonempty destination. Stop each process with Ctrl-C.

After the sibling core SDK dependencies are installed, run the upstream `HttpAgent` smoke:

```sh
node dev/ag-ui/smoke.mjs
```

Set `AGENTLY_AGUI_URL` to override the endpoint. The smoke validates capability discovery, real streamed chat, real tool execution and reduction by upstream AG-UI. An independent response clone validates every raw wire event against the official AG-UI 1.0 schema while the client consumes the live stream. It treats RUN_ERROR as a failure explicitly because upstream delivers that event via a callback and resolves the run promise.

For resource checks, enable `features.goals.enabled: true` in the isolated
workspace config and run `node dev/ag-ui/resource-smoke.mjs`. It verifies all
six RFC6902 operations, stale state hashes, native goal lifecycle, live goal
subscription updates, and subscription cancellation. `--expiry` separately
checks successful bounded observer expiry; it does not prove cancellation.
These command envelopes include the required versioned `requestId`.

For automatic goal accounting, run `node dev/ag-ui/goal-accounting-smoke.mjs`
against the same goal-enabled fixture. It completes an initial chat to warm the
provider cache, creates a goal with a 10-token budget, opens an independent
subscription, and executes a second chat. Only the `fixture-goal-accounting`
prompt makes the mock emit a standard streamed 15-token usage snapshot. The
native completed-turn runtime commits accounting and `budget_limited`, and its
publisher refreshes the subscription without a goal management mutation. The
proof validates all raw events against the pinned official schema. A real
1.2-second provider delay also verifies at least one accounted elapsed second.
It does not establish elapsed-time limits or scheduled model wakeup execution.

For actual scheduled model continuation, enable `features.wakeups.enabled: true`
with `minWakeDelaySeconds: 1` and `maxWakeDelaySeconds: 60` in the isolated
workspace, start the backend with `AGENTLY_SCHEDULER_RUNNER=1`, and run
`node dev/ag-ui/goal-scheduler-smoke.mjs`. The existing assembled watchdog polls
every 30 seconds. The proof creates its controller through AG-UI with a two-second
wake delay, observes the pending wakeup, then observes a native model continuation
without another chat POST. Two 15-token turns exhaust its 20-token budget and
stop further continuation. A separate goal is paused before its pending wakeup;
it remains paused at 15 tokens through a full 35-second polling window. This
proof exercises the real native scheduler and model provider, not a run-now API
or fabricated scheduler event.

The core SDK's `TestAGUIGoalHTTPTwoRuntimeCrashTakeover` separately checks real
public subscription recovery: kill only its owned child runtime after accepted
admission, reconnect through another native runtime sharing the workspace, wait
the real one-minute lease expiry, observe a committed change, cancel with owner
scope, and replay the exact journal with one `RUN_STARTED`. It does not shorten
leases or modify journal status to simulate a crash.

The core SDK's `TestAGUIGoalHTTPOverflowReplaysErrorAndFreshSubscriptionRecovers`
uses actual MemoryBus overflow during committed native goal updates. Overflow
closes that subscription with a durable `GOAL_SUBSCRIPTION_ERROR`: replaying the
same accepted input returns the exact terminal error and does not restart it.
Recovery requires a **new subscription run ID** on the same owned thread; its
initial authoritative snapshot and subsequent live updates preserve chat state.
This terminal policy differs from crash recovery of a still-running subscription.

For process restart verification, run
`node dev/ag-ui/restart-smoke.mjs prepare /tmp/agui-restart.json`, stop and
restart only your owned backend with the same workspace, then run
`node dev/ag-ui/restart-smoke.mjs resume /tmp/agui-restart.json`. It checks exact
durable replay, stable scoped tool identity, and continuation of the original
pending tool. This proves persisted handoff recovery, not active model execution
recovery. The temporary JSON contains the local fixture session cookie.

The generator also creates `approval_fixture` with the native queue approval
policy and explicit `queueBehavior: wait` for the same read-only environment
tool. `node dev/ag-ui/approval-smoke.mjs` requires an approval interrupt before
any tool result, then approves and rejects separate runs using standard AG-UI
resume entries. It asserts each decision completes the original public tool
call once and that rejected execution never returns the fixture environment
value. It uses no legacy decision endpoint.

For a counted execution proof, start the mock with
`--approval-effect-log /tmp/agui-new-effect-log.txt` using a new file path, and
set `AGENTLY_AGUI_APPROVAL_EFFECT_LOG` to the same path when running the approval
smoke. The separate `approval_effect_fixture` selects only
`system/exec:execute`; its deterministic command appends one line to that owned
file and prints a fixture marker. The smoke verifies zero writes before the
decision, exactly one approved write, zero rejected writes, and exact resume
replay without another write. This option runs only local fixture commands.

For the external CopilotKit shell:

With the actual assembly and shell running, `bash dev/ag-ui/browser-smoke.sh`
drives the official CopilotKit 1.77.0 component using Playwright CLI. Set
`AGUI_BROWSER_URL` for your owned shell port and `AGUI_PWCLI` for the CLI wrapper
path. It checks default chat, extension-selected backend tool arguments/result,
capability discovery, and Forge safe fallback. It also observes the browser's
actual SSE response for the typed activity and the outgoing extension selection.
Screenshots are saved under the core harness's `output/playwright/`. This is
bounded browser coverage; approval, resource, and restart scripts separately
exercise the official AG-UI HTTP client without browser UI.

The smoke also requests `fixture-forge` from the mock. Its seven-character
stream chunks split Forge fences and authoring JSON; the upstream client must
receive a typed `agently.rendered-content` activity and safe prose with
`[Interactive content]`, with no authoring payload in assistant chat. Tool
assertions verify logical-turn-scoped public IDs and matching result IDs.

```sh
cd ../agently-core-ag-ui/examples/ag-ui-shell
AGENTLY_BACKEND_URL=http://127.0.0.1:18081 npm run dev -- --host 127.0.0.1
```

Use the shell's extension panel with agent `simple` and model `local_mock`. For a tool run, select agent `tool_fixture`, model `local_mock`, and enter `fixture-tool please`. Consult the shell README for its exact proxy environment variable and pinned package versions. This shell uses official upstream CopilotKit components as an interoperability test harness. It is not the upstream Dojo demo application, and this development setup does not replace Agently's production UI.

Backend resource and tool-handoff capabilities are available as described above,
while the final integration and production-shell cutover gates remain open. See
the sibling core `doc/ag-ui-complete-plan.md` and
`doc/ag-ui-operation-matrix.md` for current detailed status; the earlier phase
one capability list is historical.

MCP Apps fixture (owned loopback processes only):

```sh
MCP_APP_PORT=18243 MCP_APP_LOG=/tmp/owned-mcp-app-calls.jsonl node dev/ag-ui/mcp-app-fixture.mjs
AGENTLY_AGUI_URL=http://127.0.0.1:18241/v1/ag-ui/run MCP_APP_AGENT=mcp_app_fixture node dev/ag-ui/mcp-app-smoke.mjs
```

Configure the isolated workspace's native MCP server as `fixture`, Streamable HTTP at `http://127.0.0.1:18243/mcp`, exposing `fixture_view` and policy-guarded `fixture_approved`. Both declare `ui://fixture/app.html`. The deterministic model selects `fixture_view` for `fixture-mcp-app` prompts. Each tool response includes text/image/resource-link content, nested structuredContent and host-only `_meta`; resource reads include resource metadata and top-level metadata. `mcp-app-smoke.mjs` requires backend-issued activities for two conversations, distinct opaque aliases, unchanged server hash, isolated public proxy threads, and byte-identical replay. Its passing output does not establish approval, restart, or browser correctness; those remain separate gates.

## MCP Apps assembled acceptance

Create the isolated workspace with `--mcp-app-port <owned-port>` and start
`mcp-app-fixture.mjs` with `MCP_APP_PORT` and `MCP_APP_LOG` set to owned loopback
resources. The fixture uses a real MCP handshake/session and no credentials.
Run `mcp-app-smoke.mjs --approval` against `AGENTLY_AGUI_URL` to verify distinct
app bindings, complete host result envelopes, exact replay, approve-one-effect
and reject-zero-effect. `mcp-app-restart-smoke.mjs prepare <owned-file>` saves
its fixture cookie privately; stop/reopen only the owned backend with the same
workspace, then run `resume <owned-file>` for persisted alias/receipt/handoff
recovery. Do not publish the temporary cookie file.

The upstream middleware ordinarily makes direct MCP requests. Our external
shell's explicit forwarding adapter removes inherited model inputs and assigns
a separate proxy thread before POSTing through AG-UI. Actual CopilotKit 1.77
iframe initialization and button callback passed, with all three application
requests using `/v1/ag-ui/run`, zero console errors, and upstream development
and iframe-sandbox warnings. Production untrusted iframe origin isolation is
a later shell requirement; the fixture is trusted and credential-free.

Local development additionally replaces `github.com/viant/mcp` with
`../mcp-ag-ui`, based on v0.24.0, for scoped session/auth/redirect retry suppression.
The original dirty dependency checkout is preserved. Publish/pin dependency
changes before consuming these branches without sibling replacements.

## Authenticated Steward backward compatibility

Use an owned loopback backend with an isolated copy of Steward metadata and a
fresh STEWARD_RUNTIME_ROOT. Exclude original databases, runtime state and indexes.
Set STEWARD_OAUTH_CONFIG_URL to the supplied encrypted client reference when
starting the backend. Preserve deployment OAuth scopes and authorization rules.

```sh
go run ./dev/ag-ui/steward-compat --api http://127.0.0.1:20431 \
  --oauth-config '/Users/awitas/.secret/idp_viant.enc|blowfish://default' \
  --oob '/Users/awitas/.secret/awitas_dsp_ui.enc|blowfish://default'
```

This calls the existing SDK's AuthLocalOOBSession with PKCE and the complete
Steward scope set, then validates legacy session/identity, public agents,
conversation listing and web/iOS/Android workspace metadata. On the same cookie
session it checks AG-UI discovery and workspace streams against the pinned
schema, then checks legacy identity again. Tokens stay in memory; failures
report only categories, never server bodies or credentials.

The 2026-10-03 run passed, including all nine AG-UI wire events. Evidence:
/tmp/agui-steward-sdk-oob-compat-20261003.log. It does not invoke live models or
remote business mutations and does not prove native device UI or every remote
MCP business operation. The broader legacy CLI and MySQL bootstrap limitations
in the core completion audit remain separate.

## Production native report and feed presentation

The optional `--presentation-fixture-port` workspace-generator flag adds a
credential-free MCP feed plus an authored Forge table. `fixture-report` streams
static progressive report fences; `fixture-feed` and `fixture-feed-update`
perform real local MCP reads with stable synthetic row identity. Use the current
production `ui/dist` through `serve --ui-dist`, with an owned runtime root and DB
path. See [presentation-acceptance.md](presentation-acceptance.md) for exact
setup, browser artifacts, retained application-API boundaries, and the still-open
native inactivation/cold-observer gate. These fixtures never query business data.
