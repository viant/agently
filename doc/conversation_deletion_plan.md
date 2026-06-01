# Conversation Deletion Implementation

Status: implemented for history-panel deletion. Schedule deletion reuse remains a follow-up.

## Implemented Flow

The left history panel now uses the real core API:

```http
DELETE /v1/conversations/{id}
```

The UI opens a confirmation dialog before calling the API. The sidebar removes the row only after backend success. Backend errors are not swallowed:

- `409`: conversation is still in progress.
- `403`: only the owner can delete the conversation.
- `404`: conversation was already deleted or is no longer available.

If the deleted row was selected, the main window moves to a new conversation.

## Core Backend Contract

The backend source of truth is documented in:

```text
../agently-core/doc/conversation-deletion.md
```

The reusable primitive is:

```go
DeleteConversationTree(ctx context.Context, rootConversationIDs ...string) error
```

It is exposed through the Go SDK as:

```go
DeleteConversation(ctx context.Context, id string) error
```

And through the TypeScript SDK as:

```ts
deleteConversation(id: string): Promise<void>
```

## Tree Scope

Deletion includes the full conversation tree:

- Root conversation.
- Child conversations through `conversation.conversation_parent_id`.
- Linked conversations through `message.linked_conversation_id`, recursively.

The backend deletes deepest children before parents.

## Ownership And Active Guard

Every conversation in the tree must be owned by the effective user:

```text
conversation.created_by_user_id == effective user id
```

Conversations with missing owners are rejected for normal user deletion.

Deletion is blocked when active work exists and the newest active timestamp is less than 48 hours old. If the active marker is older than 48 hours, deletion is allowed so broken stuck conversations can be cleaned up.

## Database Cleanup

The core delete runs in one SQL transaction.

Before deleting rows, it collects:

- Conversation IDs.
- Message IDs.
- Turn IDs.
- Run IDs connected by `conversation_id` or `turn_id`.
- Tool approval IDs connected by `conversation_id`, `message_id`, or `turn_id`.
- Deprecated `schedule_run` IDs when the table exists.
- Payload IDs referenced by messages, model calls, tool calls, and generated files.

The implementation explicitly deletes dependent rows instead of relying on FK cascade, because SQLite and MySQL behavior differs in practice.

Handled explicitly:

- `investigation`: retained, with `conversation_id` set to `NULL` when the table exists.
- `schedule_run`: deleted when the deprecated table exists.
- `tool_approval_queue`
- `run`
- `turn_queue`
- `model_call`
- `tool_call`
- `generated_file`
- `message`
- `turn`
- `conversation`
- collected unreferenced `call_payload` rows

## Payload Policy

Payload deletion is scoped, not global:

1. Collect payload IDs connected to the target conversation tree before deletion.
2. Delete the tree.
3. Delete only collected payload IDs that have no remaining references.

This avoids racing with code that creates a payload row before attaching it to a parent row.

Object-backed payload cleanup currently removes only the DB row when unreferenced. Physical object deletion is intentionally deferred until Agently-owned managed storage can be distinguished from workspace/user/external paths.

## Schedule Deletion Reuse

Schedule deletion now reuses the same conversation-tree cleanup through the core schedule cascade delete:

1. Verify schedule ownership.
2. Collect conversations connected to the schedule from schedule annotations and schedule runs.
3. Start root conversation deletion oldest-to-newest.
4. Preserve child-before-parent deletion inside each conversation tree.
5. Delete remaining schedule runs without conversations.
6. Delete the schedule.

## Tests Added

Backend:

- Deletes a tree containing parent, child, and linked conversations.
- Deletes dependent message, turn, run, model call, tool call, generated file, tool approval, and unshared payload rows.
- Keeps a collected payload when another conversation still references it.
- Rejects non-owner deletion.
- Returns not found for a missing root conversation.
- Rejects recent active conversations.
- Allows stale active conversations older than 48 hours.
- Adds SDK HTTP client and handler coverage for `DELETE /v1/conversations/{id}`.

Frontend:

- Sidebar helper removes only the deleted row.
- Sidebar delete error mapping covers in-progress, permission, and not-found failures.
- TypeScript SDK client coverage for `deleteConversation`.

## Verification Commands

```bash
GOCACHE=/tmp/agently-core-delete-gocache /usr/local/go/go/bin/go test ./app/store/data ./sdk -run 'TestDeleteConversationTree|TestHTTPClient_DeleteConversation|TestHandler_DeleteConversation|TestHandler_UpdateConversation_ErrorStatusMapping' -count=1
npm test -- client.test.ts
APPSERVER_URL=http://127.0.0.1:8080 npm test -- Sidebar.test.js
```
