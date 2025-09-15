# Domain/DAO Unification — Status and Migration Plan

This document tracks the consolidation of Agently's persistence-facing domain API around the DAO read/write models, and the migration away from legacy in-memory DTOs and filters. It also lists remaining unification tasks across HTTP/UI and executor.

## Completed (kept up-to-date)

- Domain interfaces aligned to DAO models
  - Conversations: uses `conversation/read` views and `conversation/write` for Patch.
  - Messages: adapter returns DAO read views; transcript aggregation uses DAO read views for model/tool calls and payloads (no duplicate domain structs).
  - Payloads: uses `payload/read` views and `payload/write` Patch.
  - Usage: uses `usage/read` Input and `usage/write` Patch.
  - Turns: List returns `turn/read.TurnView`; Start/Update accept `turn/write.Turn`.
  - Operations: write accepts DAO `modelcall/write.ModelCall` and `toolcall/write.ToolCall`; read via DAO in v2 HTTP.

- Adapters and wiring
  - Dedicated adapters under `internal/domain/adapter/{conversation,message,operations,payload,turn,usage}`.
  - Composite `Store` wires DAO backends into domain adapters.
  - Data-driven unit tests for each adapter using memory DAO implementations.
  - Store smoke test validates minimal cross-surface behavior.
  - Transcript DTOs (ModelCallTrace, ToolCallTrace) embed DAO `read` views; no duplicate domain `ModelCall`, `ToolCall`, or `Payload` structs.

- Cleanup and naming
  - Legacy domain filters removed (Conversation/Message/Turn/Usage/Payload filters, TranscriptOptions).
  - “Trace” replaced by “Operations” where applicable; transcript aggregation attaches model/tool call summaries.

## Remaining (prioritized)

1) Executor integration (replace history/execution traces)
   - Add a Domain writer/sink to executor and agent services (wires `domain.Store` + mode):
     - RecordMessage(ctx, msg) → `domain.Messages.Patch` (DAO write.Message).
     - RecordTurnStart/Update(ctx, write.Turn) → `domain.Turns.Start/Update`.
     - RecordModelCall(ctx, meta) → `domain.Payloads.Patch` request/response (optional) + `domain.Operations.RecordModelCall(*modelcall/write.ModelCall, requestPayloadID, responsePayloadID)`.
     - RecordToolCall(ctx, name, args, result, status) → snapshots as payloads (optional) + `domain.Operations.RecordToolCall(*toolcall/write.ToolCall, requestPayloadID, responsePayloadID)`.
     - RecordUsageTotals(ctx, convID, per-model + totals) → `domain.Usage.Patch`.
   - Hook points (no behavior change with mode off, shadow writes when enabled):
     - Messages: `genai/extension/fluxor/llm/agent/query_handler.go` → `addMessage`, `recordAssistant`, `recordAssistantElicitation`.
     - Turns: `runWorkflow` (before/after process start) to create/update turn; store TurnID in context for downstream calls.
     - Model calls: `genai/extension/fluxor/llm/core.Generate` right after request/response; avoid double counting; map provider/model, timings, usage if present.
     - Tool calls: `genai/extension/fluxor/llm/exec.CallTool` after input/output publish.
     - Usage totals: after `directAnswer`/`runWorkflow` completion using `qo.Usage`/aggregator.
   - Context propagation:
     - Ensure `ConversationID` and `MessageID` are present (already set in query_handler); introduce `TurnID` in context when workflow starts.
   - Mode gating (AGENTLY_DOMAIN_MODE): off → noop; shadow → write to domain plus legacy; full → deferred to Phase 2.

2) HTTP API v2 (domain-first)
   - Implemented in Phase 2 (initial cut):
     - `GET /v2/api/agently/conversation/{id}/transcript` (TranscriptAggOptions via query params: excludeInterim, includeTools, includeModelCalls, includeToolCalls, payloadLevel, payloadInlineMaxB, redact, since, limit)
       - Returns DAO `message/read.MessageView` rows (ordered + deduped) with embedded `ModelCallView`/`ToolCallView` when present.
     - `GET /v2/api/agently/messages/{messageId}/operations` → `{ "modelCalls": [ModelCallView], "toolCalls": [ToolCallView] }`
     - `GET /v2/api/agently/turns/{turnId}/operations` → `{ "modelCalls": [ModelCallView], "toolCalls": [ToolCallView] }`
     - `GET /v2/api/agently/payload/{id}` (returns DAO read view; optional raw streaming)
     - `GET /v2/api/agently/conversation/{id}/usage`
     - `GET /v2/api/agently/conversation/{id}/turn`
   - Next: payload range support and stricter redaction; SQL-backed store wiring.
   - v1 endpoints remain for compatibility.

### Operations — Response Shape and Examples

Endpoints:
- `GET /v2/api/agently/messages/{messageId}/operations`
- `GET /v2/api/agently/turns/{turnId}/operations`

Response:
```
{
  "status": "ok",
  "data": {
    "modelCalls": [ ModelCallView... ],
    "toolCalls":  [ ToolCallView... ]
  }
}
```

