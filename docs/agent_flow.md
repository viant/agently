# Agent Processing Flow

This document summarizes the agent data model, the runtime services involved
(`agent.Service` and the conversation `Manager`), and the end‑to‑end flow that
turns a user query into a response. It is a practical reference for extending
agents, tools, or the surrounding orchestration.

```
┌─────────────────────────────────────────┐
│ 1. Client / CLI issues a request        │
└─────────────────────────────────────────┘
                │  (query, conversation-id, agent-id)
                ▼
┌─────────────────────────────────────────┐
│ 2. Executor initialises                │
│    • Reads executor.Config             │
│    • Builds meta.Service               │
│      – baseURL = cfg.BaseURL OR        │
│        workspace.Root() ( ~/.agently ) │
└─────────────────────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────┐
│ 3. Agent resolution                     │
│    executor.DefaultAgentFinder()       │
│      └─ Finder.Find()                  │
│          • Cache lookup                │
│          • Otherwise:                  │
│              Loader.Load(ctx,name)     │
│                ├─ metaService.Load()   │
│                │   – resolves $import │
│                │   – joins BaseURL +  │
│                │     relative path    │
│                └─ YAML → agent.Agent  │
└─────────────────────────────────────────┘
                │  (*agent.Agent)
                ▼
┌─────────────────────────────────────────┐
│ 4. Conversation manager                │
│    • Accept()                          │
│      – ensures conversation-id         │
│      – decorates ctx with conv id      │
│      – delegates to agent.Service      │
└─────────────────────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────┐
│ 5. Planning & Execution                │
│    • agent.Service builds Binding      │
│      – Transcript → History            │
│      – Tools: Signatures/Executions    │
│      – Knowledge + MCP resources       │
│    • Orchestrator executes plan        │
│      – tool calls (optional MCP)       │
└─────────────────────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────┐
│ 6. LLM Invocation                      │
│    • agent.Model → llm/provider.<X>    │
│    • Request adapter serialises to API │
│    • provider captures usage           │
└─────────────────────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────┐
│ 7. Response                            │
│    • Conversation memory updated       │
│    • Manager returns QueryOutput       │
└─────────────────────────────────────────┘
```

## Workspace lookup details
Workspace is the single place all editable artefacts live.  Layout:

```
~/.agently/
  agents/      # agent YAML
  models/      # llm provider configs
  workflows/   # fluxor graphs
  mcp/         # MCP servers
```

On start-up the executor automatically merges files in these folders with any
inline definitions found in the main config file, preferring the latter on
name collisions.  Front-ends interact with the workspace through the generic
`ws` CLI group or REST endpoints.

## Modifying the workspace programmatically

```go
import "github.com/viant/agently/internal/workspace"

// add / replace an agent definition
_ = workspace.SaveAgent("demo.yaml", yamlData)

// list all models
models, _ := workspace.ListModels()

// remove an MCP entry
_ = workspace.DeleteMCP("local.yaml")
```

These helpers are pure library calls – the CLI simply reuses them.

## Agent Shape (YAML/JSON)

The `genai/agent.Agent` type is the canonical in‑memory representation of an
agent. Key fields below map 1:1 to YAML/JSON:

- id, name: agent identity; loader sets these from the file when omitted.
- model selection: `modelRef`, plus runtime overrides in QueryInput.
- temperature: float. Defaults as per provider.
- description: free‑form description.
- prompt/systemPrompt: templated `prompt.Prompt` objects.
- knowledge/systemKnowledge: `[]Knowledge` sources (URIs with match rules).
- tool: `[]llm.Tool` definitions or patterns (used to expose functions).
- toolCallExposure: `turn | conversation` – controls how prior tool calls are
  surfaced to prompts/templates.
