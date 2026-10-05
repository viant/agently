# Real Steward workflow follow-up — 2026-10-04

## Scope

User requested media planning, troubleshooting and report creation using the
Steward workspace, then comparison with `https://steward.agently.viantinc.com/`
using the identical media-plan prompt and `show advertisers` as a chat prompt.
Workspace-supplied styles and themes must remain intact.

Local build: branch `ag-ui`, `/tmp/agui-web-queue-final-20261004`, current
production `ui/dist`, port 20531. Workspace:
`/Users/awitas/go/src/github.com/viant-internal/steward_ai/deployment/steward`.
Separate test runtime/database: `/tmp/agui-steward-workflows-20261004`.
Real BFF authentication and real Media Planner were used; no mock plan result.

## Media planning — created and rendered

One explicitly labeled QA draft was created in each application with identical
prompt text. Inputs: public Viant website, awareness, US marketing/advertising
decision-makers, USD 10,000, 2026-11-01 through 2026-11-30, all supported channels.
No publication, campaign activation, email, or existing campaign edit was requested
or performed.

- Local plan: `plan_81cd85406e034363`, version 1 draft, conversation
  `1af778c6-17d3-400f-8336-32db596daee5`.
- Original-site plan: `plan_fae2dcaf084940c9`, version 1 draft, conversation
  `3fa26967-2615-4067-969a-94b78a3cbba6`.
- Local channel amounts sum to 10,000 and percentages to 100. Both plans contain
  CTV, Video, Display, Audio and DOOH. Allocations/content differ because these
  were independent model-generated drafts; that is not a presentation comparison.
- No DSP advertiser is bound to the local QA draft; do not treat its synthetic
  planning budget or recommendations as approved business inputs.

Real testing found and fixed three gaps missed by the earlier synthetic proof:

1. A bootstrap arriving before the native identity receipt could retain both
   protocol and native user rows. Exact ID correlation now merges that proven
   alias without matching prompt text.
2. Real task-mode events omit `pageId`. The SDK now uses their verified native
   message ID as the page identity, so interim text cannot replace the final
   assistant page under a different synthetic identity.
3. The inline Forge viewport capped the entire multi-tab plan at 18vh. It now
   respects the workspace's authored non-stretch layout (`itemStretch: false`)
   with intrinsic height. Compact flex tables retain a definite viewport;
   docked/hosted behavior and authored styles are preserved.

The fixed General, Channels and Schedule tabs were visually compared with the
original. Other authored tabs were opened and their populated/empty states
captured. Desktop and 720px checks show full content and no document-width
overflow. General feed height increased from about 160px to 1,108px. Both sites
resolve the same canvas color and IBM Plex Sans workspace font; the dark summary,
chart colors, tabs and form styling remain intact.

A fresh, read-only Media Planner get in conversation
`cb34777f-c012-442c-a6e3-3750f165ad81` then completed on the fixed bundle with one
user row, the final draft summary visible without reload, and the full-size feed.

Evidence under `output/playwright/steward-workflows/` includes original/fixed tab
screenshots, `media-plan-channel-totals.txt`, `media-plan-fixed-theme-geometry.txt`,
`original-theme-check.txt`, `media-plan-fixed-narrow-geometry.txt`, and
`media-plan-fixed-live-proof.txt`. The original 85-event journal was captured for
deterministic SDK regression replay; the chat/create operation was not replayed.

## Show advertisers and remaining data-dependent tests

The exact `show advertisers` prompt opens a populated Advertisers workspace in
the original site (conversation `42752b2f-4cc1-4bda-a4ea-2ef371e4e835`). The local
AG-UI build opens the intended workspace but its permission request returns
HTTP 504: permission service timed out. A separate connectivity check confirms
the configured `http://steward.viantinc.com:5000/mcp` cannot be reached from this
machine. Existing local Datly listeners are Platform/Studio services, not a
verified Steward substitute; no shared service was changed.

Local troubleshooting and performance-report creation remain unverified pending
a reachable authorized `STEWARD_MCP_URL` or restored network connectivity. Their
success must not be inferred from media planning or the original site's table.

## Verification

- SDK: 524 tests pass, five opt-in skips; package build passes.
- Feed layout/theme regressions: 19 pass.
- Full web suite: 1,081 pass; the same three pre-existing shared-Forge
  permission/metadata tests fail.
