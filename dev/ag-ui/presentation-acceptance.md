# Isolated native presentation acceptance

## Current gate status

Progressive native report and visible inline, rail/docked, detached, and promoted
workspace feeds now pass their production-shell checks, including update and
reload. Recorded feed lifecycle recovery and unknown-status cache preservation
pass regression tests and design review; transitions no native observer recorded
remain unknown rather than inferred. The historical cold-observer investigation
below is superseded by that recorded-fact recovery contract.

A fresh final production-build inline reload plus live update is visible in
`feed-inline-final-clean.png` with an 80px table and scroll viewport and no
error toast. The older `feed-inline-restored-visible.png` included an unexplained
`conversation not found` toast and is not flawless UI acceptance evidence.

Queue cancel, force-steer, both move directions, and full-prompt edit/save,
reload, and actual queued execution now pass. Final Move proof used the same
conversation containing old completed queue records, so it does not avoid the
stale-row defect by changing direction or using a clean queue. Current canonical
snapshot ordering overrides historical admission replay. Composer agent/model
selection restoration also passes. Earlier failures below are historical and
superseded by the final proofs. Final simultaneous fresh-client startup on the
completed production binary received RUN_STARTED in 266ms/263ms and both runs
finished successfully in 792ms/1217ms, with no worker conflict logged.

## Scope and fixture

The actual production web shell consumes a deterministic native report and tool
feed from AG-UI. All rows are synthetic, embedded data. The MCP fixture only
returns a fixed row and has no resources, application bridge, remote URLs,
credentials, or business tools. The model fixture does not contact a provider.

Create a fresh workspace (the generator rejects a nonempty destination):

```sh
python3 dev/ag-ui/init_workspace.py /tmp/agui-presentation-20541 \
  --model-port 20542 --presentation-fixture-port 18255
python3 dev/ag-ui/mock_model.py --port 20542
PRESENTATION_MCP_PORT=18255 node dev/ag-ui/presentation-mcp-fixture.mjs
```

Build the current production frontend and an owned assembly binary. Serve the
actual production bundle using `--ui-dist`; the embedded deployment bundle may
be stale and must not be used as migration evidence.

```sh
cd ui
APPSERVER_URL=http://127.0.0.1:20541 npm run build
cd ..
go build -o /tmp/agui-presentation-server-20541 ./agently
AGENTLY_RUNTIME_ROOT=/tmp/agui-presentation-20541/runtime \
AGENTLY_DB_PATH=/tmp/agui-presentation-20541/runtime/db/agently.db \
AGENTLY_AGUI_MOCK_KEY=fixture-only /tmp/agui-presentation-server-20541 serve \
  --addr 127.0.0.1:20541 --workspace /tmp/agui-presentation-20541 \
  --policy auto --ui-dist /absolute/path/to/agently-ag-ui/ui/dist
```

The fixture deliberately disables authentication and uses only local synthetic
reads with auto policy. This proof does not replace authenticated browser gates.
Use a separate named Playwright CLI session, and do not change other agents'
ports, processes, sessions, databases or workspace state.

## Browser evidence, 2026-10-04

`fixture-report AGUI please` with the simple fixture agent rendered the existing
Forge report runtime, title `AG-UI synthetic report`, and the table `Synthetic
channel delivery` with Synthetic CTV/12 and Synthetic Display/7. The producer
streams seven-character chunks splitting fence names and authoring JSON. The
backend emits typed `agently.rendered-content`; the renderer receives the rich
content without showing raw authoring JSON.

Conversation: `07a7743b-63c9-4486-9b5c-e43a6ef86fc4`.

Artifacts under `output/playwright/agui-presentation/`:

- `report-agui-rendered.png`, `report-agui.sse.txt` (17 events).
- `report-agui.requests.txt`: request 193 is the native chat AG-UI POST.

Selecting `Local presentation fixture agent` and sending `fixture-feed please`
executed real MCP `presentation:fixture_feed`. The native feed notifier emitted
`agently.feed` with the fixed row. The inline feed DOM contained the existing Forge table and correct data.
`Open in workspace` mounted the same authored table DOM and existing
return-to-chat/close/split controls. Visual inspection subsequently found the
inline and promoted feed bodies blank because their renderer height chains
collapsed. These screenshots are failure/DOM-data evidence, not successful
visual acceptance. Inline, rail and promoted-workspace visual gates are reopened;
bounded inline sizing and explicit full-surface rail sizing are being verified.
Reload after an owned server restart restored correct feed data, not yet proven
visible table geometry.

Sending `fixture-feed-update please` on the same conversation performed a new
real tool call and updated the stable row DOM/data to `Updated fixture value`.

