# Conversation audit

Implemented for the native database runtime. This is a read-only web UI API;
it is separate from the MCP interface used by hosts.

One row represents one saved turn. The window provides filters, a turn table
and CSV export with full input and steering, the canonical final response,
current feedback and available usage measurements. Saved messages, model/tool
metadata, execution attempts, resumptions and payload bodies remain available
through the read-only diagnostic API; the window does not request or display
individual turn details.
The feature uses existing tables without changing recording, execution,
streaming, scheduling or feedback updates.

## Enable the experimental window

Set before starting the server:

```sh
export AGENTLY_EXPERIMENTAL_CONVERSATION_AUDIT_UI=true
```

The icon appears immediately below Scheduled tasks. The default is disabled,
including restored/direct openings. The API remains available independently.
Rebuild embedded UI assets after source changes with `cd ui && npm run build:embed`.

Users can inspect their own conversations. To give specific accounts access
to other owners' conversations, configure server user IDs:

```sh
export AGENTLY_CONVERSATION_AUDIT_USERS='["example-auditor-id"]'
```

An entry grants instance-wide conversation audit access. An empty or omitted
list grants no additional access. Identity comes from the existing authenticated
server context (or the existing anonymous browser cookie in local mode).
The request's `userId` is only an owner filter and cannot establish identity.
Use the `currentUserId` returned by `/capabilities` for the signed-in account:
this is the effective workspace identity used by conversation ownership, which
can differ from the canonical `users.id` stored for authentication.

The owner field is editable for every user. Editing it does not grant access:
the server rejects a different owner with HTTP 403 unless the caller is on the
auditor list. Leaving it empty still restricts an ordinary caller to their own
conversations. Only auditors see the explicit All users option. The same scope
checks apply to list, CSV, diagnostic details and payload reads. Workspace roles
do not automatically grant conversation audit access.

Multi-user deployments must use the existing server authentication. The local
anonymous browser cookie is a convenience identity, not verified authentication
and not a security boundary for shared deployments. The experimental UI flag
only controls window visibility; it never grants API privileges.

## Filters and API

Base: `/v1/api/conversation-audit`.

| Method and path | Result |
| --- | --- |
| `GET /capabilities` | UI flag, current user ID, ability to audit all owners |
| `GET /turns` | Bounded page of summaries |
| `GET /conversations/{conversationId}/turns/{turnId}` | One saved turn and canonical state |
| `GET /conversations/{conversationId}/turns/{turnId}/payloads/{payloadId}` | A saved body referenced by that turn |
| `POST /export` | Complete bounded CSV, one row per turn |

List query parameters and export JSON fields: `conversationId`, `userId`,
`from`, `to`. Times use RFC3339 offsets and are converted to UTC. Dates select
**conversation creation**, with `from` inclusive and `to` exclusive. Supply
an exact conversation ID or both dates spanning at most 31 days. An exact ID
takes precedence over dates. An owner filter alone is rejected. The UI defaults
to the current owner and the last seven days and waits for explicit search.

Lists also accept `limit` (default 50, maximum 200) and the returned opaque
`cursor`, tied to the selected filters and owner. Sort order is turn creation
time then ID, descending. Export rejects cursors and covers all applied filters.
Changing the form requires a new search before exporting.
The window follows server cursors with Next Page, Previous Page and First Page.
Rows per page offers 20, 50 (the default) and 100 turns. Changing it resets the
cursor history and loads the first page at the selected size. The current page
and end of results are shown above the table. With 48 matches at the default
size of 50, all results fit on one page and the navigation arrows are disabled.
There is no total-page count or direct Last Page operation; that control stays
disabled. Refresh and a new search reset to the first page. A failed page request
leaves the last successful results available.

The audit table starts with Turn time, Turn ID, Conversation ID and Conversation.
It then shows Input, Output, User, Agent, Status, Models, Input tokens, Output tokens,
Turn duration, Feedback, Feedback comment, Model calls and Tool calls. Conversation
created at is the last column. Input and Output contain full text. Rows stay
compact; the shared table's full-content button opens overflowing cell content
without an additional detail request.