- Production UI build passes; targeted layout detector has no findings.
- `git diff --check` passes.

Logs: `/tmp/agui-media-plan-layout-tests.log`,
`/tmp/agui-steward-media-plan-ui-build.log`,
`/tmp/agui-steward-workflows-web-suite.log`, and
`/tmp/agui-media-plan-layout-scan.json`.

## Report and forecast starter follow-up

The original **Build order performance report** starter was exercised through
its real Order picker, selecting authorized order 2659534 (Chinese Speaker).
Conversation `f638efbe-3849-4007-b3df-9b65db45c54d` produced a Performance Metrics
workspace for September 27–October 4, 2026. Headline KPIs, daily chart/table, bid
funnel, channel/country breakdowns, findings and recommendations rendered. The
report explicitly identifies the possibly incomplete final day. Cold reload
retained the workspace and its values. Evidence: `original-report-workspace.txt`,
`original-report-rendered.png`, `original-report-structure.txt`, and
`original-report-reloaded.txt` in the same artifact directory. This is UI and
workflow evidence, not independent warehouse reconciliation.

The local report starter opens correctly, but its Order picker terminates with
HTTP 500 from `ad_order_lookup/fetch` and a timeout message; Select stays disabled.
See `local-report-picker-result.txt`, `local-report-picker-failed.png`, and
`local-report-requests.txt`. No local report was falsely reported as completed.

The original **Forecast reach and availability** starter selected authorized
audience 7366798, Audience_A18-64_Chinese Speaking, but initially refused the
request. The user confirmed that line and audience name the same entity. The
failure came from contradictory forecasting instructions that demanded a
separate line-to-audience conversion, despite the lookup supplying AudienceId.
Clarifying the already verified audience ID completed the original forecast in
conversation `6dde275c-abf9-421b-bb83-7350e62d4ea2`.

All four forecast tabs were inspected: Overview, Targeting, Evidence and Findings.
Charts, maps and tables visibly render. The October 4 headline values match the
October 4 cube response: 243,939,689 avails, 61,425,000 devices, 473,200 households.
Daily avails for October 2–4 sum to 918,088,272; unique reach is not summed.
Evidence: `forecast-original-run-evidence.json`, `forecast-original-*.png`, and
`forecast-original-evidence-geometry.txt`. A transient raw 504 toast occurred
during this successful long-running original-site forecast and is retained as
failure-state evidence.

Local forecast picker/read and the submitted run failed on MCP connectivity;
`forecast-local-run-outcome.json` records `RUN_ERROR/AGENT_ERROR` for run
`9616edb1-2a24-4143-81f9-557ca4d98992`. No local dashboard parity is claimed.

The local Steward source now normalizes line/line-item/audience forecast and
builder scopes to the same ID and requires the actual targeting-profile input
`AudienceId: [id]`. Ad-order scope remains distinct. Contradictory skill/prompt
instructions were corrected, and the original starter text was restored rather
than depending on renamed wording. All 46 forecast contract tests pass; the new
cases execute actual configured intake patterns and captured scope values.
Log: `/tmp/agui-forecast-alias-contract-tests.log`. These source changes are not
deployed to the original site; live local re-verification still needs data access.

An isolated existing authoritative Steward MCP executable was tried on port
20535 with its configured JWT/OAuth authorization intact. Required metadata and
performance components could not load because resolving their configured database
credential returned PermissionDenied. That temporary instance was stopped; no
credential bypass, connector rewrite or database change was made, and Agently's
endpoint was not redirected to the incomplete service. Sanitized evidence:
`local-authoritative-backend-preflight.json`.

## Workspace layout preservation

Steward's Advertisers/Campaigns menus come from `ui/layout.yaml`. The generic
`WorkspaceSidebar` loads `/workspace/layout` and resolves configured applications,
menus, window actions, parameters, sidebar sizes/splits and top-bar actions;
those business window names are not hard-coded in the generic UI. The live
configuration is captured in `workspace-layout-live.txt`. Six existing sidebar
and menu identity tests pass. Existing workspace styles/theme boundaries remain
intact. This verifies the existing layout schema; it is not a claim that every
possible replacement of shell structure is supported.

## Troubleshooting baseline for simulator parity

