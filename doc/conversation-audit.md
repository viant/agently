# Conversation audit

Implemented for the native database runtime. This is a read-only web UI API;
it is separate from the MCP interface used by hosts.

One row represents one saved turn. The window provides filters, a turn table
and CSV/JSON export with full input and steering, the canonical final response,
current feedback and available usage measurements. Saved messages, model/tool
metadata, execution attempts, resumptions and payload bodies remain available
through the read-only diagnostic API; the window does not request or display
individual turn details.
The feature uses existing tables without changing recording, execution,
streaming, scheduling or feedback updates.

## Enable the experimental window

Configure the workspace's `config.yaml` before starting the server:

```yaml
conversationAudit:
  ui:
    enabled: true
  permissions:
    conversation_audit:
      users: ["*"]
    conversation_audit_admin:
      users: ["example-admin-id"]
```

`conversation_audit` grants access to the caller's own conversations. Its users
list accepts exact effective server user IDs or `"*"` for all authenticated users.
`conversation_audit_admin` grants access to all conversation owners and implies
the ordinary grant. It accepts explicit IDs only; an admin wildcard is a startup
configuration error. Admin wins when a user appears in both lists. Missing
configuration, empty lists or an unmatched user deny access. Unknown fields and
invalid value types are rejected at startup. Configuration changes require a restart.

The icon appears immediately below Scheduled tasks only when `ui.enabled` is
true and the signed-in user has either grant. Direct/restored windows enforce
the same rule. The UI is disabled by default. Authorized API access is independent
of this UI setting. The former `AGENTLY_EXPERIMENTAL_CONVERSATION_AUDIT_UI` and
`AGENTLY_CONVERSATION_AUDIT_USERS` variables are no longer read.
Rebuild embedded UI assets after source changes with `cd ui && npm run build:embed`,
then rebuild the Go binary.

Identity comes exclusively from the existing authenticated server context.
Unsigned anonymous browser cookies and request parameters cannot authenticate
audit callers, including in local mode. Audit requires existing server
authentication; other application routes keep their current behavior. Anonymous
requests receive HTTP 401. Authenticated users without a grant receive HTTP 403
on data routes; capabilities return `canAudit: false` without exposing grant lists.
Use `currentUserId` from `/capabilities` as the effective identity used for
conversation ownership; it can differ from the canonical authentication `users.id`.

Ordinary users see a locked User ID field and can only read their own conversations.
Omitting `userId` still restricts them to that identity; another owner or any
`userIdContains` filter is forbidden. Exact owner IDs are case-sensitive,
including on databases with case-insensitive collations. Admins have a single
Users selector: My conversations (the default), All users, Exact user ID, or
User ID contains. My conversations locks User ID to the effective server identity;
All users clears and disables the field. Exact and contains modes allow editing
and require nonblank text; clearing the field never selects all users implicitly.
The last edited owner text stays in memory while switching scopes, without
affecting My conversations or All users requests. Scope changes preserve the
conversation and date fields and wait for Search turns before loading data.
Fragments match all owner IDs containing the
literal text, ignoring case; `%`, `_` and `*` have no wildcard meaning. This
search uses saved owner IDs, not display names or a directory lookup. Admins
must explicitly select All users in the UI before searching without an owner.
The API allows an admin to omit both owner filters, still requiring a conversation
or bounded dates. All scope checks apply to list, totals, CSV/JSON, diagnostic details and
payload reads. Workspace roles do not automatically grant audit access.

## Filters and API

Base: `/v1/api/conversation-audit`.

| Method and path | Result |
| --- | --- |
| `GET /capabilities` | UI flag, current user ID, `canAudit`, ability to audit all owners |
| `GET /turns` | Bounded page of summaries |
| `GET /conversations/{conversationId}/turns/{turnId}` | One saved turn and canonical state |
| `GET /conversations/{conversationId}/turns/{turnId}/payloads/{payloadId}` | A saved body referenced by that turn |
| `POST /export` | Complete bounded CSV, one row per turn |
| `POST /export/json` | Complete bounded JSON array, one object per turn |

List query parameters and export JSON fields: `conversationId`, `userId`,
`userIdContains`, `from`, `to`. `userId` and `userIdContains` are mutually
exclusive; an empty or whitespace-only fragment is rejected. Times use RFC3339 offsets and are converted to UTC. Dates select
**conversation creation**, with `from` inclusive and `to` exclusive. Supply
an exact conversation ID or both dates spanning at most 31 days. An exact ID
takes precedence over dates. An owner filter alone is rejected. The UI defaults
to the current owner and the last seven days and waits for explicit search.

