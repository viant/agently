# Conversation workspace presentation

Reports and resource windows default to the conversation UI workspace. Distinct
objects append to that conversation's open set; repeated opens reuse logical
identity. An explicit view `presentation`, `region`, or `openMode` remains an
override. Use `openMode: replace` only when replacement is intended.

Inline tool feeds stay inline by default. Their optional **Open in workspace**
action promotes that one feed. It renders in one placement at a time; closing its
workspace restores the original feed placement. The historical marker remains.

## Tabs

The default is `auto`: one window/report has no object tab row, and multiple
objects have one tab each. Internal report sections are a separate navigation
level.

Configure the host with:

```js
connectorConfig: {
  workspace: { tabs: 'auto' } // auto | always | never
}
```

The web application's equivalent environment setting is `VITE_WORKSPACE_TABS`.
`always` shows an object tab even for one object. `never` hides the tab row and
uses a compact selector when multiple objects are open, so they remain reachable.

## Placements and layouts

- `inline`: content owned by an assistant turn.
- `workspace`: a durable, conversation-owned surface.
- `overlay`: a temporary drawer above the current surface.
- `rail`: the tool-feed side rail, distinct from the conversation workspace.
- `focus` and `split`: layouts of the workspace, independent of placement.

Legacy feed targets `workspace` and `detached` are accepted and normalize to
`rail` and `overlay`. New SDK emissions use canonical names. Development builds
log each legacy target once. An in-app overlay is not a separate OS window.

## Continuity and ownership

Core emits versioned workspace descriptors with an immutable origin and separate
latest-activation metadata. Renderer readiness and the matching assistant
confirmation gate automatic activation. Background restoration does not activate
new content. Explicit user marker actions can return to an existing object.

Canonical assistant messages carry structured workspace attachments. Legacy
transcript adapters remain available during migration. Closing an object retains
its historical marker; reopening performs a fresh metadata/permission preflight.
Local session state stores presentation choices and view state, never permission
grants. The production composer includes the active object, internal tabs,
filters, and selected keys as conversational context.

The shared shell is used in regular and developer modes. New objects open in
focus mode. Chat remains labeled; right-aligned red and yellow dots close the
selected object and toggle focus/split. Each dot has a 32px button target, an
accessible name, tooltip, and keyboard focus ring. Small screens keep focus mode
and disable the yellow control. Returning to an existing object preserves its
layout. The header shows the object title without a redundant Workspace eyebrow.

Within the workspace, Filter, Refresh, and Export use accent-colored icons with
tooltips and accessible names. Standalone toolbar labels remain unchanged.

## End-user progress and developer diagnostics

While work is running, end-user progress keeps ordinary tool counts and activity;
incomplete tool attempts add only a small muted warning icon, without failure
wording or red emphasis. If the overall request cannot complete, its final notice
uses neutral styling and plain recovery guidance. The regular status bar and terminal notice avoid raw
Error/failed labels and red diagnostic styling. The underlying execution state
and error payload are preserved. Developer mode retains explicit errors, raw
diagnostic messages, and red execution-detail styling.
