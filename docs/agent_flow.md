# Agent Processing Flow

This document walks through the **runtime sequence** Agently follows when a
client interacts with an agent (e.g. via the CLI command `agently chat -l
my-agent`).  Knowing the order of operations helps when you want to extend any
of the hooks (workspace, loaders, memory, LLM provider, tools, …).

```
┌─────────────────────────────────────────┐
│ 1. Client / CLI issues a request        │
└─────────────────────────────────────────┘
                │  (query, conversation-id, agent-name)
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
│      – appends user message to memory  │
└─────────────────────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────┐
│ 5. Planning & Execution                │
│    • genai/agent.Finder chooses plan   │
│    • Steps may call tools via MCP      │
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

## Workspace lookup details

When a **relative** path is supplied (like `sales.yaml`) the loader prepends
the *baseURL* defined in the executor’s meta-service.  If `Config.BaseURL` is
omitted Agently falls back to the **centralised workspace** root:

```
$AGENTLY_ROOT  (env)   default: ~/.agently
└── agents/
    └── sales.yaml
└── models/
    └── gpt4o.yaml
└── mcp/
    └── local.yaml
```

Thus agents/models dropped into the workspace are auto-discoverable without
changing application code.

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
