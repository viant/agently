# Agently: Agentic Framework and MCP Hosts

- Speaker: Welcome to Agently — we’ll explore how the framework structures agents, tools, and workflows; how it hosts MCP tools with per‑conversation isolation; and how the Forge UI provides a declarative surface for interaction and inspection.

## Agenda
- Speaker: First, establish the mental model: Workspace vs MCP Hosts. Then the request lifecycle and agentic architecture. We’ll drill into host wiring, elicitation, Forge UI, and developer workflow, and close with extensibility.
- Workspace vs MCP hosts
- Request lifecycle and agentic architecture
- MCP host wiring and elicitation
- Workspace hot‑swap and Forge UI
- CLI and HTTP workflows
- Extensibility and roadmap

## Workspace vs MCP Hosts
- Speaker: Workspace defines editable resources (agents, models, workflows, mcp). MCP hosts are external services Agently connects to using those workspace definitions. Agently is an MCP client host, not an MCP server.
- Workspace (What): Single source of truth at `$AGENTLY_ROOT` for human‑editable artifacts: `agents/`, `models/`, `workflows/`, `mcp/`.
- Workspace (Why): Version, diff, and hot‑swap safely; separate config from code; enable UI to read/write declaratively.
- MCP Servers (What): External MCP servers (stdio/SSE/streaming) defined by YAML under `mcp/`, e.g. `sqlkit.yaml`.
- MCP Servers (How): Declared in workspace; connections are created per conversation on first use and cleaned up automatically.
- Relationship: Workspace declares hosts; at runtime, Agently uses those to resolve tools via the registry and execute calls.


## Configuring MCP Hosts in Workspace
- Speaker: Hosts live under `$AGENTLY_ROOT/mcp`. Each file declares transport and connection details; no code changes required.
- Location: `~/.agently/mcp/*.yaml` (or `$AGENTLY_ROOT/mcp`).
- Transport: `stdio` with `command/args`, or `sse/streaming` with `url`.
- Discovery: `agently mcp list|add|remove` manages these files; UI reads them via workspace metadata endpoints.
- Runtime: Hosts are loaded from workspace; connections are established as needed per conversation.

## MCP Host Config Examples
- Speaker: Two minimal, concrete host definitions — stdio and SSE.
- stdio (local binary):
```yaml
name: local
transport:  
  type: stdio
  command: my-mcp
  arguments: ["--flag"]
```
- SSE (HTTP endpoint):
```yaml
name: sqlkit
transport:
  type: sse
  url: http://localhost:5000
```
Use `agently mcp add|list|remove` to manage these under `$AGENTLY_ROOT/mcp`.

## Isolation and Lifecycle (Functional)
- Speaker: Host usage is per conversation; connections are created on demand and cleaned up automatically. You don’t manage lifetimes.
- Per‑conversation scoping: The active conversation determines which host connection a tool call uses.
- Automatic cleanup: Idle host connections are cleaned up without caller involvement.
- Failure surface: Misconfigured host definitions fail fast with clear errors at call time.

## Security & Auth Between Workspace and Hosts
- Speaker: Auth is passed through, not embedded. When available, the HTTP bearer is forwarded on MCP calls and sensitive data is redacted in logs.
- Bearer propagation: API `Authorization` header is forwarded to downstream tool calls when present.
- Redaction: `AGENTLY_REDACT_KEYS` scrubs secrets in payload snapshots.
- Least Privilege: Workspace separates environment‑specific host configs from agent logic; rotate configs without redeploys.

## Agentic Architecture
- Speaker: Think of Agently as an orchestration layer. The Executor coordinates agents (YAML‑defined behaviors) with LLM providers, tool calls, and memory. Policies (auto/ask/deny) influence when external actions need approval. The separation of concerns ensures you can swap providers, add tools, or rewire plans without code rewrites.
- Executor: `genai/executor` orchestrates agents, tools, workflows (Fluxor) and policies.
- Agents: YAML‑defined behaviors, models, and orchestration flows; resolved from workspace.
- LLM Core: Uniform generation API across providers; preferences and options per request.
- Tools: MCP tools exposed via a registry; also supports virtual tools delegating to other agents.
- Memory: Conversation store and turn metadata used for context and routing.
- Policies: Tool and workflow approval modes (`auto|ask|deny`) applied at CLI/server level.

## Request Lifecycle (High Level)
- Speaker: A CLI/HTTP request becomes a conversation turn with context and attachments. The executor resolves the agent from workspace, composes the plan (Fluxor), and alternates between tool calls and LLM generation. The final answer is stored with payload bodies retrievable via dedicated endpoints for large artifacts.
- Input: CLI/HTTP provides agent id, query, context, attachments, policy, timeout.
- Resolution: Executor loads agent and model preferences from workspace.
- Planning/Workflow: Agent plan executes; steps may call tools via MCP.
- Tooling: Calls go through the MCP registry; results feed back into the plan.
- LLM Generation: Provider invoked with prompt + system prompt; usage tracked.
- Output: Conversation updated; response returned; payloads accessible via API.

