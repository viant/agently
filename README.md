# Agently

### Mic voice control

When using the mic/dictation input in the chat window, you can speak simple control phrases:

- Cancel the draft (clears composer, does not send): "cancel it now", "cancel now", "never mind"
- Submit the draft (removes the phrase, then sends): "submit it now", "submit now", "send it now"

Agently is a Go framework for building and interacting with AI agents. It provides a flexible and extensible platform for creating, managing, and communicating with AI agents powered by Large Language Models (LLMs).

## Features

- **Agent-based Architecture**: Create and manage AI agents with different capabilities and personalities
- **Multi-LLM Support**: Integrate with various LLM providers including OpenAI, Vertex AI, Bedrock, and more
- **Conversation Management**: Maintain conversation history and context across interactions
- **Tool Integration**: Extend agent capabilities with custom tools
- **Embeddings**: Support for text embeddings for semantic search and retrieval
- **CLI Interface**: Interact with agents through a command-line interface
- **HTTP Server**: Deploy agents as web services

## Why Another Agentic System

- Secure MCP hosting: Enforces authority matching, HTTPS‑only header reuse, and origin/audience allowlists by default, preventing credential leakage while supporting bearer‑first and cookie reuse where safe.
- Conversation‑scoped orchestration: Associates MCP clients, elicitation, and tool calls to a conversation, preserving auth/session boundaries and simplifying multi‑step flows.
- Local services as MCP tools: Wraps internal services as MCP servers with structured schemas and zero network hops, aligning with MCP list/call semantics for portability.
- Decoupled runtime: Coordinates tools and services directly without heavyweight runtimes, reducing latency and operational complexity.
- Workspace‑driven operations: Agents, models, MCP clients, and policies live in the workspace, enabling reproducible changes, safer reviews, and environment‑specific overrides.
- Generic LLM request/response: Normalizes model selection, options, prompts, and outputs across providers while preserving provider‑specific fields for fidelity.
- Provider request/response capture: Observers record provider‑raw payloads (request JSON, streaming deltas, timings, error surfaces) with correlation IDs for end‑to‑end tracing and compliance.
- All tools catalogue: Unified directory lists internal tools and external MCP tools (paged discovery, normalized names, descriptions, schemas) with stable IDs to drive UIs and policy.
- Elicitation in execution timeline: Assistant elicitation is persisted with callback URLs, status transitions, and appears in the turn timeline so executions remain auditable and resumable.
- Tool feed streaming: Emits start/finish/error events with input schema, argument hashes, durations, and structured results across internal/MCP tools; resilient SSE with token‑refresh reconnect.
- Zero‑code MCP activation: Add or switch MCP servers via workspace YAML/CLI; proxy handles name normalization and auth injection; auth bootstrap wires BFF cookies/tokens—no code changes to onboard tools.

### Agent Tools (v2)

Agently exposes a consolidated tool surface for agent selection and execution:

- `llm/agents:list` – returns a small, filtered directory of agents (internal and external) with `{id, name, description, tags, priority}`. Only agents with `directory.enabled: true` are listed; external entries are configured under `directory.external`.
- `llm/agents:run` – runs an agent by `agentId` with an `objective` and optional `context`. Conversation, turn, and user are derived from request context; the runtime publishes status and links parent/child conversations (when enabled in policy).

Backwards compatibility: `llm/exec:run_agent` remains available but is deprecated. Prefer `llm/agents:run` going forward.

Configuration hints:
- Internal agents may opt‑in to the directory with:
  ```yaml
  directory:
    enabled: true
    name: "Coder"
    description: "Generate and refactor code"
    tags: ["code","refactor"]
    priority: 80
  ```
- External A2A agents can be added to the directory via executor config:
  ```yaml
  a2aClients:
    - name: ext-researcher
      jsonrpcURL: https://ext.example.com/v1/jsonrpc
      streamURL:  https://ext.example.com/a2a
      headers:
        Authorization: Bearer ${A2A_TOKEN}
      streamingDefault: true
  directory:
    external:
      - id: researcher
        clientRef: ext-researcher
        name: Researcher
        description: "Find, read and summarize web sources"
        tags: ["research","web"]
        priority: 70
  ```

## Installation

### Prerequisites

- Go 1.24.x or higher

### Installing

```bash
go get github.com/viant/agently
```

