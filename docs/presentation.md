## Agently – Agent Framework + MCP Hosts

 - Agent framework in Go to build, run, and compose AI agents.
 - LLM‑agnostic: OpenAI, Vertex AI, Bedrock, others via provider adapters.
 - End‑to‑end surfaces: CLI and HTTP API.
 - Workflow‑driven using Fluxor (plan → execute → finalize, or direct action calls).

## Speaker Line:
 - "Agently brings agents, tools, and Fluxor workflows together in a production‑ready Go framework with CLI and HTTP APIs."


## Core Capabilities – Overview

- Conversation memory with durable transcripts and payload handling.
- Tool integration via MCP and internal services; policy‑controlled.
- Model and embedding providers; prompt templating with Velty.
- Scheduling and operational metadata for recurring runs.
 - Multi‑step execution via Fluxor actions/workflows; agents can be exposed as tools.

## Speaker Line:
 - "Think of capabilities in four buckets: memory, tools, models/embeddings, and Fluxor actions—with guardrails through policies."


## Conversation Memory

- Maintains conversation history and context across turns.
- Stores large outputs as payloads retrievable via payload API.
- Conversation manager ensures ID, appends user/assistant messages, and returns structured outputs.

## Speaker Line:
- "Every interaction is stateful—memory tracks turns and large outputs so agents can reliably pick up context."


## Tooling via MCP

- Integrates tools using the Model Context Protocol (MCP): stdio and SSE transports.
 - Agents can call tools like `system/exec` or external SQLKit; tools can also be triggered by Fluxor actions.
- Tool policy modes: `auto | ask | deny` for controlled execution.

## Speaker Line:
- "MCP makes tools pluggable—Agently negotiates capabilities and enforces a policy for when tools may run."


## Models and Embeddings

- Multiple LLM providers through adapters (OpenAI, Vertex AI, Bedrock, etc.).
- Embedding providers for semantic search/retrieval.
- Velty templating for prompts: `${Prompt}` and contextual variables.

## Speaker Line:
- "Swap models or add embedders via workspace files; prompts are rendered with templates for consistency."


## Policies and Governance

- Tool policy enforcement per agent/session: `auto | ask | deny`.
- Redaction of sensitive keys in logs via `AGENTLY_REDACT_KEYS`.
- HTTP API scope for workspace and agent operations.

## Speaker Line:
- "Execution is governed—tools require explicit policy and sensitive data is scrubbed by default."


## Scheduling

- Built‑in schedule and run tracking (cron/interval/adhoc) with SQL schema.
 - Services expose CRUD and history; scheduled executions create linked conversations and runs.
- Scheduled executions create linked conversations and runs.

## Speaker Line:
- "Schedulers drive recurring agent runs and track history—ideal for reports, sync, or monitoring tasks."


## Multi‑Agent Composition

- Chains — compose sub‑agents inside an agent for targeted delegation; most powerful composition.
- Agents as Tools — expose agents as virtual tools to enable hand‑offs between agents.
- Fluxor Actions/Workflows — coordinate complex, multi‑step plans across services and agents.

## Speaker Line:
- "Chains, agents‑as‑tools, and workflows are independent options—chains enable the deepest composition; pick per use case."


## Interfaces – CLI, HTTP, Fluxor

- CLI: `chat`, `list`, `serve`, `mcp`, `exec`, `ws`, `model-switch`.
- HTTP API: conversation endpoints under `/v1/api/*` and payload retrieval.
- Fluxor: call actions directly from agents or services for planning/execution.

## Speaker Line:
- "Same engine, two primary faces: CLI for devs and HTTP for services—with Fluxor actions for execution."


## Workspace – Overview

- Single root for editable resources; default `~/.agently` or `$AGENTLY_ROOT`.
- Kinds: `agents/`, `models/`, `embedders/`, `workflows/`, `mcp/`, `feeds/`.
- Hot‑swap reload merges workspace with config; generic `ws` CLI manages resources.

## Speaker Line:
- "Everything lives in the workspace—drop a file, and Agently picks it up live."


## Workspace: Models

