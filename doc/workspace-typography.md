# Workspace design-system foundation

Agently and Forge are shared open-source owners. Runtime behavior, semantic APIs,
neutral defaults and examples stay generic. Workspace-owned configuration supplies
brand colors, fonts, reviewed values, business content and variant selection.

The local proof now also renders Forge ChartSpecimen: all existing families,
seven data states, metric/two-axis choices, shared selectors and keyboard point
selection. An external fixture can supply authored report charts and reference
assertions. The harness uses their real builder-to-chart translation and records
sample selection locally. No report query or business mutation is executed.
The host loader test also verifies the document/small-multiples metadata survives
serialization. These local changes require an approved new Forge frontend and Go
revision before submission; current remote pins predate them.
See Forge's canonical doc/typography.md for its full foundation/component map.

The shared optional v1 contract is defined by Agently Core's
`protocol/ui/theme`. Agently validates the same role names, families and numeric
bounds. Forge owns the reusable Typography implementation and canonical neutral
fallbacks. Workspace assets own the family and reviewed values. Settings uses
`typographyProps` from `forge/components` on the actual heading, prose, label,
select and metadata elements when the active profile supplies a role weight.
Legacy profiles retain the original settings declarations. No global shell
migration or vendor-font dependency is introduced.

`npm run test:typography` retains the original family/inheritance proof and adds
`typography-components-proof.html`. That proof uses the real style manager,
WorkspaceStyleProvider, WorkspaceAppearanceSettings, Forge theme boundary,
Foundation and Typography specimens, WidgetRenderer, and Chart. By default it reads the shared
Core fixture. Optional `AGENTLY_TYPOGRAPHY_WORKSPACE_PROOF` points at bytes exported
by Core's actual workspace style service test. Optional
`AGENTLY_TYPOGRAPHY_PILOT_JSON` supplies an external workspace's container metadata,
sample values, theme, font expectation and element/action checks. Shared proof
code contains no customer-specific names or palette. It uses the real widgets,
form bindings and Toolbar. Handlers record local test events only.

The proof verifies role propagation across application/renderer boundaries at
1440px and 390px, loaded workspace family, authored identity roles, and page
containment. Screenshots are optional via `AGENTLY_TYPOGRAPHY_SCREENSHOTS` and
must stay out of product source submissions when they contain private material.
The style manager keeps its existing immutable-asset and cascade behavior.

Swift and Kotlin validators now accept and validate optional role/foundation
fields and workspace-primary/monospace names. Native rendering keeps its existing
system fallback. This is catalog compatibility, not native UI migration.

## UX1 adoption inventory

Source scan on October 22, 2026. Counts are declarations without a variable or
inherit/normal value, not a defect count; documented legacy defaults are valid.

| Stylesheet | Size | Weight | Leading | Tracking |
| --- | ---: | ---: | ---: | ---: |
| shell.css | 137 | 79 | 51 | 26 |
| rich-content.css | 14 | 7 | 6 | 4 |
| ui-settings.css | 6 | 4 | 3 | 0 |
| execution-workspace.css | 10 | 5 | 1 | 2 |

UX1 maps brand and major section headings to section; ordinary history rows,
search, composer, Send and prose to body; structural controls to label; timestamps,
status, pagination and supporting scope to metadata; compact copy to small;
technical examples to code. Chat title appearance uses heading only where it is
a page title. Markdown h1/h2 semantics remain independently correct. Selected
states should use the existing surface/color/border roles rather than adding a
new weight scale. Inline component styles and rich-content payloads also need
review in the touched scope. Do not remove unrelated legacy declarations as a
bulk cleanup or call these counts resolved by passing a font test.

The design-system workflow runs these checks. Making its status merge-required
still needs a repository administrator before the migration gate can close.


## Contributing and extending