### Quick Start
```bash
# Set your OpenAI API key
export OPENAI_API_KEY=your_key

# Clone the repository
git clone https://github.com/viant/agently.git
cd agently/agently

# Set the Agently root directory (defaults to ~/.agently if not set)
export AGENTLY_WORKSPACE=./agently_workspace

# Create the directory
mkdir -p $AGENTLY_WORKSPACE

# Build the application
go build -o agently .

# Check available commands
./agently -h


# Start a agently webservice on :8080 port
./agently serve

# Start a chat cli session
./agently chat
```

### How to run MCP server
To run an MCP (Model Context Protocol) server with SQLKit support:

```bash
# Clone the MCP SQLKit repository
git clone https://github.com/viant/mcp-sqlkit.git

# Navigate to the project directory
cd mcp-sqlkit

# Start the MCP server on port 5000
go run ./cmd/mcp-sqlkit -a :5000
```

The server will be available at http://localhost:5000 and can be used with Agently for database operations.

### Quick Start with MCP server and sqlkit tool

This guide will help you set up Agently with the MCP server and SQLKit tool for database operations.

#### Prerequisites
- Complete the Quick Start steps above to set up Agently
- Have a MySQL server running (this example uses a local MySQL server)

#### Step 1: Configure the MCP Server
1. First, ensure both Agently and any running MCP servers are stopped.

2. Create the MCP configuration file:
```bash
# Create the mcp directory if it doesn't exist
mkdir -p $AGENTLY_WORKSPACE/mcp

# Create the SQLKit configuration file
cat > $AGENTLY_WORKSPACE/mcp/sqlkit.yaml << EOF
name: sqlkit
version: ""
protocol: ""
namespace: ""
transport:
    type: sse
    command: ""
    arguments: []
    url: http://localhost:5000
auth: null
EOF
```

#### Step 2: Configure the Agent to Use SQLKit
Update your agent configuration to include the SQLKit tool:

```bash
# Create or update the chat agent configuration
mkdir -p $AGENTLY_WORKSPACE/agents
cat > $AGENTLY_WORKSPACE/agents/chatter.yaml << EOF
description: Default conversational agent
id: chat
modelRef: openai_o4-mini
name: Chat
orchestrationFlow: workflow/orchestration.yaml
temperature: 0
tool:
  - pattern: system/exec
  - pattern: sqlkit
knowledge:
  - url: knowledge/
EOF
```

#### Step 3: Start the Services
1. Start the MCP server with SQLKit (follow the "How to run MCP server" steps above)
2. Start Agently (run `./agently chat`)

#### Step 4: Test Database Connectivity
Ensure your MySQL server is running. For this example, we assume:
```
port: 3306
user: root
password: dev
database: my_db
```

#### Step 5: Query Your Database
Send a query to test the database connection:
```
> tell how many tables do we have in my_db db?
```

A configuration wizard will guide you through setting up the MySQL connector:
1. When prompted for connector name, enter: `dev`
2. For driver, enter: `mysql`
3. For host, enter: `localhost`
4. For connector name, accept the default: `dev`
5. For port, enter your MySQL port (e.g., `3306`)
6. For project, just press Enter to skip (not needed for MySQL)
7. For DB/Dataset, enter: `my_db`
8. For flowURI:

   8.1 open given link in browser and submit user and password (e.g., `root` and `dev`)

   8.2 enter any value or accept the default

After configuration, Agently will execute your query and return the result, such as:
```
There are 340 base tables in the my_db schema when accessed via the dev connector.
```

You can now use natural language to query your database through Agently!

## Usage

### Match Defaults (auto full vs match)

You can control auto full vs match behavior and result capping via a single default in your workspace config:

```yaml
default:
  match:
    # Used when a knowledge/MCP entry doesn't specify maxFiles.
    # Also drives the auto decision: if a location has more files than this,
    # the runtime switches to Embedius match; otherwise it loads files directly (full).
    maxFiles: 5
```

Notes:
- minScore (when provided on a knowledge/MCP entry) only filters results in match mode; it does not force match mode.
- URIs are normalized (TrimPath) for stable references and better token caching.
- System documents are injected as separate system messages (content only) and are not rendered in system.tmpl to avoid duplication.

Resources tools