The regular site's Troubleshoot ad order starter was exercised through its live
picker for order 2659534. Conversation
`c6093b5e-8ec7-4d0f-867b-1f440c6a16e9` completed and rendered Overview, Delivery
posture, Causal evidence, Configured goal, Supply-path impact, Recent changes,
and Corrective validation. Every tab was opened. The response identifies setup
as the primary blocker with moderate confidence: the setup packet does not
confirm an active flight. It separately identifies restricted inventory and
explains that pacing posture does not prove pacing-control rejection. These
stated limitations are part of the comparison, not independently established
causality.

Evidence: `original-diagnostic-progress.txt`, `original-diagnostic-overview.png`,
`original-diagnostic-causal.png`, and the per-tab `original-diagnostic-*.txt`
captures. Local troubleshooting remains blocked by the data MCP connection.

The full active cross-platform acceptance matrix is now
`cross-platform-parity.md`, with the 16 configured starters inventoried in
`steward-starter-inventory.json`. Successful legacy iOS/Android application
builds are retained as baselines; neither establishes AG-UI simulator parity.

## Follow-up: connectivity recovered and native acceptance underway

The earlier data MCP timeouts are historical. Authenticated lookup and cube
reads now return HTTP 200. A separate registry lifetime defect left discovery
refresh bound to the short initialization context; the repaired local server
retains runtime refresh and exposes 344 tools, including the forecasting suite.

Local report conversation `ce6a912e-abde-4801-aa6a-7c027e8609da` completed native
report run `2eba7218-94c4-4be6-b8b9-0b1f9d03ba32`, while its UI incorrectly
remained at zero of five datasets. The report runtime repair fences stale
requests and hydrates the matching completed native run. The repaired real
report visibly renders its $309 spend; evidence is `local-report-repaired.png`.
A fresh browser cold reload also retains the KPI values, but renders zero report
SVGs and zero tables. The restored authored chart contains a `[MaxDepth]`
placeholder and compiles to a null model, while the authoritative completed run
retains valid chart/table definitions. Full chart/table restoration therefore
remains a failed acceptance gate. Evidence: `local-report-repaired-cold-reload.*`,
`report-cold-hook-visual-shapes.txt`, `report-cold-native-visual-spec.txt`.

A later forecast reached OpenAI but failed because only eleven outputs were
available for twelve declared tool calls. The same error-suppression paths
exist in the regular core source, so this is not proven to be AG-UI-specific.
The source repair preserves call identity, persists truthful tool errors and
stops before another model request when infrastructure failure prevents a
replayable output. Streaming/nonstreaming regression suites and focused race
checks pass. The rebuilt server's single live starter rerun in conversation
`42a342be-1407-4282-b92d-85b5795d8a33` completed without a provider error, but
refused to drop the selected audience's Viant segment because that filter is
absent from the live cube tool schema. No dashboard or twelve-call batch was
produced; forecasting acceptance remains open.

Both current native applications now use AG-UI with existing BFF authentication.
Real QA draft creation and pane evidence are recorded in
`output/simulator-parity/android/qa-result.json` and
`output/simulator-parity/ios/media-plan-facts.json`. Android still needs native
lookup/theme/font/promotion fixes. iOS Channels layout freezing is repaired
with a bounded inline viewport and full-screen workspace view; final lookup,
cold restore and theme checks remain open. Neither platform's Media Plan
evidence closes reporting or troubleshooting acceptance.

The web bridge now requests full JSON window forms so authored report
definitions are not persisted as depth-limited summaries. Snapshot tests cover
deep chart fields, repeated column objects, long arrays/text and cycle removal;
compact callers keep their existing limits. Forge snapshot/restore and 54
application bridge/window tests pass, and the web build succeeds.

A fresh **Build order performance report** starter selected order 2659534 through
the real picker. Conversation `808516b4-72b0-450a-9fdd-a5765e455994` completed,
rendering a 1186×240 daily spend chart and four populated tables. Cold browser
reload retains the chart, table row counts (5/2/2/3 including headers), $309
spend, one user message and one native turn. Evidence:
`fullform-report-live-proof.txt`, `fullform-report-chart-table.png`,
`fullform-report-cold-proof.txt`, `fullform-report-cold-render-proof.txt`.
This verifies new report snapshot preservation. Older snapshots that already
contain truncation markers remain a separate recovery gate.
