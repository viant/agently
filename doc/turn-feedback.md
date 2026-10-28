# Turn feedback

The conversation owner can rate a completed final answer with Good response or
Bad response. Clicking a selected thumb clears both the rating and comment.
Changing thumbs preserves the saved comment. After rating, an icon-only comment
action appears. A saved comment replaces that action with a bubble icon,
single-line preview and chevron in the same row as the thumbs. The preview is
at most 560px wide and uses a CSS ellipsis when space runs out; whitespace is
collapsed only for display. Clicking it opens the inline editor directly with
the complete original comment, including line breaks. The preview is hidden
while editing; Save comment replaces the comment and Cancel discards the draft.
Both return to the compact row. An empty saved comment restores the icon-only
action. Add/Edit comment remains the accessible name and hover hint.
Controls are disabled during a write. Failed writes retain the saved vote and
comment draft and show a retry message.

`TurnFeedback` composes Forge Button, Blueprint TextArea and the existing Phosphor icons.
It uses the shell's semantic typography, surface, border, accent and selected
roles. Thumb buttons expose `aria-pressed`, the comment has a label and character
limit, and pending/error states use status/alert roles.
The initiating control regains focus after a write unless focus moved outside
the feedback form. Closing the comment editor returns focus to its action.
This change is scoped to the final-answer footer; existing bubble and execution-card typography remains
legacy scope described in workspace-typography.md.
At 390px the action row stays on one line; icons keep their shared target sizes
while the preview shrinks. The footer and editor fit within the viewport. The
existing topbar/user button still extends about 5px past the viewport even when
feedback is hidden; that shell layout is outside this change.

The canonical turn carries `feedback: {rating, comment, updatedAt}` and the
server-computed `feedbackEditable` capability. Shared/public readers cannot
write. Running, failed, canceled, queued and host-request turns are ineligible,
as are turns without a final answer. A final answer is rated once per turn, even
when the feed also renders an execution card or a standalone final message.

PUT `/v1/conversations/{id}/turns/{turnId}/feedback` replaces feedback with
`{"rating":1,"comment":null}` (rating must be -1 or 1). DELETE at the same path
idempotently clears it. Both return the full feedback object; the cleared value
is `{"rating":0,"comment":null,"updatedAt":null}`. Comments may contain up to
4000 Unicode characters; whitespace-only comments become null. Timestamps use
UTC. Older transcript requests and stream snapshots cannot overwrite a locally
acknowledged write; a read started after that write is authoritative again.

Core's MySQL upgrade 43→44 adds the three columns, without CHECK constraints or
new SQL indexes. SQLite startup also adds missing columns to existing stores.
Core's generated Datly readers/writer must be regenerated when the schema changes.
The authored turn lifecycle also initializes a new turn's rating to zero before
Datly validates it; sparse updates to existing turns preserve their feedback.

Targeted checks are Core's `TestHTTPTurnFeedbackNative` (SQLite and optional
`AGENTLY_TEST_MYSQL_DSN`), `TestService_Ensure_UpgradesLegacyTurnSchema`, TypeScript
`turnFeedback.test.ts`, and UI `TurnFeedback.test.jsx`, `chatStore.test.js` and
`ChatFeedFromChatStore.test.js`. MySQL schema checks are in
`tools/schema/test_mysql_protocol_reuse.py`.
Rebuild the embedded UI using `cd ui && npm run build:embed`, then rebuild the Go
binary, or use `cd e2e && endly -t=build`. Browser validation should cover both
thumbs, comment editing/canceling, reset, reload, independent turns, request
failure/retry and keyboard use at desktop and narrow widths.

Local verification uses a temporary Go workspace containing Agently and
agently-core. The released Core version in Agently's go.mod must be updated when
the Core changes are published. The current UI also requires Forge's shared
design-system exports (available at `origin/codex/ux0-design-system-foundation`,
revision `47b990c`). If the local Forge checkout lacks those exports, point
`FORGE_SOURCE_ROOT` at a checkout of that revision when building or testing UI.
UI Vitest also needs `APPSERVER_URL` set, as required by the existing Vite config
(for example `APPSERVER_URL=http://127.0.0.1:9193`).