- Service `resources` provides generic resource discovery and retrieval across filesystem and MCP:
  - `resources:roots` — discover configured roots.
    - Input: `{ maxRoots: int }`
    - Output: `{ roots: [{ uri, label, description, kind, source }] }`
  - `resources:list` — list files/resources under provided `locations`.
    - Input: `{ locations: [string], recursive?: bool, maxFiles?: int, trimPath?: string }`
  - `resources:match` — semantic selection over `locations` using the configured embedder.
    - Input: `{ query, locations, model, maxDocuments?, match?, includeFile?, trimPath? }`
- Defaults (optional) under `default.resources` in executor config:
  - `locations`: array of roots (relative to workspace or absolute `file://` / `mcp:server:/prefix`)
  - `trimPath`: optional display trim
  - `summaryFiles`: description lookup order (default: [`.summary`, `.summary.md`, `README.md`])
  - `roots`: optional structured roots (local/workspace) with `upstreamRef`
  - `upstreams`: optional upstream DB definitions for local/workspace resources
  - `indexPath`: optional Embedius index root (supports `${workspaceRoot}`, `${runtimeRoot}`, `${user}`)
  - `snapshotPath`: optional MCP snapshot cache root (supports `${workspaceRoot}`, `${runtimeRoot}`, `${user}`)
  - `runtimeRoot`: optional runtime root (supports `${workspaceRoot}`)
  - `statePath`: optional runtime state root (supports `${workspaceRoot}`, `${runtimeRoot}`)
  - `dbPath`: optional sqlite db file path (supports `${workspaceRoot}`, `${runtimeRoot}`)

### HTTP API (v1)

The embedded server exposes a simple chat API under `/v1/api`:

- Create a conversation:
  ```bash
  curl -s -X POST http://localhost:8080/v1/api/conversations | jq
  # { "status": "ok", "data": { "id": "..." } }
  ```

- Post a message to a conversation:
  ```bash
  curl -s -X POST \
    -H 'Content-Type: application/json' \
    -d '{"text":"Hello"}' \
    http://localhost:8080/v1/api/conversations/CONV_ID/messages | jq
  ```

- Get conversation messages and status:
  ```bash
  curl -s http://localhost:8080/v1/api/conversations/CONV_ID/messages | jq
  ```

- Fetch payload bytes (e.g., attachments or large LLM outputs):
  ```bash
  # raw bytes (204 when empty)
  curl -s -L 'http://localhost:8080/v1/api/payload/PAYLOAD_ID?raw=1' > body.bin

  # JSON envelope without inline body
  curl -s 'http://localhost:8080/v1/api/payload/PAYLOAD_ID?meta=1' | jq
  ```

### Configuration (environment variables)

- `AGENTLY_DB_DRIVER` and `AGENTLY_DB_DSN`: optional SQL connector for persistence
  - Examples:
    - SQLite: `AGENTLY_DB_DRIVER=sqlite`, `AGENTLY_DB_DSN=file:/path/to/db.sqlite?cache=shared`
    - Postgres: `AGENTLY_DB_DRIVER=postgres`, `AGENTLY_DB_DSN=postgres://user:pass@host:5432/db?sslmode=disable`
  - When unset, Agently falls back to a local SQLite database under `$AGENTLY_WORKSPACE/db/agently.db`.

- `AGENTLY_REDACT_KEYS`: comma-separated list of JSON keys to scrub from payload snapshots
  - Default: `api_key,apikey,authorization,auth,password,passwd,secret,token,bearer,client_secret`
  - Example: `AGENTLY_REDACT_KEYS=apiKey,Authorization,password`
- `AGENTLY_INDEX_PATH`: override Embedius index root (supports `${workspaceRoot}`, `${runtimeRoot}`, `${user}`)
- `AGENTLY_SNAPSHOT_PATH`: override MCP snapshot cache root (supports `${workspaceRoot}`, `${runtimeRoot}`, `${user}`)
- `AGENTLY_RUNTIME_ROOT`: override runtime root (supports `${workspaceRoot}`)
- `AGENTLY_STATE_PATH`: override runtime state root (supports `${workspaceRoot}`, `${runtimeRoot}`)
- `AGENTLY_DB_PATH`: override sqlite db file path (supports `${workspaceRoot}`, `${runtimeRoot}`)

Config example (paths)