`GET /v1/api/conversation-audit/turns` accepts optional `includeContent=true`
(or `false`, the default). This adds `input`, `output`, `models` (a sorted,
distinct array of actual `provider/model` values), `executionSpanMs` and any
warnings to each row. Input includes user steering within the same turn,
separated by blank lines. Output is the canonical saved final answer, excluding
interim answers. Models are comma-separated in both the UI and CSV.
Token totals include all recorded model calls in the turn, across resumptions.
Other turns and linked conversations are separate.

CSV exports exactly the table's 18 columns in the same order, across all matching
turns. Headers use snake_case field names; Turn time maps to `turn_created_at`,
Conversation to `conversation_title`, and Turn duration to `execution_span_ms`.
Dates use RFC3339 and duration uses milliseconds. Input and Output retain their
full text. Run IDs, warnings, costs, execution details and payload bodies are
excluded. Text is quoted for CSV and protected against spreadsheet formulas;
numeric feedback retains `-1`, `0`, or `1`.

The diagnostic turn endpoint retains saved execution details. Its `row.runs`
contains the saved run records, including their `id`, attempt and
`resumedFromRunId` when recorded. A turn can have several runs when execution
resumes. These records are available through the API without a UI inspector.

## Limits and interpretation

- Export: 1,000 turns, 32 MiB, 30 seconds, two concurrent exports per instance.
  The complete file is buffered before success headers. Exceeding a limit
  returns an error; no successful truncated file is sent.
- Diagnostic API details: at most 10,000 saved messages and 10,000 runs, with
  serialized projection size bounded to 32 MiB. Oversized details return an error.
- Enriched lists apply the same per-turn limits, with both the batch projection
  and complete response bounded to 32 MiB. Exceeding a limit returns an error,
  without silently shortening Input or Output. Narrow the filters or request
  a smaller page. The UI defaults to 50 turns per page.
- Diagnostic API payload bodies load only on demand, after checking conversation
  ownership and reference membership. Inline and decompressed bodies are limited to
  2 MiB. External URIs are not fetched. Binary bodies use base64.
- Unknown or incomplete measurements stay unknown. Execution span covers the
  first run start through the last run completion, including waits between
  resumptions; missing timing makes it unknown.
- A resumed turn remains one row. A separate retry turn remains a separate
  row with `retryOf`. Current feedback is read without modifying it.
- Results reflect saved data at read time. They are not immutable audit
  evidence or a transactionally frozen snapshot of an active conversation.
  Missing/expired payloads cannot be reconstructed.

The audit Datly projection caps candidate turns before calculating per-turn
summaries. Default list requests do not hydrate message bodies. Enriched lists
then recheck scope and load only the selected page's turn IDs in one batch,
excluding the pagination lookahead. Raw messages, runs and payload bodies stay
out of list responses. No indexes or schema migrations are introduced. Database
performance checks use synthetic SQLite fixtures; production MySQL load still
requires environment-specific measurement.

## Targeted checks

```sh
# agently-core
go test ./service/conversationaudit ./adapter/http/conversationaudit
go test ./sdk ./service/workspace -run 'TestConversationAudit|Test.*Feedback|Test.*Layout'

# agently
go test . -run '^TestConversationAudit'
cd ui
npm test -- src/components/ConversationAuditWindow.test.jsx --maxWorkers=1 --minWorkers=1
node scripts/test-conversation-audit-browser.mjs
```

The browser check uses synthetic API responses and the real host window,
Forge controls and workspace theme at 1440px and 390px. It uses installed Chrome
by default; `PLAYWRIGHT_CHANNEL` selects another installed channel.
Calendar vendor styles use CSS `@scope` inside this destination. Calendar
popovers stay inside it; other windows do not receive those styles.

Verification also covered the existing chat shell, navigation, typography,
scheduler and feedback tests. A live model conversation and production MySQL
were not exercised. Two baseline failures were reproduced independently of
this change and left untouched:

- `sdk/TestCanonicalModelInclusiveNativeTimelineFixture`: fixture does not
  include the current feedback fields returned by the canonical builder.
- `app/server/TestENGManagedInternalLayoutScopesDiscoveryAndPreservesRoleVisibility`:
  the existing menu-read case returns 200 where the test expects 503.
