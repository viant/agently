# Turn feedback

The conversation owner can rate a completed final answer using the combined
thumbs action, which opens Good response / Bad response. After saving, it shows
the selected thumb with an accent background; clicking it clears both the rating
and comment.
The Feedback options ellipsis menu also offers the ratings, preserving the saved
comment when switching, and Add comment / Edit comment after a rating exists.
Saved comments have no inline preview.

A positive rating saves immediately without opening a form. A negative rating
saves immediately and opens the comment dialog only after acknowledgment of that
user action. Reading previously saved feedback never opens the dialog. Either
rating can receive a comment manually through Feedback options.
The small dialog confirms that the rating is already saved and has one optional
four-row textarea, a Unicode character counter, Save comment and a close action.
The dismiss button reads Not now for a new comment and Cancel when editing an
existing comment. It has no reason categories or checkboxes.
Save comment is enabled only when the text differs from the saved comment;
whitespace-only text is equivalent to no comment. Editing retains the complete
original text, including line breaks. Not now, Cancel,
Escape, the close action and the backdrop discard only the unsaved draft; the
saved rating and comment remain. Saving empty text clears just the comment.
Controls are disabled during a write. Failed writes retain the saved vote and
comment draft and show a retry message.
After saving a comment, the existing footer status announces `Comment saved` for
three seconds, or until the next feedback action. This does not move the controls.

`TurnFeedback` composes Forge Button/Icon and Blueprint Popover/Menu/Dialog/TextArea.
The toolbar and menu use the shared Lucide thumbs and ellipsis glyphs. Its scoped
styles live in chat/turnFeedback.css.
It uses the shell's semantic typography, surface, border, accent and selected
roles and the owning Forge boundary's themed portals. The selected thumb exposes
`aria-pressed`; menus support arrows/Home/End and Escape. The comment has a label
and character limit, and pending/error states use status/alert roles.
The initiating control regains focus after a write unless focus moved outside
the feedback controls. The dialog focuses its textarea after opening, contains
keyboard focus, and returns it to the initiating footer action after closing.
This change is scoped to the final-answer footer; existing bubble and execution-card typography remains
legacy scope described in workspace-typography.md.
The shared Forge compact-button rule also retains its 44px minimum target on
coarse pointers; desktop compact dimensions remain unchanged.
The first glyph aligns with the answer text in all three rating states, using a
stable 28px icon slot and a local offset that preserves the shared hit area.
Both combined thumbs use 14px Lucide glyphs with a 2px overlap; a selected thumb
remains 16px.
Document answers with feedback have a 12px gap before the toolbar; other messages
retain their existing spacing. The form has one focus ring, a vertically resizable
textarea with a 96px minimum height, and right-aligned dismiss / Save comment actions.
At 390px the action row stays on one line and icons keep their shared target
sizes. The dialog is at most 560px wide with 16px viewport margins. The
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
The existing `npm run test:typography` browser proof exercises the actual
TurnFeedback and SDK with synthetic local responses in both light/dark modes
and desktop/tablet/touch sizes, including icon alignment across rating states,
modal focus, transient save confirmation and comment preservation.

Local development can use sibling agently-core and Forge checkouts through
`go.mod` replacements. UI builds and Vitest also resolve those sibling sources
by default; `AGENTLY_CORE_SOURCE_ROOT` and `FORGE_SOURCE_ROOT` can override them.
UI Vitest also needs `APPSERVER_URL` set, as required by the existing Vite config
(for example `APPSERVER_URL=http://127.0.0.1:9193`).
The design-system workflow must pin a published Forge commit containing
`lucide:thumbs-up`, `lucide:thumbs-down` and `lucide:ellipsis` using its full SHA.