```yaml
default:
  runtimeRoot: "/var/lib/agently/runtime"
  statePath: "${runtimeRoot}/state"
  dbPath: "${runtimeRoot}/db/agently.db"
  resources:
    indexPath: "${runtimeRoot}/index/${user}"
    snapshotPath: "${runtimeRoot}/snapshots"
```

Runtime layout (default)

```
workspaceRoot/
  config.yaml
  agents/ models/ embedders/ workflows/ tools/ mcp/ ...
runtimeRoot/ (defaults to workspaceRoot)
  state/
    mcp/            # MCP auth state + cookies
  index/<user>/     # Embedius indexes
  snapshots/        # MCP snapshot cache
  db/agently.db     # SQLite fallback (when AGENTLY_DB_* unset)
```

- Scheduler execution (recommended: dedicated runner in serverless deployments)
  - `AGENTLY_SCHEDULER_RUNNER`: enable watchdog inside `agently serve` when set to `1` (default: disabled)
  - `AGENTLY_SCHEDULER_INTERVAL`: watchdog interval when enabled (default: `30s`)
  - `AGENTLY_SCHEDULER_API`: disable scheduler HTTP endpoints when set to `0` (default: enabled)
  - `AGENTLY_SCHEDULER_RUN_NOW`: disable run-now routes when set to `0` (default: enabled)
  - `AGENTLY_SCHEDULER_LEASE_TTL`: DB lease TTL used by the runner (default: `60s`)
  - `AGENTLY_SCHEDULER_LEASE_OWNER`: optional stable lease owner id (otherwise auto-generated)
  - See `docs/scheduler.md`.

### Command Line Interface

Agently provides a command-line interface for interacting with agents:

```bash
# Chat with an agent
agently chat 

# Chat with an agent with a specific query
agently chat -l <agent-location> -q "Your query here"

# Continue a conversation
agently chat -l <agent-location> -c <conversation-id>

# List existing conversations
agently list

# Manage MCP servers
agently mcp list                         # view configured servers
agently mcp add   -n local -t stdio \
  --command "my-mcp" --arg "--flag"
agently mcp add   -n cloud -t sse   --url https://mcp.example.com/sse
agently mcp remove -n local

# List available tools (names & descriptions)
agently list-tools

# Show full JSON definition for a tool
agently list-tools -n system/exec.execute --json

# Run an agentic workflow from JSON input
agently run -i <input-file>

# Start HTTP server
agently serve

# Run schedule watchdog in a dedicated process
agently scheduler run --interval 30s

# Workspace management (new)

Agently stores all editable resources under **`$AGENTLY_WORKSPACE`** (defaults to
`~/.agently`).  Each kind has its own sub-folder:

```
~/.agently/
  agents/      # *.yaml agent definitions
  models/      # LLM or embedder configs
  workflows/   # (deprecated)
  mcp/         # MCP client definitions
```

Use the generic `ws` command group to list, add or remove any resource kind:

```bash
# List agents
agently ws list   -k agent

# Add a model from file
agently ws add    -k model   -n gpt4o -f gpt4o.yaml

# Get raw YAML for workflow
agently ws get    -k workflow -n plan_exec_finish

# Delete MCP server definition
agently ws remove -k mcp -n local
```

### Clearing agents cache
1. Close all running agents.
2. Delete content of ~/.emb

### Model convenience helpers

```bash
# Switch default model for an agent
agently model-switch -a chat -m gpt4o

# Reset agent to inherit executor default
agently model-reset  -a chat
```

### MCP helpers (now stored in workspace)

```bash
# Add/update server definition (stored as ~/.agently/mcp/local.yaml)
agently mcp add    -n local -t stdio --command my-mcp

# List names or full JSON objects (with --json)
agently mcp list   [--json]