Lists also accept `limit` (default 50, maximum 200) and the returned opaque
`cursor`, tied to the selected filters and authenticated caller. Sort order is turn creation
time then ID, descending. Export rejects cursors and covers all applied filters.
Changing the form requires a new search before exporting.
The window follows server cursors with Next Page, Previous Page and First Page.
Rows per page offers 10 (the UI default), 20, 50 and 100 turns. Changing it resets
the cursor history and loads the first page at the selected size. The navigation
bar shows `Page X of Y`; empty results show `Page 1 of 1`. Last Page remains
disabled because cursor paging does not provide the last page's cursor.
Refresh and a new search reset to the first page. A failed or canceled request
leaves the last successful rows, total and cursor history available.

`GET /turns` accepts optional `includeTotal=true` (or `false`, the default).
When requested, the response adds `totalCount`, including zero. A separate
Datly COUNT joins only turns and conversations, applying the same trusted owner,
fragment, conversation and date filters. It ignores the cursor and candidate
limit and never hydrates messages, runs, model/tool calls or payloads. The UI
requests this total only on Search turns and Refresh; page navigation and size
changes reuse it. Totals reflect that last request, not a frozen snapshot.
Next Page follows `nextCursor` even if data changed after the total was read.

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
Dates use RFC3339Nano and duration uses milliseconds. Input and Output retain their
full text. Run IDs, warnings, costs, execution details and payload bodies are
excluded. Text is quoted for CSV and protected against spreadsheet formulas;
numeric feedback retains `-1`, `0`, or `1`.

JSON exports the same 18 fields in the same order, with snake_case keys. Models
is an array, measurements and counts are numbers, and unknown measurements are
`null`. Dates use RFC3339Nano. Text preserves the original content without CSV's
formula prefix. No matches produce `[]`. Both buttons export all matching turns
using the applied filters, independently of the current page and page size.

The diagnostic turn endpoint retains saved execution details. Its `row.runs`
contains the saved run records, including their `id`, attempt and
`resumedFromRunId` when recorded. A turn can have several runs when execution
resumes. These records are available through the API without a UI inspector.

## Limits and interpretation

- CSV and JSON share limits: 1,000 turns, 32 MiB, 30 seconds, two concurrent exports per instance.
  The complete file is buffered before success headers. Exceeding a limit
  returns an error; no successful truncated file is sent.
- Diagnostic API details: at most 10,000 saved messages and 10,000 runs, with
  serialized projection size bounded to 32 MiB. Oversized details return an error.
- Enriched lists apply the same per-turn limits, with both the batch projection
  and complete response bounded to 32 MiB. Exceeding a limit returns an error,
  without silently shortening Input or Output. Narrow the filters or request
  a smaller page. The UI defaults to 10 turns per page; the API default remains 50.
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
functionality checks use synthetic SQLite and MySQL 8 fixtures; production
MySQL load still requires environment-specific measurement.

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

The optional `TestAuditMySQLLiteralOwnerFragments` check runs when
`AGENTLY_AUDIT_TEST_MYSQL_DSN` is set. Use a disposable test MySQL instance.
It creates and drops a uniquely named synthetic database, ignoring the database
name in the supplied DSN; the test account needs database creation and deletion
privileges. It checks literal owner fragments, exact ID case sensitivity,
pagination, totals, enriched content and CSV/JSON scope.

Set `AGENTLY_AUDIT_TEST_MYSQL_LOAD=1` with the same disposable DSN to also run
`TestAuditMySQLSyntheticLoad`. It measures the real Datly count and enriched
first/midpoint pages separately at 10,000 and 100,000 synthetic turns, for own,
admin all, exact owner and owner fragment scopes. Samples exclude initial
component compilation. These fixtures contain no saved message bodies or model
calls; they measure selection overhead, not large-content hydration or live load.

The local MySQL 8 run on 2026-11-03 passed at both sizes. At 100,000 turns,
median counts across the four scopes took 367–452 ms; enriched first pages took
873–1,668 ms and midpoint pages 1,087–1,565 ms. Each median used five warmed
samples. These are local synthetic measurements under shared machine load,
not production latency guarantees. Navigation does not repeat the count.

The browser check uses synthetic API responses and the real host window,
Forge controls and workspace theme at 1440px and 390px. It uses installed Chrome
by default; `PLAYWRIGHT_CHANNEL` selects another installed channel.
Calendar vendor styles use CSS `@scope` inside this destination. Calendar
popovers stay inside it; other windows do not receive those styles.

Verification also covered existing chat shell, navigation, typography and
feedback tests. A scheduler timing test that failed during concurrent checks
passed on its own. A live model conversation and production MySQL were not
exercised. Two SDK baseline failures were reproduced using the unchanged SDK
files from HEAD and left untouched:

- `sdk/TestCanonicalModelInclusiveNativeTimelineFixture`: fixture does not
  include the current feedback fields returned by the canonical builder.
- `sdk/TestMobileSDKPublicSurfacesCoverClientContract`: the Android surface
  lacks `clearTurnFeedback` and `setTurnFeedback`.