Conversation: `4e88e530-19f6-45f4-8af4-c240522fcd28`.

- `feed-forge-inline.png`, `feed-forge-workspace.png`, `feed-updated.png`.
- `feed-update-chat-agui.sse.txt`: 23 official-schema-valid events including a
  real tool call, feed activity, and successful terminal outcome.
- `feed-updated.requests.txt`: request 498 is the update chat AG-UI POST.

Both captured streams pass the pinned `@ag-ui/core` 1.0.1 `EventSchema`.
Supporting application APIs remain visible: feed definitions/data use
`/v1/feeds/.../data`, and the explicit native-and-application compatibility
observer uses `/v1/stream?compatibilityScope=native-and-application`. The chat
submission stage uses `/v1/ag-ui/run`. Request logs include an earlier embedded
legacy-bundle baseline (request 52 `/agent/query`); that earlier stage and
`static-report-rendered.png` are **baseline only**, never AG-UI evidence.
No report export, remote business datasource, patch or business operation was
performed. Report export and deeper host-interaction parity remain separate.

## Historical inactivation and cold-observer investigation (superseded)

The fixture specifies `activation.scope: turn`. A later tool-free native turn
completed, but the feed retained the prior matching result; see
`feed-after-tool-free-turn.png`. This is an observation, not an assertion that
native terminal completion must remove the feed.

`FeedActivation` currently describes data gathering. `feed_resolver.go` selects
all historical matches for `all`, and the latest matching turn for other scope
values. `feedNotifier.EmitInactiveForMissing` explicitly removes only turn-scoped
feeds missing from its supplied current-tool list, but has no production caller.
`backendClient.Query` creates a new notifier for each query, so its active map
cannot establish lifecycle across turns. Canonical feed reduction is a view;
transcript restoration reconstructs feeds from retained payloads.

At the time of this earlier probe, a brand-new AG-UI subscription could not discover a background-only inactive
change existing only in an older completed observer journal. Historical payload
retention, observer completion, or native turn completion cannot be treated as
an explicit inactive receipt. That investigation led to the approved recorded-fact recovery contract: restore
recorded native lifecycle facts durably, and preserve unknown status when no
observer recorded a transition. It does not introduce a new native producer or
infer activation from historical payload or terminal turn status. No backend lifecycle semantics were changed by
this fixture work. Activation/update/rendering evidence does not claim full feed
or workspace/report parity.

Fixture checks:

```sh
python3 -m unittest discover -s dev/ag-ui -p presentation_fixture_test.py
node --test dev/ag-ui/presentation-mcp-fixture.test.mjs
```

## Overlap and second-client verification

A real active native turn plus another composer submission initially duplicated
steering text: the local optimistic request ID was absent from the persisted
native steering user. The additive `clientRequestId` is now forwarded by the
existing steering request, persisted as a versioned scalar tag in the existing
message Tags column, and included in canonical user DTOs. A trusted native
command receipt correlates its generated native message ID to the exact local
request. The SDK coalesces an echo-before-receipt race by those IDs, within the
owned turn. Native canonical IDs and send/steer semantics remain unchanged; no
prompt-text matching was added.

Conversation `1ce9acb1-b535-443b-853d-3e208f681504` displayed the delayed initial
user once plus two separately submitted **identical** steering prompts once
each, both at terminal Ready and after cold reload. Artifacts:
`steering-fixed-terminal.png`, `steering-fixed-reload.png`, and
`steering-fixed-identities.txt`. The earlier failure is retained in
`overlap-duplicate-steering.png` and `overlap-identities.json.txt`.

A separate official HttpAgent client starting a new native run on an already
mounted conversation initially completed while the browser stayed unchanged.
Committed native admission/terminal `aguiUpdated` hints now trigger authorized
run discovery. A new independent client marker appeared without UI reload in
conversation `3997fb26-441c-461b-8704-edc458246089`; see
`independent-client-visible-without-reload.png` and
`independent-client-fixed-result.txt`. Read-command and replay paths do not emit
an endless chain of admission hints. This is native discovery evidence, separate
from the standard external backend UI acceptance.

## Earlier Queue controls and composer restoration evidence

The production Queue feed uses its stock authored Forge controls. Real feed
contexts now resolve the owning conversation from trusted context identity and
unwrap the actual selected row; no text or composite-ID inference is used.
Conversation `8f256ef3-890e-4f97-a272-f660bc7af01b` produced a native queued-turn
DELETE returning 204 and official cancelled outcome. A second queued turn
produced force-steer 202 and left the queue, including after cold reload. The
move control emitted the correct owning conversation/native turn ID but returned
500 `conversation not found`; this was a historical failure, superseded by the final Queue proof below. Request evidence:
`queue-controls-requests.txt`. The real-context regression invokes all three
actual handlers and asserts both native IDs.