- Define provider configs (OpenAI, Vertex, Bedrock) as YAML under `models/`.
- Select per‑agent or via `model-switch`; inherit from executor defaults.
- Example defaults populated on first run (e.g., `openai_o4-mini.yaml`).

## Speaker Line:
- "Models are just files—add or switch them without code changes."


## Workspace: Embedders

- Embedding provider definitions under `embedders/` (e.g., OpenAI text).
- Used for semantic retrieval and knowledge grounding.
- Managed via the same `ws` commands and workspace loaders.

## Speaker Line:
- "Embedders mirror models—configured as first‑class workspace assets."


## Workspace: Agents

 - Agent YAML references prompts, tools, knowledge, and workflow reference.
- Prompts are templated: system and user under `prompt/*.tmpl`.
- Tools include MCP patterns and internal services; chains reference sub‑agents.

### Agent Anatomy
- System Prompt – role, guardrails, and capabilities.
- User Prompt – primary query template and context bindings.
 - Knowledge – optional local documents and notes for grounding.
- Tools – list of MCP/internal tools available to the agent.
- Chains – optional sub‑agent composition for delegation.

## Speaker Line:
- "Agents bind prompts, tools, and flows—compose chains when a specialist sub‑agent helps."


## Workspace: Knowledge

- Attach local knowledge sources (docs, notes) to an agent folder.
- Used by retrieval workflows to ground responses.
- Easy to iterate: drop files under `agents/<name>/knowledge/`.

## Speaker Line:
- "Ground agents with project‑specific docs for better precision."


## Workspace: MCP Servers

- Configure MCP clients under `mcp/` (stdio, SSE, streaming) as YAML.
- CLI supports `mcp add/list/remove`; hot‑swap updates live.
- Fluxor actions can invoke MCP tools as part of plans.

## Speaker Line:
- "Define tools once via MCP configs—Agently handles discovery and routing."


## Workspace: Workflows

- Fluxor graphs stored under `workflows/`; referenced by agents.
- Actions (plan, execute, finalize) and custom actions coordinate multi‑step work.
- Agents and services call Fluxor actions directly.

## Speaker Line:
- "Workflows make agent behaviour explicit—plan, execute, and finalize are visible and testable."


## Workspace: Elicitation

- Overlay, not source: workspace `elicitation/` files augment base elicitation schemas produced by LLM plans or tools.
- Matching: overlays select by field set (names) and apply UI attributes onto JSON‑Schema properties.
- Visual attributes: order, labels, placeholders, width, hints, enums; ensures consistent UX across tools/steps.
- Non‑intrusive: overlays do not change business logic—only presentation metadata for your UI layer.
- Integration: applied to elicitation emitted by tools or LLM; acceptance still flows via conversation API callbacks.

## Speaker Line:
- "Workspace overlays enrich elicitation with consistent visual attributes—matched by fields and merged without changing behaviour."


## Workspace: Tool Feeds

- Purpose: explain agent actions by tracking tool calls and capturing results for review.
- Match: each feed matches a tool (`service/method`) and binds data for UI rendering.
- Activation modes:
  - `history` — read recorded tool outputs from the current turn (e.g., terminal logs).
  - `tool_call` — actively invoke a related tool to fetch extra details (e.g., snapshot of pending patches).
-- Defaults:
  - `plan` (history of planning action updates) — shows current steps and statuses.
  - `terminal` (history of system/exec.execute) — streams command logs.
  - `changes` (tool_call to system/patch.snapshot) — previews patch files and offers commit/revert actions.
- Integration: feeds can call MCP servers to enrich context (e.g., list of patched files after code edits).
- Validation: feeds are validated on startup; invalid specs fail fast for safety.

## Speaker Line:
- "Tool feeds track and visualise what tools did—and can call tools again to fetch richer context like patch snapshots."


## Demo Ideas

- Chat + Tools: run `agently chat` with `ask` tool policy; show `system/exec`.
- MCP + SQLKit: configure `mcp/sqlkit.yaml`, query a live DB by natural language.
- Fluxor Action: trigger a planning/execution action and show step updates.

## Speaker Line:
- "We’ll show chat with a guarded tool call, a DB query via MCP, and the UI managing agents and schedules."