Notes:
- `ModelCallView` and `ToolCallView` are DAO read views. Key fields include provider/model/kind, token usage, `finishReason`, `latency_ms`, `cost` (model), and `status`, `error_message`, `latency_ms`, `cost` (tool).
- Payload snapshots are not embedded; fetch via `/v2/api/agently/payload/{id}`.

Curl examples
-------------

- Transcript with model/tool calls and preview payloads:

```
curl -s "http://localhost:8080/v2/api/agently/conversation/CONV_ID/transcript?includeModelCalls=1&includeToolCalls=1&payloadLevel=preview" | jq
```

- Operations by message (grouped):

```
curl -s "http://localhost:8080/v2/api/agently/messages/MESSAGE_ID/operations" | jq
```

- Operations by turn (grouped):

```
curl -s "http://localhost:8080/v2/api/agently/turns/TURN_ID/operations" | jq
```

3) UI alignment
   - Point ExecutionDetails to v2 operations/payload endpoints.
   - Switch chat message polling to v2 transcript where available.
   - Define Forge metadata for new views (menu + window) using existing patterns.

4) Decommission in-memory ExecutionStore in API
   - Replace `/v1/api/conversations/{id}/execution…` data source with `domain.Operations` + payloads.
   - Keep a thin compatibility layer mapping to prior shapes until UI fully migrates.

5) SDK surface
   - Promote stable domain interfaces (or a public `pkg/domain`) and provide a small HTTP client for v2 endpoints.
   - Include examples for transcript aggregation and operations listing.

6) Deprecations follow-up
- Remove any remaining references to removed domain filters.
- Domain duplicates removed: `domain.ModelCall`, `domain.ToolCall`, `domain.Payload`.

## Phased migration plan

Phase 0 — Readiness
- Compile/runtime flag to enable domain-backed mode (default on in dev).
- Wire a domain.Store with DAO backends (memory for tests, SQL when available).
- Add smoke tests for Store facets (Conversations, Messages, Turns, Operations, Payloads, Usage).

Configuration:
- `AGENTLY_DOMAIN_MODE`: off|shadow|full (Phase 0 enables shadow wiring only)
- `AGENTLY_DB_DRIVER`/`AGENTLY_DB_DSN`: optional SQL wiring for v2 endpoints (memory fallback)
- `AGENTLY_V1_DOMAIN`: 1 enables v1 reads from domain store (compat layer)
- `AGENTLY_REDACT_KEYS`: override default redaction keys for payload snapshots

Phase 1 — Write-path instrumentation (executor)
- Messages: append via `domain.Messages.Patch` (DAO write.Message).
- Turns: `domain.Turns.Start/Update` via DAO write.Turn; carry ConversationID/TurnID.
- Operations: record model/tool calls via `domain.Operations.Record…`. Persist request/response via `domain.Payloads.Patch` and reference IDs.
- Usage: after model completion, patch conversation totals via `domain.Usage.Patch`.
- IDs/causality: ensure deterministic IDs are propagated in context.

Phase 2 — Read-path replacement (server)
- Transcript: add `/v2` transcript endpoint backed by `domain.Messages.GetTranscript` or `GetTranscriptAggregated`, honoring TranscriptAggOptions.
- Operations: add `/v2/messages/{id}/operations` and `/v2/turns/{id}/operations`.
- Payloads: add `/v2/payloads/{id}` (with preview/inline/full levels and redaction).
- Usage: add `/v2/conversation/{id}/usage` backed by `domain.Usage.List`.

Phase 3 — HTTP compatibility layer
- Keep `/v1` endpoints for UI while migrating; reimplement handlers to read from domain.Store and adapt shapes.
- Stage reporting: continue `StageStore` for live progress; optionally mirror into Turns status.

Phase 4 — UI/Forge migration
- Chat view: switch to `/v2` transcript; fallback to `/v1` until parity is proven.
- ExecutionDetails: load operations and payloads from `/v2` endpoints; drop dependency on in-memory execution traces.
- Forge metadata: add windows/menus for operations/usage/turns using existing patterns.

Phase 5 — CLI migration
- Chat/run: write through domain; print responses using transcript aggregation.
- Optional: add CLI commands to inspect operations/payloads with domain.Store.

Phase 6 — Cleanup and decommission
- Remove ExecutionStore and legacy history write paths (guarded by flag until UI completes migration).
- Remove any remaining references to legacy domain filters.

## Testing guidelines

- Continue using data-driven tests with JSON graph comparison and `assert.EqualValues`.
- Prefer memory DAO implementations for unit tests; add SQL-backed integration tests separately.
- For transcript aggregation, test payload levels (none/preview/inline_if_small/full) and redaction.

## Forge notes

- Forge is a data-driven UI (menu + windows defined via REST). For new domain views, follow the existing metadata pattern under `deployment/ui`.
- Keep this document updated as tasks complete or requirements change. Treat sections as living: move items from Remaining → Completed, and adjust Phased plan if we discover better sequencing.