## MCP Host: What Agently Provides
- Speaker: Agently treats each conversation as an isolated execution space. Hosts are configured in the workspace; tools are discovered and invoked via a unified registry. Agents can also be exposed as callable tools. When present, bearer tokens are forwarded on MCP calls.
- Workspace‑backed MCP config: YAML under `$AGENTLY_ROOT/mcp` drives transport (stdio/SSE) and options.
- Per‑conversation isolation: tool calls are scoped to the active conversation for routing, persistence, and auth.
- Tool Registry Bridge: Unified catalogue and execution path for MCP tools across CLI and HTTP.
- Virtual Agent Tools: Expose agent behaviors as `service/method` for chaining and composition.
- Auth propagation: API bearer tokens are forwarded to downstream hosts when available.

## Agent→Agent Patterns
- Speaker: Agently can expose an agent as a callable tool. The registry adds virtual tool entries like `agentExec/<agentId>` that call `llm/exec:run_agent` under the hood. This allows coordinator→specialist handoffs or multi‑stage pipelines, especially when paired with Fluxor graphs that encode role‑specific steps.
- Agent As A Tool: Registry injects virtual tools per agent (see `adapter/tool/registry.InjectVirtualAgentTools`). Calls delegate to `llm/exec:run_agent` with `agentId`.
- Chaining: One agent can invoke another via these virtual tools, enabling planner/worker patterns or specialist handoffs.
- Fluxor Workflow: Agents reference `orchestrationFlow` graphs; steps can call MCP tools or internal agent exec, allowing multi-agent pipelines.
- Discovery: Virtual tools appear in the tool catalogue so UI/CLI can enumerate and invoke them like any other tool.


## Elicitation (Schema‑Driven Prompts)
- Speaker: When a step lacks required data, the elicitation service records a control message with a schema, provides a callback URL, and waits. In CLI, a stdin awaiter prompts inline; on the server, the Forge UI presents a form overlay and posts the result. The same router delivers the decision back to the blocked step.
- Service: `genai/elicitation.Service` records control messages, refines schemas, and waits for resolution.
- Router: Conversation‑scoped router delivers decisions back to the waiting goroutines.
- Persistence: Elicitations are stored as conversation messages with a callback URL per request.
- CLI vs Server: CLI uses a stdin awaiter; server uses HTTP callbacks from the UI – no console blocking.
- Unified Endpoints: `POST /v1/api/conversations/{id}/elicitation/{elicId}` resolves accept/decline with optional payload.

## Elicitation Overlays
- Speaker: Overlays are declarative. The design also supports auto/hybrid modes where a helper agent attempts to fill the schema and only falls back to the user if needed. This is configured globally with per‑workflow overrides, keeping plans deterministic yet flexible.
- UI Overlay: Forge windows render requested schemas as forms; submission hits the conversation‑scoped callback URL.
- Auto/Hybrid Modes: Design supports auto‑elicitation via a helper agent or hybrid fallback to user (see `docs/elication_ext.txt`).
- Declarative Control: Default mode at executor level with per‑workflow overrides in Fluxor metadata.
- Audit: Synthetic responses are recorded distinctly so clients can differentiate user vs auto‑filled entries.

## Tool Execution + Policies
- Speaker: Tool invocations run through one path regardless of entry point. Policies are enforced early: in CLI, ‘ask’ prompts on stdin; on server, approvals flow through HTTP/UI. Results are normalized to text/bytes/JSON, and persisted in the conversation for later visualization.
- Policy Modes: `auto` (no approval), `ask` (interactive in CLI), `deny` (blocked). Server ask is handled via UI.
- Invocation Paths: `chat`, `exec`, and server routes enrich context with conversation and auth before invoking tools.
- Result Shaping: Results returned as text, bytes, or JSON depending on tool output; persisted where applicable.

## Detailed Execution
- Speaker: Context carries conversation/turn IDs and policies. The registry resolves the canonical tool name, matches it to a configured MCP host, and executes. Agent‑as‑tool shortcuts delegate to agent execution. Results and payload bodies are persisted; large bodies are fetched via payload endpoints.
- Context enrichment: Conversation ID and turn meta enable routing, persistence, and host scoping.
- Policy injection: Tool and Fluxor policies govern approvals; CLI uses stdin for ask, server uses HTTP/UI approvals.
- Host selection: Tool names map to hosts from workspace configuration; calls execute through the selected host.
- Tool dispatch: Unified definition/execute path; agent tools delegate to agent run.
- Persistence: Messages, tool calls, and payloads are recorded for later retrieval and UI visualization.

