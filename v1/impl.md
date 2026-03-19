# Canonical Render Pipeline

## Goal

Replace the mixed stream/transcript/normalizer rendering with a single canonical render pipeline shared across:

- `agently-core` protocol and transcript shape
- TS/Go SDK normalization helpers
- `agently/v1` UI rendering

The target is:

- one canonical turn model
- one canonical execution model
- one canonical elicitation model
- one canonical linked-conversation model
- one reducer/selectors path from stream and transcript into render state

This moves state semantics down from the UI into `agently-core` and SDK layers so every client does not have to rediscover the same bugs.

## Implementation Status

### Phase 1: Core Canonical Protocol — COMPLETED

All canonical event types are defined and emitted by `agently-core`:

**Event types** (`runtime/streaming/event.go`):

- Stream deltas: `text_delta`, `reasoning_delta`, `tool_call_delta`
- Turn lifecycle: `turn_started`, `turn_completed`, `turn_failed`, `turn_canceled`
- Model lifecycle: `model_started`, `model_completed`
- Assistant content: `assistant_preamble`, `assistant_final`
- Tool call lifecycle: `tool_call_started`, `tool_call_completed`
- Metadata: `item_completed`, `usage`
- Elicitation: `elicitation_requested`, `elicitation_resolved`
- Linked conversation: `linked_conversation_attached`
- Control: `control`, `error`
- Legacy (Response fallback only): `chunk`, `tool`, `done`

Legacy `llm_request_start` and `llm_response` events have been removed entirely.

**Emitters wired**:

- `internal/service/conversation/service.go` — emits `model_started`/`model_completed`, `assistant_preamble`/`assistant_final`, turn lifecycle events with distinct failed/canceled types
- `service/elicitation/service.go` — emits `elicitation_requested`/`elicitation_resolved`
- `service/linking/service.go` — emits `linked_conversation_attached`
- `service/reactor/service.go` — uses `model_completed` for planned tool calls

### Phase 2: SDK Canonical State — COMPLETED

All canonical state types and pipeline live in `sdk/`:

**State types** (`sdk/canonical.go`):

- `ConversationState`, `TurnState`, `TurnStatus`
- `UserMessageState`, `AssistantState`, `AssistantMessageState`
- `ExecutionState`, `ExecutionPageState`
- `ModelStepState`, `ToolStepState`
- `ElicitationState`, `ElicitationStatus`
- `LinkedConversationState`

**Transcript builder** (`sdk/canonical_transcript.go`):

- `BuildCanonicalState(conversationID, turns)` — single entry point converting transcript DAO views into canonical `*ConversationState`

**Reducer** (`sdk/canonical_reducer.go`):

- `Reduce(state, event)` — source-agnostic state machine applying stream events to `*ConversationState`
- Handles all canonical event types

**Selectors** (`sdk/canonical_selectors.go`):

- `SelectRenderTurns(state)`
- `SelectVisibleExecutionPage(turn, pageIndex)`
- `SelectAssistantBubble(turn, pageIndex)`
- `SelectElicitationDialog(turn)`
- `SelectLinkedConversations(turn)`
- `SelectLinkedConversationForToolCall(turn, toolCallID)`
- `SelectExecutionPageCount(turn)`
- `SelectTotalElapsedMs(turn)`

**API surface**:

- `Client.GetTranscript()` returns `*ConversationState` directly
- Legacy types (`ExecutionGroup`, `TranscriptTurn`, `TranscriptOutput`) removed
- Legacy `execution_groups.go` gutted — only internal helper functions retained for transcript building

### Phase 3: UI Migration — COMPLETED

**Adapter layer** (`chatRuntime.js`):

- `isCanonicalState(data)` — detects canonical `ConversationState` response format
- `canonicalPageToExecutionGroup(page, index)` — converts `ExecutionPageState` → legacy `ExecutionGroup` shape
- `canonicalStateToLegacyTurns(state)` — converts full `ConversationState` → old turn format with `message[]` and `executionGroups[]`
- `fetchTranscript` — auto-detects canonical response and converts via adapter

**Stream event handlers** (`chatRuntime.js`, `ExecutionWorkspace.jsx`):

- `llm_request_started` → `model_started` (with backward-compat fallback)
- `llm_response` → `model_completed` (with backward-compat fallback)
- Added handlers: `turn_started`, `turn_failed`, `turn_canceled`, `assistant_preamble`, `assistant_final`, `elicitation_requested`, `elicitation_resolved`, `linked_conversation_attached`