The versioned root README and AGENTS.md point to this guide. Compose supported
components before adding a new pattern. A shared extension includes its API,
variants, keyboard/focus and data states, specimen, product consumer, migration
limit and existing contract/component checks in the same change. PR evidence
identifies those items for the existing shared-component owner's review. Never
silently enlarge a legacy allowance or move canonical values into a page.

The repository now supplies versioned entry points for Codex, Claude Code and
Cursor, linked below. The design-system workflow runs the owning contract,
component and browser checks. GitHub policy verification found no merge-required
shared checks. An administrator must require `Design system contracts` before
team-wide merge enforcement can be claimed.

## AI contributor entry points

The team uses Codex, Claude Code and Cursor. Root `AGENTS.md` links this guide;
`CLAUDE.md` imports that entry point, and `.cursor/rules/design-system.mdc` is an
always-applied thin pointer to this same guide. These files are versioned so a
fresh checkout has the usage path. Instructions guide contributors; the contract
and component checks reject defined violations. Neither replaces design review.

Mechanisms verified against [Claude Code project memory](https://code.claude.com/docs/en/memory)
and [Cursor project rules](https://cursor.com/docs/rules). The Cursor rule uses
`.mdc` frontmatter and `alwaysApply: true`; no user/team settings are changed.

The browser proof uses Playwright's matching Chromium by default. Install it with
`npx playwright install chromium` before `npm run test:typography` in ui/.
`PLAYWRIGHT_CHANNEL` can explicitly select another installed channel.


## Local appearance-select correction

WorkspaceAppearanceSettings now uses Forge's shared SelectControl for both
native dropdowns. The same indicator frame is used in the shared specimen and
document-table editors, replacing the settings-only browser arrow. The real
native select retains its accessible label, binding, mode availability, required
semantics where authored and keyboard behavior. Type roles remain optional for
legacy profiles. The browser proof checks 32px reserved text space, a 16px SVG
inset 10px, native selection and disabled color mode at desktop and touch sizes.
This source correction remains local pending review. Before submitting the host
follow-up, publish the approved Forge change and pin CI's frontend checkout to
its exact commit. The current remote pin predates this local shared export.


## Section navigation pattern

A workspace opts into the shared app rail and section menus with
`left.navigation.presentation: sections` in its existing layout. Omission retains
the legacy split sidebar. The layout still supplies permission-filtered
applications, menu actions and topbar actions. Topbar actions become rail
destinations in sections mode; their existing handlers and parameters are reused.

`SectionNavigation` is the shared structure. `WorkspaceSidebar` supplies live
menus and `Sidebar`'s `appearance="document"` variant supplies title-only history.
Use body for history/menu entries, label for groups and metadata for the history
heading. The canonical `sectionNavigation.css` owns geometry, shared semantic
appearance and the bounded phone-input size. Workspace branding stays in its
existing theme. Do not reproduce this pattern with application CSS overrides.

`Root` hosts app destinations in `app.section`, independently of conversation
outputs in `chat.top`. Account controls retain their existing MenuBar handlers
when portaled into the rail. A modal phone drawer focuses its selected section,
contains Tab navigation, closes on Escape and restores its trigger. Search and
account disclosures handle their own Escape before the drawer. Disabled history
is not mounted, and selecting a collapsed rail section expands its menu.

The real component is shown in `NavigationSpecimen` in the existing typography
component proof. `navigation-components-proof.html` accepts the same external
workspace theme and actual layout endpoint, with sample history supplied by its
local runner. Its destination content is explicitly a placeholder. A host's
authenticated window/draft/URL integration still needs its normal integration
checks before release. The section pattern supplies navigation; the document
message, composer and starter integration is described below. The complete
saved-output lifecycle remains unverified. Header/viewer controls are described
below.

Existing design-system CI runs the navigation/source-role checks and affected
shell suites. The typography browser proof exercises the specimen at desktop and
phone widths. Making checks merge-required remains an administrator dependency.


## Document shell header and viewer

`MenuBar` selects the shared document header only with the existing section
navigation activation. `ShellLocation` supplies the single label-role chat
heading, Back/Forward controls and the phone menu trigger. `ShellIconButton`
uses Forge Icon and the label role with a visible keyboard focus state.
Pointer focus explicitly clears Blueprint’s global outline; keyboard focus
retains the workspace focus tokens. Equal viewer targets and the approved gap
are checked alongside both input modes in the existing browser proof. The recipe owns
54px desktop and 56px phone headers, 18px shell glyphs and the 760px phone
drawer threshold. Coarse-pointer controls retain 44px targets; vertical phone
padding adapts to keep the header at 56px. The legacy shell keeps its 820px
threshold and existing controls. No workspace fonts or palette are in this code.

The brand portal sits in the adjacent sidebar masthead; its text uses section.
Search keeps its real handler and focus-return ref when portaled beside it.
The profile remains in the rail. Desktop navigation is visible by default;
its retained collapse/expand action is in the account menu. Existing approvals,
usage, backend selection, settings and logout callbacks remain available.

`ConversationWorkspaceSurface presentation="document"` portals the real Forge
runtime tabs and separate `panel-right` hide/show and maximize/minimize actions
into `MenuBar`'s header slot. The transcript and renderer mounts are retained.
At 1180px and wider the split transcript is 380px; narrower screens switch
between full surfaces. Hide does not call close. Close still uses the owning
host's capability checks and handler. Named tabs stay visible for one object,
unless the host explicitly selects `tabs: never`, which retains a native shared
SelectControl. Ancestry actions, working/unread status, lazy first mounting,
retained renderer state, lifecycle callbacks and scroll restoration remain.
Optional renderConversation/renderWorkspace slots allow hosts and specimens to
supply content without copying the shell or its state controls.

The document header orders named tabs first, then the saved-output inventory,
then viewer actions. A selected history title uses interaction text over the
selected surface. Pointer focus clears the inherited outline; keyboard focus
keeps the neutral visible focus recipe. These states belong to the shared
pattern, while the active workspace supplies their values.

`useShellNavigation` owns native browser entries for section and main-chat
visits only while enabled. It preserves other query parameters and restores
known entries without adding a visit during Back/Forward. Its buttons cannot
leave the known current-session trail. Existing conversation selection and
window ownership stay authoritative. Legacy hosts retain replaceState.
Heading lookup requests header metadata without a transcript, listens for title
updates and discards late results. Destination-level URLs, reload recovery of
app-section windows and authenticated end-to-end history still need coverage.

The existing NavigationSpecimen now includes the actual header and workspace
surface. The local navigation proof uses those same components, the external
workspace layout/theme/brand name and sample conversation/output content. It
checks retention locally, not live business saving or output persistence.
Existing CI includes the affected surface, conversation and navigation-hook
suites. The existing typography browser proof checks header geometry, glyphs,
roles, one-object tabs and hide/show at desktop/phone sizes. The source-role
guard covers both new stylesheets and rejects raw colors, direct type scales
and important overrides. This slice does not complete UX1 or UX2.

## Document chat integration

Native new/conversation windows select the optional Forge `chat.appearance:
document` recipe. The actual App/Root, host NamedLookupInput, chat service and
Forge Chat/Composer remain the implementation. The existing component specimen
now includes their real message/composer/task components. It is a verification
surface, not a replacement application or a second theme.

Document BubbleMessage uses body for both compact right-aligned user messages
and unboxed assistant prose. Rich content inherits all five role properties;
code and authored headings retain their own semantic treatment. Avatar removal
is limited to this presentation. Native lookup and skill suggestions, uploads,
camera, microphone, stop/queue and host selector callbacks remain available.
The Tools disclosure uses shared SelectControl, not styled native arrows.
Phone headers retain visible Back and Forward controls with their actual
enabled/disabled state, alongside the menu trigger, location label and existing
shell utilities. Coarse-pointer history actions retain 44px targets. Native
browser Back remains available as well.

The header's SavedOutputInventory consumes structured workspace objects from
server-projected attachments, live windows and in-memory history. It deduplicates
object identity by highest revision, rejects foreign-chat and non-reopenable
objects and does not invent outputs from ordinary text. Reply links stay with
the original owning reply across revisions. Close retains the inventory entry;
Hide retains mounted renderer state. Reopen uses the existing fresh metadata
and authorization path. Browser storage retains bounded window IDs and surface
preferences only, never a trusted descriptor or report payload. On reload,
fresh server descriptors are required. The sidebar reads the window-level form
signal directly, so opening an output without a datasource does not throw.

Reading position and following state are retained per chat. Wheel/keyboard/touch
navigation toward earlier messages releases automatic following; background completion cannot force a
reader to the end. Sending re-enables following. Focused drafting, narrow
screens and an explicitly hidden workspace prevent automatic surface promotion.

Home, prepared-task and conversation drafts use separate keys in the existing
session draft store. Task Back and refresh preserve exact edits, including an
intentionally empty draft. Explicit New Chat keeps chatRuntime's existing
blank-draft reset and clears the prepared-task URL parameters; it is distinct
from restoring a chat or returning from a selected task.

The existing typography browser runner checks actual role propagation, input,
keyboard focus and Tools at desktop and touch sizes. Existing host suites cover
inventory, attachment origin, preferences, reading and the datasource-free window.
The existing native metadata-loader test checks document appearance survives
host loading. Local full-app fixture review adds Home/tasks, draft/reload and
open/hide/close/reopen checks. Keep fixture coverage separate from authenticated
existing-chat/report reads: neither establishes live saving, access recovery or
one complete report lifecycle. Destination restoration, authenticated narrow
coverage and that lifecycle remain required before report-family migration.
Legacy inline Tool Feed presentation and report-body split overflow remain
separate UX2/report migration work; corrected shell controls do not approve
whole-surface parity.

### Home metadata, slots and role verification

Core's optional `StarterTask.resultLabel` survives the host task normalization
and is passed with the available tasks to Forge. Its document browser displays
the short caption and searches it along with title, description and category.
Core also publishes optional `ui.composer.placeholder` as workspace metadata
`composer.placeholder`. `chatService` forwards a nonempty value in the existing
`NamedLookupInput` input props, through Forge Composer; omission keeps the
existing input hint. Neither field executes a task, changes permissions or
promises output persistence.

The host supplies `renderHomeRecent` to native Forge Chat. The shared
HomeRecentConversations component reads up to three actual conversations through
the existing client, normalizes the page, resolves titles/timestamps and reuses
`openConversationInMainWindow`. It handles loading, empty and retry states,
refreshes on conversation/metadata events and discards stale results. Its
section heading uses section, conversation links use body and supporting labels
and dates use small. The Forge-owned library transition hides this slot with
the Home question and composer. Task selection prepares an editable draft;
returning preserves the mounted browser's filters and that draft.

The quiet document sidebar uses body for New Chat, search and history text,
metadata for Conversations, and section for the masthead brand. Shared navigation
CSS owns the masthead/history insets, title measure and vertical rhythm, while
the workspace may select scoped role values for brand and history heading.
No customer palette, vendor typeface or private role value belongs in this
repository. History search, refresh, selection and removal remain functional.

ChatSpecimen includes the real native Forge Chat Home as well as the existing
message/composer/task examples. The existing desktop/touch typography runner
checks the native Home heading-to-composer gap and composer-before-task order
with a short list. It checks every category control's small role and perturbs
all five small-role properties on category controls and task descriptions;
real message prose retains its five-property body-role check. Public specimens
use generic content; workspace fixtures supply private branding and measured
reference values. Actual-application comparisons add Home/library/recent,
draft/history and shell-control evidence without replacing these shared checks.

## Canonical report output handoff

In document presentation, `canonicalReportSources` reads admitted assistant and
iteration content, including Core's `assistantFinal`. `workspaceAdapters` maps
supported committed report assemblies into the existing workspace-object
contract. Conversation, turn (message fallback), scope and report ID form the
identity. Same-turn revisions and reset generations update that object; different
turns stay separate unless a backend descriptor supplies shared identity.
Ordinary answers and foreign, incomplete, unsupported or diagnostic reports do
not create empty output tabs.

`InlineReportWorkspace` resolves the current report from the admitted chat store
and mounts the existing report reader. Keep sources and data out of navigation
preferences: `workspaceSession` persists bounded open/closed IDs, selection and
surface mode only. The originating `WorkspaceAttachmentCard` uses label for the
report name, metadata for its version/state and shared Lucide icons. The compact
document link, neutral keyboard focus and viewer controls belong here; workspace
assets supply their reviewed values.

Hide retains the view; close retains its reply link and inventory entry. Reopen
and reload resolve fresh admitted descriptors and honor explicit reopen limits.
Unavailable sources and renderer failures produce recoverable states; retry
remounts the reader through its lifecycle attempt. Already promoted Tool Feeds
retain workspace ownership after close in document mode. Untouched authored-inline
feeds keep their existing placement. Phone viewing moves approvals, usage and
backend controls into the existing account menu, preserving their handlers.

`OutputHandoffSpecimen` in the existing component proof mounts the actual
`ChatFeedFromChatStore`, `ConversationWorkspaceSurface` and report reader. Existing
component/service and desktop/touch browser checks cover handoff states, role
propagation and retained Home behavior. Actual-app fixtures cover two reports,
exact drafts, close/reopen and reload; signed-in desktop read-only evidence
covers returning to an existing report after reload and chat switching. This
extends the earlier unverified-lifecycle notes only for these paths. Report-body
migration, authenticated access-loss/recovery, cross-device retention and other
report families remain open. Phone evidence uses browser touch/viewport
emulation. Local review still precedes submission and dependency publication.

## Output recovery

`WorkspaceOutputState` is the shared in-view loading and recovery composition.
It uses Forge section/body typography and secondary/ghost buttons, with status
semantics while loading and an alert for denied, unavailable, network or renderer
failures. Try again repeats the owning read; Return to conversation uses the
surface's existing handler and retains its draft, output link and inventory.
The host stylesheet supplies spacing and semantic text colors. Do not add a
parallel type scale, private brand values or a separate error theme.

`InlineReportWorkspace` calls `readWorkspaceReport` on reader mount and retry.
That service probes the existing identity, reads the owning transcript through
`readConversationHistory`, and reuses Core's canonical snapshot reducer/projector
to validate conversation, turn, scope and report identity. Denied, missing or
foreign fresh sources cannot fall back to cached report content. Later admitted
stream revisions remain available after validation. The existing opaque account
scope fence rejects stale responses; the SDK's unauthorized handler still owns
sign-in. These client checks do not grant backend access. A closed loading reader
ignores its late result. Retained hidden readers do not poll for permission
changes; fresh validation happens on mount or retry.

Hosted `ToolFeedDetail` uses the same recovery actions and opening/failed lifecycle.
A null or empty result shows recovery rather than a blank viewer. Denied cache
and wired signals are discarded, and late read or metadata responses from a
different account scope are ignored. Hosted report materialization and render
failures conceal incomplete bodies and exports behind recovery. Authored inline
feeds and existing report-body presentation retain their established behavior.

The maintained `OutputHandoffSpecimen` exercises the actual reader with a neutral
canonical fixture and neutral identity response. Existing Root, component,
source-role and shared typography browser checks cover the extension; no workflow
or backend schema was added. Actual-app local responses verify denied, missing,
network, retry, return, delayed-close, two-report and reload paths at desktop and
emulated phone widths. A signed-in existing report verifies a fresh successful
read, close and reopen. Production permission revoke/regrant and real account
switching still need separate integration evidence. Existing global notifications,
other workspace flows, cross-device retention and report-family migration remain
outside this recovery extension. It does not complete UX1 or UX2.

## Workspace destination routes

In sections mode, `useShellNavigation` carries the selected catalog destination
in native URL/history entries. Workspace identities are application and nested
menu IDs, not window keys, descriptors or input values. `WorkspaceSidebar`
resolves each selection or restoration against the current permission-filtered
layout and reuses its existing window action, provider identity and parameter
bindings. Action sections use their current catalog action. URL actions keep
their isolated-tab handler and are not executed by route restoration.

Missing or disabled destinations show an unavailable state with navigation
retained. Required context comes from current inputs; an unresolved binding asks
for selection rather than persisting private context in the URL. `Root` matches
the section view to both section and destination, so a stale view cannot supply
an unrelated cached window. Overview clears the destination and retains the
existing landing composition. Conversation path synchronization ignores an
unchanged path and preserves independent `app.section` windows during delayed
chat startup. Returning to Chat selects its native window and retains its draft.

Use `SectionDestinationButton` for destination rows. It applies the complete body
role and `aria-current="page"`; shared navigation CSS supplies semantic selection,
neutral visible keyboard focus and no extra pointer outline. `NavigationSpecimen`
uses the actual control for ordinary, selected and disabled states. Workspace
branding, fonts and role values remain workspace-owned.

Existing CI suites and the shared browser runner cover route restoration and
the control's five-property body-role and focus/selection states. Actual-app
fixtures cover desktop and emulated phone navigation. This extends the earlier
destination-restoration coverage gap only for these paths. Signed-in metadata,
Back and reload are verified separately; a blocked read-only-preview datasource
request leaves live rows and business actions unverified. Existing destination
bodies, icons and Overview are preserved. Other workflows, report families,
cross-device and real permission/account changes remain open. Local review,
dependency publication and merge-required check policy remain release gates.

## Reading document integration

The Tool Feed host forwards an authored root `ui.appearance` and marks its outer
wrapper with the resulting `data-forge-appearance`. Forge owns the document's
geometry and role consumers. A workspace can narrow its legacy compatibility
selectors around that marker, without the host containing product-specific CSS.
Explicit `hideLabel: false` is retained when forwarding workspace actions.
The settings pilot now renders its externally supplied authored Container,
including nested regions, rather than rebuilding identity fields in a bespoke
proof layout. Its neutral local datasource is declared in the pilot context.
Workspace proof fixtures continue to supply labels, values, theme and assertions.

### Analytical report hosting

A canonical report selecting `viewer.appearance: document` is hosted inside the
existing `ForgeThemeBoundary`. Forward the compiled document/spec to
`ReportRuntime` and the existing export action through `reportActions`. Forge
owns document/header/panel/table/date-control geometry and typography. The host
retains lifecycle, validation, data materialization, filtering and export
callbacks. Selected document actions use the public Forge button/icon roles;
legacy reports keep their existing path. Do not add a branded report stylesheet
or bypass the workspace boundary. The hosted report render regression covers
selected document and legacy consumers; the shared specimen/browser check covers
actual controls and input modality.
The existing required `npm run test:typography` runner includes the real hosted
report render regression before its browser proof. No CI workflow change is
needed. Shared dependency references still need their eventual approved commits
when this local package is prepared for PR submission.

### Local report review choices

Canonical checkbox review tables and legacy planner tables use Forge `ReviewTable`.
The canonical host keys compilation to report content and authored sequence so
ordinary history refreshes cannot erase a selection. A new source revision resets
local choices. The same compiled fill/print snapshot supplies PDF export. No new
save callback is introduced. Legacy Submit/Preview payload creation, enabled
rules and CSV callback remain in the host. The existing required typography test
includes a mounted selection/export/reset test; RichContent SSR uses jsdom to
share the linked UI dependencies' React runtime.

## Document request lifecycle

Section-navigation workspaces place the actual TurnProgressStatus beside the
active iteration in ChatFeedFromChatStore. Legacy shells keep their existing
top-level status. Select one running iteration ahead of queued pending work;
queued requests must not duplicate the status or redirect Stop. Connection-only
bootstrap status remains available before an iteration exists.

RunProgressView consumes label/body/metadata roles and shared icons/buttons.
Elapsed time has no live announcement; phase updates use polite status. Public
task narration remains a separate admitted channel from private reasoning and
tool bodies. Inline document status sets publicNarrationInFeed so the original
assistant transcript and attachments own that text once. Standalone status keeps
public narration; operational fallback remains when no public task exists.
Normal document progress omits tool/token telemetry; developer
execution details remain available through their existing path. Stop keeps the
actual scoped cancelTurn handler, waits for authoritative lifecycle, and exposes
a recoverable local failure. A new turn clears the previous stopping state.
RunNotice uses the same body/label grammar for failed/canceled requests. Review
and retry preserves the existing composer-prefill action; it never submits.

RequestReviewDialog composes Blueprint's existing dialog lifecycle, the Forge
theme boundary and shared action controls. MenuBar queue decisions and the
ElicitationOverlay retain their original target/payload/close/permission paths.
Document review selects SchemaBasedForm field-tracks and the existing ReviewTable.
Optional planner column flex/width/minWidth metadata reaches the shared layout.
Resolved or submitting tables disable native selection; review dialogs can omit
the CSV action. Decision failures retain edited fields and choices and explain
that an unconfirmed outcome must be checked before retrying. Legacy appearance
is retained when document selection is absent.

RequestLifecycleSpecimen demonstrates the actual neutral progress, notice,
dialog, editable and locked review table. Existing required Root/typography tests
and browser proof cover the scoped handlers, field forwarding, roles and states.
No CI workflow or private palette/font dependency is added. Actual-App local
examples use synthetic transcript/API responses, not live business execution.
Live stream reconnect/cancellation receipts, queue/steering visual integration,
real approval/provider schemas and permissions remain separate verification.

## Authored activation and existing reports

The sections shell marks its application root as document presentation. Unmigrated
app destination bodies explicitly restore the selected theme with `legacy`.
Core's optional `presentations.document` mapping supplies a workspace-owned profile
without changing global theme or stored preferences. Native document chat,
authored document Tool Feeds, report viewers and review content mark their owned
Forge boundaries; content and owned dropdown portals use the same role values.

In the opted-in document workspace viewer, a saved canonical report without an
explicit appearance receives `viewer.appearance: document` in a shallow render
copy. Its title, blocks, data, query references, identity and permission checks
remain unchanged. Nothing is persisted or backfilled. Explicit appearances and
legacy shell viewers remain authoritative. Fresh-read, account fencing and report
version selection still precede rendering. The existing hosted-report suite
checks actual rendering, untouched saved content and retained viewer options.

## Observable progress in document chat

The progress indicator animates from the existing motion token and respects
reduced motion. Label, metadata and body roles remain the only text styles.
Elapsed time stays outside live announcements. Completed/running/queued step
counts come from identified scoped tool events, not an estimated percentage or a
claim about the total work remaining. Runtime tool names stay in developer details;
the public chat and footer use plain activity labels.

RunProgressView observes a stable active-turn progress key. Thirty seconds without
a changed activity, public update, model step or tool state displays "Waiting for
an update" and the observed quiet time. A timer tick, unrelated history render or
elapsed time is not new progress. A real new update restores the current activity;
input-required and stopping states remain distinct. This is an observation of the
client's received progress, not a diagnosis of a failed connection or a backend
timeout. Stop still waits for authoritative cancellation confirmation.

RequestLifecycleSpecimen uses the actual component under a document boundary;
its local "Complete example step" action demonstrates resuming after a quiet
period. Existing Root/progress contracts and the existing typography browser
runner cover quiet-state recovery, normal cancellation, role/bypass guards,
desktop/narrow layout and reduced motion. No new workflow or backend API is needed.