## Workspace and Hot‑Swap
- Speaker: Everything customizable lives under `$AGENTLY_ROOT`. You can add agents, models, workflows, and MCP definitions while the process runs; hot‑swap detects changes and minimally reloads the affected registry without downtime.
- Layout: `~/.agently/{agents, models, workflows, mcp}` with CLI `ws` helpers to list/get/add/remove resources.
- Hot‑Swap: File changes are detected and minimally reloaded without restarting the process.
- MCP Entries: `agently mcp list|add|remove` manage client definitions in the workspace.

## Forge UI Integration
- Speaker: The UI is metadata‑driven. Navigation and windows are declared in YAML and served by Agently. Windows define data sources that point to workspace and conversation endpoints. This lets you add or reshape UI flows without code changes.
- Metadata Endpoints: `GET /v1/workspace/metadata` aggregates defaults, agents, tools, models.
- UI Metadata: Served under `/v1/api/agently/forge/*` from `metadata/` (navigation and window definitions).
- Pattern: Windows declare a datasource to the aggregated metadata endpoint; menus point to windows by key.
- Tool Visualization: Declarative instrumentation can surface plan steps, diffs, and exec history (see `docs/tool.md`).

## Tool Feeds
- Speaker: Feeds describe how to project tool outputs into structured data sources. Using selectors, Agently extracts values from request/response payloads; names are auto‑disambiguated per turn. Feeds can also proactively invoke a tool to populate a view when no prior call exists in the turn.
- FeedSpec: Match on `service/method`, choose activation `history` or `tool_call`, and define `scope` (`last|all`).
- DataSources: Provide a `source` selector for extracting from request/response payloads; names are per‑turn hashed to avoid clashes.
- Engine: See `pkg/agently/conversation/tool_feed.go` for selection, merge, and stable ordering; UI refs are rewritten to hashed names.
- On‑Demand Invocation: With `activation.kind=tool_call`, a feed can invoke a tool to populate its view even without a prior call.
- UI Binding: Containers (tabs/panels) consume the transformed data sources; payload bodies are trimmed from messages once feeds are computed.

## Developer Workflow
- Speaker: Start with `agently chat` for rapid iterations; use `agently exec` to drive tools directly; and `agently serve` for the full HTTP + UI surface. Manage MCP entries with `mcp add/list/remove` in the workspace so configuration is consistent across environments.
- CLI Chat: `agently chat -a chat -q "…"` with `--context` and `--attach` support.
- Direct Exec: `agently exec -n service/method -i '{…}'` with stdin and file JSON options.
- Server: `agently serve` exposes chat API, payload fetch, tool run, elicitation callbacks, and Forge UI.
- MCP Quick Start: Configure `$AGENTLY_ROOT/mcp/*.yaml`; run external servers (e.g., sqlkit) and add agent tool patterns.

## Error Handling and Observability
- Speaker: HTTP responses use a consistent envelope with precise status codes; logs capture LLM/tool/task events. Sensitive keys in snapshots are redacted via environment configuration to keep operational data safe.
- HTTP: 200 OK, 202 Accepted, 204 No Content, 400/404 for client errors, 500 on server errors; JSON includes status/message.
- Logging: Unified log file option for LLM, TOOL, TASK at runtime; optional tool debug logging for registry calls.
- Redaction: Configurable key redaction for payload snapshots via `AGENTLY_REDACT_KEYS`.

## Extensibility
- Speaker: Add agents and tools declaratively; extend with custom windows for visualization; and compose richer multi‑agent workflows. Future work targets deeper execution history, more granular approvals, and additional virtual agent helpers.
- Add Agents: Drop YAML into `agents/` and reference workflows/models; hot‑swap picks up changes.
- Add Tools: Point agents to MCP tool patterns; augment MCP workspace configs; virtual agent tools can wrap agents.
- Add Models: Configure providers in `models/`; switch defaults per agent or at runtime.
- Custom UI: Add windows/navigation in `metadata/` to surface new capabilities in Forge.

## Roadmap Notes (Examples)
- Speaker: A few directions commonly requested by teams.
- Deeper tool execution history and diff viewers in UI.
- Advanced approval policy editors per tool or workflow node.
- More built‑in virtual tools for common agent behaviors.

## Key Functional Areas
- Speaker: Where to extend or configure functionality conceptually.
- Workspace: Define agents, models, workflows, and MCP hosts declaratively.
- Tools: Discover and invoke MCP tools; expose agents as callable tools.
- Elicitation: Present schema‑driven prompts via CLI or UI; support user or auto/hybrid modes.
- APIs: Use chat, tool‑run, payload, and elicitation endpoints to integrate with apps.