**Pagination** (`IterationBlock.jsx`):

- `loadHistoricalPage` updated to use full transcript fetch with client-side slicing (backend `ExecutionGroup` selector removed)

### Phase 5: Streaming Contract Unification — COMPLETED

**Richer domain event types** (`runtime/streaming/event.go`):

- Stream delta types: `text_delta`, `reasoning_delta`, `tool_call_delta`
- Lifecycle types: `tool_call_started`, `tool_call_completed`, `turn_started`, `turn_completed`, `turn_failed`, `turn_canceled`
- Model lifecycle: `model_started`, `model_completed`
- Assistant content: `assistant_preamble`, `assistant_final`
- Metadata: `item_completed`, `usage`
- Legacy `chunk`/`tool`/`done` retained only for un-migrated Response fallback path
- `FromLLMEvent` maps each provider Kind to a distinct domain event type (no more collapsing to chunk/tool/done)

**StreamOutput preserves full canonical stream** (`service/core/stream.go`):

- `StreamOutput` uses `streaming.Event` directly (was `stream.Event`)
- Deleted `service/core/stream/event.go`
- `appendStreamEvent` maps typed Kind to richer event types; no longer drops `usage`/`item_completed`
- Terminal condition handles both `EventTypeTurnCompleted` and legacy `EventTypeDone`
- `canRetryStreamConsume` detects meaningful output across all event types

**Typed stream deltas** (`genai/llm/stream.go`):

- `StreamEventKind` enum: `text_delta`, `reasoning_delta`, `tool_call_started`, `tool_call_delta`, `tool_call_completed`, `usage`, `item_completed`, `turn_started`, `turn_completed`, `error`
- `StreamEvent` carries: `Kind`, `ResponseID`, `ItemID`, `ToolCallID`, `Role`, `Delta`, `ToolName`, `Arguments`, `Usage`, `FinishReason`
- `Response *GenerateResponse` field retained for observer callbacks and un-migrated consumers

**All providers migrated to typed Kind deltas**:

- OpenAI adapter (`provider/openai/stream.go`, `api.go`):
  - `response.output_item.added` → `StreamEventToolCallStarted`
  - `response.output_text.delta` → `StreamEventTextDelta`
  - `response.function_call_arguments.delta` → `StreamEventToolCallDelta`
  - `emitResponse()` populates `Kind`, `ToolCallID`, `ItemID`, `FinishReason`
- Grok adapter (`provider/grok/stream.go`):
  - Text content deltas → `StreamEventTextDelta`
  - Finalized choices → `StreamEventTurnCompleted`
- Ollama adapter (`provider/ollama/api.go`):
  - Text chunks → `StreamEventTextDelta`
  - Done → `StreamEventTurnCompleted`
- Vertex AI Claude adapter (`provider/vertexai/claude/api.go`):
  - `content_block_start` tool_use → `StreamEventToolCallStarted`
  - `content_block_delta` text → `StreamEventTextDelta`
  - `content_block_delta` partial_json → `StreamEventToolCallDelta`
  - `content_block_stop` → `StreamEventToolCallCompleted`
  - `message_stop` → `StreamEventTurnCompleted`
- Vertex AI Gemini adapter (`provider/vertexai/gemini/api.go`):
  - Text content → `StreamEventTextDelta`
  - Tool calls → `StreamEventToolCallCompleted`
  - Finish → `StreamEventTurnCompleted`
- Bedrock Claude adapter (`provider/bedrock/claude/api.go`):
  - `content_block_start` tool_use → `StreamEventToolCallStarted`
  - `content_block_delta` text → `StreamEventTextDelta`
  - `content_block_delta` partial_json → `StreamEventToolCallDelta`
  - `message_delta` with stop_reason → `StreamEventToolCallCompleted` + `StreamEventTurnCompleted`

**Reactor handles typed events** (`service/reactor/service.go`):

- `handleTypedStreamEvent` processes Kind-based events directly
- `text_delta` → accumulates content
- `tool_call_completed` → creates plan step and launches execution
- Legacy Response path retained for fallback
- `launchPendingSteps` extracted as reusable helper

**SDK reducer handles stream deltas** (`sdk/canonical_reducer.go`):