- autoSummarize: whether to compact conversation after a turn.
- showExecutionDetails, showToolFeed: UI defaults for chat surfaces.
- parallelToolCalls: hint for providers that support native parallel tool‑use.
- persona: default role for messages this agent emits.
- toolExport: optional exposure as a virtual tool (service/method/domains).
- attachment: behavior for binary attachments; mode `ref|inline`, size/TTL, and
  tool‑result→PDF conversion threshold.
- chains: post‑turn follow‑ups with conditions and publishing policy.
  - on: succeeded|failed|canceled|*
  - target.agentId: required
  - conversation: reuse|link (default link)
  - when: expr or LLM‑evaluated query with expect rules
  - publish: role/name/type/parent
  - limits: maxDepth
- mcpResources: enable attaching top‑N matched resources selected by Embedius
  from declared `locations` (or agent knowledge when locations omitted). Fields:
  `enabled`, `locations`, `maxFiles`, `trimPath`, `match`.

Validation highlights:
- chains[].target.agentId must be non‑empty; `conversation` must be reuse|link.
- `Init()` applies defaults for UI flags and attachments.

## agent.Service (runtime)

Constructor: `service := agent.New(llmCore, agentFinder, augmenter, toolRegistry,
runtime, defaults, conversationClient, opts...)`

Composition and responsibilities:
- llm core: provider‑agnostic generate API (+ usage tracking).
- tool registry: resolves tool signatures and executes functions.
- agentFinder: loads agent definitions on demand.
- augmenter: Embedius‑backed document selection for RAG/MCP resources.
- runtime + orchestrator: Fluxor runtime to run multi‑step plans.
- conversation client: fetch transcript and write results/usage.
- elicitation: manages assistant‑originated missing‑input requests.
- cancel registry: exposes per‑turn cancellation to external actors.
- optional MCP manager: resolves `mcp:` resources for attachments.

Options:
- `WithElicitationRouter(r)` – UI/HTTP callback based elicitation.
- `WithNewElicitationAwaiter(fn)` – local awaiter (CLI).
- `WithCancelRegistry(reg)` – inject or override cancel scope registry.
- `WithMCPManager(mgr)` – enable MCP resource resolution.

Public surface:
- `Name()` → `"llm/agent"`.
- `Methods()` exposes `query` with typed IO.
- `Method(name)` returns an executable matching `query`.

Query IO:
- `QueryInput`:
  - conversationId, parentConversationId, messageId, agentId | agent object
  - query, attachments, transcript (optional preloaded)
  - overrides: `model`, `tools` allow‑list, `context`
  - elicitationMode, autoSummarize, allowedChains, disableChains
  - toolCallExposure override, embeddingModel (for RAG/MCP selection)
- `QueryOutput`:
  - conversationId, agent, content, plan, elicitation, usage, model, messageId

Binding and execution path:
1) BuildHistory – map transcript into prompt history (user/assistant/tool).
2) Build tool layer – signatures and prior executions as per exposure rules.
3) Build documents – agent knowledge and optional MCP resources (top‑N).
4) Build task – query + attachments; include context map.
5) Orchestrate – execute plan/steps using runtime and tool registry.
6) Persist – post usage and messages via conversation client.

Error handling follows Go idioms: errors are returned up the call chain. No
printing/logging inside helpers unless required by an interface.

## Conversation Manager

Type: `genai/conversation.Manager` – a thin coordinator that owns:
- `handler` (QueryHandler): delegates to `agent.Service.Query` (or a stub in
  tests).
- `idGen`: defaults to `uuid.NewString`.
- `resolver`: optional agent resolver used by `ResolveAgent`.

Key methods:
- `Accept(ctx, *QueryInput) (*QueryOutput, error)`
  - ensures `conversationId` (generates if empty)
  - decorates context with the conv id via `memory.WithConversationID`
  - delegates to handler, returns typed output
- `ResolveAgent(ctx, id) (*agent.Agent, error)` when a resolver is installed.

This separation keeps transport layers (CLI/HTTP) minimal: they only need to
construct `QueryInput` and call `Accept`.