# Remove definition
agently mcp remove -n local
```

## Forge UI

Agently embeds a Forge-based web UI with data-driven menus and windows.

- Endpoints:
  - `GET /v1/workspace/metadata` — aggregated workspace metadata (defaults, agents, tools, models). Used by windows to populate forms and menus.
  - `GET /v1/api/agently/forge/*` — serves embedded Forge metadata (navigation and window definitions) from `metadata/`.

- Navigation pattern (`metadata/navigation.yaml`):
  - Define a menu node and point `windowKey` to a window definition under `metadata/window/...`.

  Example:
  ```yaml
  - id: tools
    label: Tools
    icon: function
    childNodes:
      - id: list
        label: Catalogue
        icon: list
        windowKey: tool
        windowTitle: Tools
  ```

- Window pattern (`metadata/window/<name>/...yaml`):
  - Windows can declare a datasource pointing to the aggregated metadata endpoint.

  Example (datasource):
  ```yaml
  service:
    endpoint: agentlyAPI
    uri: /v1/workspace/metadata
    method: GET

  selectors:
    data: data
  ```

Customize menus by editing `metadata/navigation.yaml` and add new windows under `metadata/window/`. The server automatically serves these via `/v1/api/agently/forge/`.

### Options

### Templated agent queries (velty)

Agently supports rendering prompts with the velty template engine so you can
compose rich queries from structured inputs.

Where you can use templates:
- Agent prompts can be templated (Velty). Templates render with variables:
  - Prompt – the original query string
  - everything from context – each key becomes a template variable

Example (templated prompt):

```
query: "Initial feature brief"
queryTemplate: |
  Design this feature based on:\n
  Brief: ${Prompt}\n
  Product: ${productName}\n
  Constraints: ${constraints}
context:
  productName: "Acme WebApp"
  constraints: "Ship MVP in 2 sprints"
```

In addition, the core generation service uses the same unified templating for
Template (with ${Prompt}) and SystemTemplate (with ${SystemPrompt}) together
with any values placed in Bind.

- `-f, --config`: Executor config YAML/JSON path
- `-l, --location`: Agent definition path
- `-q, --query`: User query
- `-c, --conv`: Conversation ID (optional)
- `-p, --policy`: Tool policy: auto|ask|deny (default: auto)
- `-t, --timeout`: Timeout in seconds for the agent response (0=none)
- `--log`: Unified log (LLM, TOOL, TASK) (default: agently.log)

#### mcp list / add / remove

- `mcp list` — lists configured servers. `--json` to output full objects.

- `mcp add` flags:
  - `-n, --name`  – unique identifier
  - `-t, --type`  – transport: `stdio`, `sse`, or `streaming`
  - `--command`   – stdio command (when `-t stdio`)
  - `--arg`       – repeatable extra arguments for stdio (when `-t stdio`)
  - `--url`       – HTTP endpoint (when `-t sse|streaming`)

- `mcp remove` flags:
  - `-n, --name`  – identifier to delete

#### list-tools

- `-n, --name` – Tool name (`service/method`) to display full schema
- `--json` – Print tool definitions in JSON (applies to single or all tools)

#### exec

- `-n, --name` – Tool name to execute
- `-i, --input` – Inline JSON arguments (object)
- `-f, --file` – Path to JSON file with arguments (use `-` for STDIN)
- `--timeout` – Seconds to wait for completion (default 120)
- `--json` – Print result as JSON

Example (properly quoted for Bash/Zsh):

```bash
./agently exec -n system/exec.execute \
  -i '{"commands":["echo '\''hello'\''"]}'
```


## Development

### Project Structure

- `cmd/agently`: Command-line interface
- `genai/agent`: Agent-related functionality
- `genai/conversation`: Conversation management
- `genai/embedder`: Text embedding functionality
- `genai/executor`: Executes agent tasks or workflows
- `genai/extension`: Extensions or plugins
- `genai/llm`: Large Language Model integration
- `genai/memory`: Conversation memory or history
- `genai/tool`: Tools or utilities for agents

## Further documentation

For an in-depth walkthrough of how Agently processes a request – from the CLI
invocation through agent resolution, planning, LLM call and response – see
`docs/agent_flow.md`.  The document also explains the `$AGENTLY_WORKSPACE` workspace
mechanism introduced in the 2025-06 release.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

This product includes software developed at Viant (http://viantinc.com/).
### Tool-Call Timeout Default

Set a default per-tool execution timeout in your workspace config:

```yaml
default:
  # Seconds to allow each tool call before it is canceled.
  # If not set, Agently defaults to 300s (5 minutes).
  toolCallTimeoutSec: 600
  # Optional: seconds to wait for elicitation (assistant/tool) before auto-decline.
  elicitationTimeoutSec: 120
```

- The agent layer applies these values per turn.
- You can still override tool timeouts via env `AGENTLY_TOOLCALL_TIMEOUT` for ad‑hoc runs; the workspace default takes precedence when present.