- `text_delta` → accumulates text on current page
- `reasoning_delta` → accumulates reasoning on current page
- `tool_call_delta` → no-op (waits for tool_call_completed)
- `usage`/`item_completed` → explicit no-op handlers

**P1/P2 fixes applied**:

- Elicitation schema/callback extraction from `UserElicitationData.InlineBody` JSON
- Linked conversation ID from `MessageView.LinkedConversationId` (not JSON content parsing)
- Reducer timestamps from event times (not `time.Now()`)
- Child agent failure: `RunOutput.Error` field with original error
- Stream anchor continuation restored via `BuildContinuationRequest`

### Phase 4: Regression Coverage — PARTIAL

SDK unit tests cover:

- `BuildCanonicalState` with multi-iteration execution pages
- Parent tool message attachment by iteration
- Assistant state extraction (preamble + final)
- Transcript filtering, elicitation enrichment, noise pruning
- Internal conversation service stream event tests
- Stream retry tests with canonical event types

E2E tests updated to use canonical types.

---

## Architecture

Pipeline:

```text
stream events ----\
                   > canonical reducer -> canonical conversation state -> selectors -> UI
transcript -------/
```

Layers:

1. `agently-core`

- defines canonical protocol shape
- emits explicit stream events
- exposes transcript in canonical turn/execution form

2. SDK

- provides reducer/store/selectors
- reconciles stream events and transcript snapshots
- owns source-agnostic state semantics

3. UI

- renders selector output
- owns layout, dialogs, collapse/expand, pagination controls
- does not infer execution structure

## Canonical State Model

```ts
type ConversationState = {
  conversationId: string
  turns: TurnState[]
}

type TurnState = {
  turnId: string
  status: "running" | "waiting_for_user" | "completed" | "failed" | "canceled"
  user: UserMessageState | null
  execution: ExecutionState | null
  assistant: {
    preamble?: AssistantMessageState
    final?: AssistantMessageState
  }
  elicitation?: ElicitationState
  linkedConversations: LinkedConversationState[]
}

type ExecutionState = {
  pages: ExecutionPageState[]
  activePageIndex: number
  totalElapsedMs: number
}

type ExecutionPageState = {
  pageId: string
  assistantMessageId: string
  parentMessageId: string
  turnId: string
  iteration: number
  status: "running" | "completed" | "failed" | "canceled"
  modelSteps: ModelStepState[]
  toolSteps: ToolStepState[]
  preambleMessageId?: string
  finalAssistantMessageId?: string
}

type ModelStepState = {
  modelCallId: string
  assistantMessageId: string
  provider: string
  model: string
  status: string
  requestPayloadId?: string
  responsePayloadId?: string
  providerRequestPayloadId?: string
  providerResponsePayloadId?: string
  startedAt?: string
  completedAt?: string
}

type ToolStepState = {
  toolCallId: string
  toolMessageId: string
  toolName: string
  status: string
  requestPayloadId?: string
  responsePayloadId?: string
  linkedConversationId?: string
  startedAt?: string
  completedAt?: string
}

type ElicitationState = {
  elicitationId: string
  status: "pending" | "accepted" | "declined" | "canceled"
  message: string
  requestedSchema: object
  callbackURL?: string
  responsePayload?: object
}
```

## Canonical Event Contract

Stream and transcript both map to the same semantic events. Two layers:

**Transport layer** (`llm.StreamEvent`) — provider-normalized, low-level:
```
text_delta, reasoning_delta, tool_call_started, tool_call_delta,
tool_call_completed, usage, item_completed, turn_started,
turn_completed, error
```