Composer restoration is separately verified in conversation
`12b238f1-607e-4d98-979e-b4b52d4eb25d`: presentation agent and local model remain
selected after first-submit route remount and cold reload, and follow-up request
bodies retain both IDs without manual reselection. Artifacts:
`selected-agent-after-first-submit.png`, `selected-agent-cold-reload.png`, and
`selection-preserved-inputs.json`.

The corrected inline renderer has a definite bounded height. Actual viewport
and table heights are nonzero (160px renderer and 80px table/scroll viewport),
verified visually in `feed-inline-restored-visible.png` and measurements in
`feed-inline-visible-check.txt`. This supersedes the earlier DOM-only blank-pane
evidence. Rail, detached and promoted workspace visual gates were independently
rerun by the parent with live update and reload evidence. Feed placement is
server-declared configuration, not an invented client placement toggle.

The final backend binary `/tmp/agui-web-final-20261004` fixes Move's principal
lookup: actual native move requests now return 204. At that intermediate stage this did **not** close
acceptance; the final proof below supersedes those defects. A clean two-item queue persisted the intended reordered sequence,
but the AG-UI view still rendered original order live and after reload. Evidence:
`queue-move-principal-fixed-requests.txt` and
`queue-move-stale-projection-reload.txt`. An older completed native turn also
retained queued lifecycle in the queue storage, interfering with adjacency;
both defects were reported separately for repair.

Simultaneous official clients on an existing history sent fresh runs at exactly
2026-10-04T16:17:44.393Z. RUN_STARTED arrived after 106ms and 149ms; the first
finished after 499ms and the second transitioned queued-to-running after 492ms.
No startup worker conflict was logged. The intentional model delay is distinct
from admission latency. Timestamped official event evidence is retained in
`concurrent-fresh-start-fast.log` and `concurrent-fresh-start-delayed.log`.

## Final Queue control proof

Final backend: `/tmp/agui-web-queue-final-20261004`, with the current production
`ui/dist`; owned runtime and DB remain under `/tmp/agui-presentation-20541`.
The installed Queue feed still has its old preview-bound editor. The UI
normalizes only the known stock editor when the stock save handler is present;
custom-only forms and explicitly authored content bindings remain untouched.
The model opt-in `--cancel-latest-user-only` holds only the initial latest-user
marker for 150 seconds. `--queue-input-log` captures synthetic latest-user input
for execution proof; normal model behavior remains unchanged without these flags.

Conversation `8f256ef3-890e-4f97-a272-f660bc7af01b` contains the old completed
queue entries that exposed the stale-neighbor bug. Current native membership
filters them. Turn `808c30de-5c70-4008-944a-9da828812ddf` moved UP then DOWN
against valid queued neighbor `1b10022c-d511-426a-a88b-c3c660963554`; both POSTs
returned 204. The actual Queue table changed to second/first after UP, retained
that order on cold reload, then changed to first/second after DOWN and retained
that order on reload. Evidence: `queue-final-up-live.txt`,
`queue-final-up-reload.txt`, `queue-final-down-live.txt`,
`queue-final-down-reload.txt`, and `queue-final-controls-requests.txt`.

A later editor test deliberately moved B up/down to trigger a refresh that
reseeded table selection. Save now takes native identity and full content from
one coherent editor snapshot, never from a different selected row. Native B
`1857c859-2370-41f5-a5c0-edd094747102` received PATCH 204; its 698-character
body matched the full cold-reloaded editor value exactly. The actual queued
model request consumed that same full input exactly once. Official run
`3d059ad6-6bdb-4a5d-a0f6-8cab1652f692` finished successfully at
2026-10-04T17:29:22.012Z. Browser returned Ready with no remaining queued rows.
Evidence: `queue-coherent-editor-reloaded.png`,
`queue-full-editor-patch-body.txt`, `queue-full-editor-reload-value.txt`,
`queue-full-editor-run-events.log`, and
`queue-full-editor-execution-proof.json`. The earlier wrong-row save was
retained as failure evidence rather than counted as passing acceptance.

Final concurrent fresh clients sent at 2026-10-04T17:30:09.496Z on existing
history. Runs `bf593c89-7342-44bb-9602-e7a46b097a2c` and
`f257c062-a5e3-4629-bbd9-07cc4b0030c8` both started promptly and completed
successfully. See `concurrent-final-one.log` and `concurrent-final-two.log`.