**Domain layer** (`streaming.Event`) — product events, stable for SDK/UI/storage:
```ts
type ConversationEvent =
  // Stream deltas
  | { type: "text_delta"; content: string }
  | { type: "reasoning_delta"; content: string }
  | { type: "tool_call_delta"; toolCallId: string; toolName: string; content: string }
  // Turn lifecycle
  | { type: "turn_started"; conversationId: string; turnId: string; userMessageId: string; createdAt: string }
  | { type: "turn_completed"; turnId: string; status: string }
  | { type: "turn_failed"; turnId: string; error: string }
  | { type: "turn_canceled"; turnId: string }
  // Model lifecycle
  | { type: "model_started"; turnId: string; assistantMessageId: string; modelCallId: string; provider: string; model: string; createdAt: string }
  | { type: "model_completed"; turnId: string; assistantMessageId: string; modelCallId: string; status: string; completedAt: string }
  // Assistant content (aggregated)
  | { type: "assistant_preamble"; turnId: string; assistantMessageId: string; content: string; createdAt: string }
  | { type: "assistant_final"; turnId: string; assistantMessageId: string; content: string; createdAt: string }
  // Tool call lifecycle
  | { type: "tool_call_started"; turnId: string; toolCallId: string; toolName: string; createdAt: string }
  | { type: "tool_call_completed"; turnId: string; toolCallId: string; toolMessageId: string; toolName: string; status: string; responsePayloadId?: string; linkedConversationId?: string; completedAt: string }
  // Metadata
  | { type: "usage" }
  | { type: "item_completed" }
  // Elicitation
  | { type: "elicitation_requested"; turnId: string; assistantMessageId: string; elicitationId: string; message: string; requestedSchema: object; callbackURL?: string }
  | { type: "elicitation_resolved"; turnId: string; elicitationId: string; status: string; responsePayload?: object }
  // Linked conversation
  | { type: "linked_conversation_attached"; turnId: string; toolCallId: string; linkedConversationId: string }
```

## Render Ownership Rules

The SDK reducer owns state. The UI only renders selectors from that state.

Selectors:

- `selectRenderTurns(state)`
- `selectVisibleExecutionPage(turn, pageIndex)`
- `selectAssistantBubble(turn, pageIndex)`
- `selectElicitationDialog(turn)`

Rules:

1. Exactly one assistant bubble per visible execution page.
2. If visible page has final content, show final content.
3. Else if visible page has preamble, show preamble.
4. Elicitation is not rendered as raw JSON bubble.
5. Linked conversation affordance comes only from canonical `toolStep.linkedConversationId`.

## Source Ownership Rules

The reducer is source-agnostic, but the coordinator must be explicit about transport ownership.

### Live-owned turns

Use stream events as primary updates for the current in-session conversation.

### Transcript-owned running turns

Use transcript polling only.

Do not attach live SSE just because the conversation is currently running if the browser/session did not originate the turn.

### Reconciliation

Transcript snapshots reconcile and settle stream-owned turns by canonical ids:

- `turnId`
- `assistantMessageId`
- `toolCallId`
- `toolMessageId`
- `elicitationId`

## Elicitation Rules

Elicitation is first-class.

### Backend

If a model emits elicitation JSON in assistant content, it becomes canonical elicitation state — not just plain assistant text or a `plan` side channel.

### UI

Execution details: show one elicitation step/status row.

Dialog: show schema form, support submit, support cancel/decline.

Do not render duplicate inline form, duplicate assistant bubble with same elicitation text, or duplicate execution block before/after elicitation.

## Linked Conversation Rules

`llm/agents/run` and similar tool rows expose child conversation linkage in one place only: `ToolStepState.linkedConversationId`.

The reducer attaches this as soon as observed from either stream event, transcript tool message, or transcript linked conversation join. Once attached, it is stable. No component-local fallback lookup.

## Payload Rules

Tool/model payload presentation should be normalized before reaching the UI.

Tool response payload: decoded body when available, otherwise plain error message. Not raw compressed wrapper or wrapped JSON unless it is an explicit overflow/continuation protocol body.

## Continuation Rules

### Implementation

Stream anchor continuation via `BuildContinuationRequest` is re-enabled. It:

- Selects the latest assistant response anchor (`resp.id`) from binding history
- Includes only tool-call messages mapped to that anchor
- Falls back to full transcript for multi-tool anchors (>1 tool call or mismatched counts)
- Logs anchor id, expected tool call ids, tool result ids when skipping

### Guard rails

Multi-tool anchor continuation is explicitly skipped to avoid provider-side 400s when function-call outputs are not fully materialized. Single-tool anchors proceed normally.

## Success Criteria

The redesign is complete when:

1. `agently-core` transcript and stream both produce the same canonical turn/execution semantics.
2. TS SDK can drive the UI from a single reducer/store.
3. `v1/ui` no longer synthesizes execution structure locally.
4. The following flows are stable end to end:
   - `hi`
   - `show HOME env variable`
   - linked child conversation
   - pending elicitation + resolve/cancel
   - repo analysis with successful child summary relay
