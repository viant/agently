# Agently

Agently is a full-featured AI agent platform built on [agently-core](https://github.com/viant/agently-core).
It provides an HTTP server, embedded web UI, CLI, and workspace management for
creating and interacting with AI agents powered by LLMs.

## Why Agently

- **Secure MCP hosting** — authority matching, HTTPS-only header reuse, and origin/audience allowlists prevent credential leakage while supporting bearer-first and BFF cookie reuse.
- **Conversation-scoped orchestration** — MCP clients, elicitation, and tool calls are associated to a conversation, preserving auth/session boundaries across multi-step flows.
- **Workspace-driven operations** — agents, models, MCP clients, and policies live in the workspace; reproducible, reviewable, and environment-overridable.
- **OAuth / JWT authentication** — BFF, SPA, bearer, mixed, and local auth modes; RSA/HMAC JWT; distributed token refresh across pods.
- **A2A protocol** — agent-to-agent communication via `/.well-known/agent.json` and `/v1/api/a2a/*`.
- **MCP tool exposure** — optionally expose workspace tools as an MCP HTTP server for external agents.
- **Parallel tool calls** — enabled by default; agents invoke multiple tools concurrently within a single reasoning step.
- **Embedded web UI** — Forge-based React UI served directly from the binary.

## Features

- Multi-LLM support: OpenAI, Vertex AI (Gemini + Claude), Bedrock Claude, Grok, InceptionLabs, Ollama
- MCP integration with per-user BFF auth round-tripper
- Conversation management with SQLite (default) or MySQL
- Scheduler: cron/interval/adhoc with distributed lease coordination
- JWT (RSA/HMAC) and OAuth BFF/SPA/bearer authentication
- CLI: `query`, `list-tools`, `chatgpt-login`, `serve`
- Embedded Forge web UI with navigation and window metadata

## Installation

```bash
# Prerequisites: Go 1.25+, Node.js (for UI builds)

git clone https://github.com/viant/agently.git
cd agently/agently

export OPENAI_API_KEY=your_key

go build -o agently .
```

## Quick Start

```bash
# Start the server (default :8080)
./agently serve

# Start on a custom port
./agently serve -a :9595

# Start with a specific workspace
./agently serve -w /path/to/workspace

# Query an agent (auto-detects local server)
./agently query

# Query with a prompt
./agently query -q "How many tables in my database?"

# List available tools
./agently list-tools

# Login to ChatGPT OAuth
./agently chatgpt-login --clientURL "scy://..."
```

## Server Options

```
./agently serve [flags]

  -a, --addr         Listen address (default :8080)
  -w, --workspace    Workspace root path (overrides AGENTLY_WORKSPACE)
  -p, --policy       Tool policy: auto|ask|deny (default: auto)
      --expose-mcp   Expose tools as MCP HTTP server
      --ui-dist      Optional local UI dist directory
  -d, --debug        Enable debug logging
```

## Tool Policy

Agently has two layers of tool control:

1. coarse runtime tool policy
2. per-bundle approval rules

The coarse runtime policy is set by `--policy` on `agently serve`.

Available modes:

- `auto` — normal operation; tools run when allowed by the selected bundle and agent
- `ask` — interactive approval-oriented mode for risky operations
- `deny` — deny tool execution

Approval rules are separate from the coarse runtime policy and live on tool
bundle match rules.

Supported approval modes:

- `none` — no approval
- `prompt` — block the active turn and ask inline
- `queue` — create a queued approval item for the user

Approval is configured at the bundle-rule level, not on agent tool items.

```yaml
match:
  - name: "system/os:*"
    approval:
      mode: queue
```

Execution order:

1. coarse runtime policy is checked first
2. matching bundle approval config is resolved
3. approval mode is applied (`none`, `prompt`, or `queue`)

That means:

- `deny` still denies before approval is considered
- approval only applies after the tool is otherwise allowed
- queue/prompt approval is a finer-grained control than the top-level policy

## Configuration

Agently uses a workspace directory (`$AGENTLY_WORKSPACE`, default `~/.agently`) with YAML files:

```
~/.agently/
  config.yaml           # defaults, auth, internalMCP
  agents/               # agent definitions (*.yaml)
  models/               # LLM/embedder configs (*.yaml)
  embedders/            # embedder configs (*.yaml)
  mcp/                  # MCP client definitions (*.yaml)
  tools/
    bundles/            # tool bundle definitions
```

### config.yaml defaults

```yaml
default:
  agent: chatter
  model: openai_gpt-5.4
  embedder: openai_text

auth:
  enabled: true
  cookieName: agently_session
  defaultUsername: devuser    # auto-login for local dev; remove for production
  ipHashKey: your-hmac-salt
  local:
    enabled: true
  # OAuth IDP (BFF mode — uncomment to enable SSO)
  # oauth:
  #   mode: bff
  #   name: my-idp
  #   label: My IDP
  #   client:
  #     configURL: ""          # scy resource URL for OAuth client config
  #     redirectURI: ""        # https://your-host/v1/api/auth/oauth/callback
  #     scopes: [openid, profile, email]
```

### Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `AGENTLY_WORKSPACE` | `~/.agently` | Workspace root |
| `AGENTLY_ADDR` | `:8080` | Listen address |
| `AGENTLY_DB_DRIVER` | `sqlite` | Database driver |
| `AGENTLY_DB_DSN` | (workspace SQLite) | Database connection string |
| `AGENTLY_UI_DIST` | (embedded) | Optional local UI dist path |
| `AGENTLY_DEBUG` | `false` | Enable verbose logging |
| `AGENTLY_SCHEDULER_RUNNER` | `false` | Enable scheduler watchdog in-process (scheduled runs only) |
| `AGENTLY_SCHEDULER_API` | `true` | Mount scheduler HTTP endpoints |
| `AGENTLY_SCHEDULER_RUN_NOW` | `true` | Enable run-now endpoint |
| `AGENTLY_SCHEDULER_MAX_CONCURRENT_RUNS` | `0` | Cap on in-flight scheduler runs; `0` = unbounded |
| `AGENTLY_CHATGPT_CALLBACK_PORT` | `1455` | Local OAuth callback port for `agently chatgpt-login`. Integer or `auto` (OS-picked). Must match the OAuth redirect allowlist for OpenAI; `auto` only works with issuers accepting arbitrary localhost ports. Overridden by `--port`. |

## Authentication

### Local (development)

Default `config.yaml` enables local auth with a dev user. Remove `defaultUsername` and set `local.enabled: false` for production.

### JWT

Add to `config.yaml`:

```yaml
auth:
  enabled: true
  ipHashKey: your-hmac-salt
  cookieName: agently_session
  jwt:
    enabled: true
    rsa:
      - /path/to/public.pem
    rsaPrivateKey: /path/to/private.pem
```

### OAuth BFF

```yaml
auth:
  enabled: true
  ipHashKey: your-hmac-salt
  cookieName: agently_session
  oauth:
    mode: bff
    label: My IDP
    client:
      configURL: "scy://..."  # encrypted OAuth client config
      redirectURI: "https://your-host/v1/api/auth/oauth/callback"
      scopes: [openid, profile, email]
```

### Token Refresh

Tokens are proactively refreshed before expiry (default: 15 min lead time). Configurable:

```yaml
auth:
  tokenRefreshLeadMinutes: 15   # refresh tokens this many minutes before expiry
```

## Agent Configuration

Agents are YAML files under `$AGENTLY_WORKSPACE/agents/`:

```yaml
# my-agent.yaml
id: my-agent
name: My Agent
modelRef: openai_gpt-5.4
temperature: 0
parallelToolCalls: true        # enable parallel tool calls (default when omitted)
tool:
  - pattern: system/exec       # internal tools
  - pattern: sqlkit            # MCP tool patterns
knowledge:
  - url: knowledge/
profile:
  enabled: true
  name: My Agent
  description: "What this agent does"
  tags: [code, data]
```

## MCP Server Setup

```yaml
# $AGENTLY_WORKSPACE/mcp/sqlkit.yaml
name: sqlkit
transport:
  type: sse
  url: http://localhost:5000
```

```bash
# Start an MCP server (example: mcp-sqlkit)
git clone https://github.com/viant/mcp-sqlkit
cd mcp-sqlkit && go run ./cmd/mcp-sqlkit -a :5000
```

## MCP Tool Exposure

Expose workspace tools as an MCP HTTP server for external agents:

```bash
./agently serve --expose-mcp
```

Configure in `config.yaml`:

```yaml
mcpServer:
  port: 9090
  toolItems:
    - "system/*"
    - "resources"
```

## CLI Reference

### `agently serve`

Start the HTTP server.

```bash
./agently serve -a :8080 -w /path/to/workspace
```

### `agently query`

Query an agent interactively or one-shot. Auto-detects a running local server.

```bash
./agently query
./agently query -q "What is the schema of my database?"
./agently query --api http://server:8080 --token $TOKEN
./agently query --oob "/path/to/user_cred.enc|blowfish://default"
```

Flags:
- `-q, --query` — prompt text
- `-a, --agent-id` — agent identifier
- `-c, --conv` — conversation ID to continue
- `--api` — server URL (skip auto-detect)
- `--token` / `AGENTLY_TOKEN` — Bearer token
- `--oob` / `AGENTLY_OOB_SECRETS` — OOB credentials for BFF auth

### `agently list-tools`

List available tools from the running server.

```bash
./agently list-tools
./agently list-tools --api http://server:8080
./agently list-tools -s system/os
./agently list-tools -n system/exec.execute --json
```

### `agently mcp list`

List tools in an MCP-oriented format.

```bash
./agently mcp list --api http://server:8080 --token $TOKEN
./agently mcp list --api http://server:8080 --session $SESSION_ID
./agently mcp list -s forecasting --api http://server:8080 --token $TOKEN
./agently mcp list -n forecasting/Total --example --schema --json
```

### `agently mcp run`

Run a tool by exact name with JSON arguments.

```bash
./agently mcp run -n forecasting/Total -a '{"viewId":"TOTAL"}' --api http://server:8080 --token $TOKEN
./agently mcp run -n forecasting/Total -a '{"viewId":"TOTAL"}' --api http://server:8080 --session $SESSION_ID
./agently mcp run -n resources/read -a @args.json --api http://server:8080 --token $TOKEN --json
```

### `agently chatgpt-login`

Login via ChatGPT/OpenAI OAuth and persist tokens.

```bash
./agently chatgpt-login --clientURL "scy://..."
```

## Project Structure

```
agently/
  agently/            # Binary entry point (package main)
    main.go           # Imports cmd/agently, wires cloud storage
    build.yaml        # Endly build pipeline
  cmd/agently/        # CLI commands: serve, query, list-tools, chatgpt-login
  main.go             # Serve() and server orchestration (package agently)
  server/             # HTTP auth, OAuth endpoints, speech, JWT keygen
  runtime/            # Model/embedder finders, tool plugins, scheduler options
  bootstrap/          # Workspace default seeding and config loading
    defaults/         # Default agent, model, embedder YAML files
  metadata/           # Forge UI navigation/window metadata (embed)
  deployment/ui/      # Built UI bundle (embed)
  ui/                 # React/Vite UI source
  e2e/                # End-to-end tests (endly + Go)
  e2e/build-ui-embed.sh  # UI build script
```

## Multi-Platform Architecture

Agently is a multi-platform app:

- `web` — embedded Forge/React UI served by the `agently` binary
- `ios` — SwiftUI app using local `AgentlySDK` and `ForgeIOSPackage`
- `android` — Compose app using local `agently-core-sdk` and `forge-sdk`

### Shared Target Context

Platform targeting should use one shared shape across metadata requests, query
context, and runtime resolution:

```json
{
  "platform": "web|android|ios",
  "formFactor": "desktop|tablet|phone",
  "surface": "browser|app",
  "capabilities": ["markdown", "chart", "upload", "code", "diff"]
}
```

Rules:

- Forge should own the canonical target-context contract for metadata-driven UI
  targeting
- Agently should reuse that same shape for metadata calls and `context.client`
  instead of inventing an app-specific variant
- server-side metadata resolution should consume the same shape
- client-side fallback resolution should consume the same shape

### Metadata Branching

Metadata should be separated by explicit platform and form-factor branches
instead of letting mobile changes mutate shared web windows.

Recommended structure:

```text
metadata/window/<window-key>/
  shared/
  web/
  android/
    phone/
    tablet/
  ios/
    phone/
    tablet/
```

Resolution order should be:

1. exact platform + form factor
2. platform
3. shared
4. legacy fallback only during migration

### Important Constraint

Mobile work must not remove metadata that web still depends on.

### Local Multi-Repo Development

This repo now expects local multi-repo refactors to use the workspace file at:

```text
/Users/awitas/go/src/github.com/viant/go.work
```

That workspace ties together:

- `agently`
- `agently-core`
- `forge`

Use `go.work` for local cross-repo development instead of committing module-level
`replace` directives in `go.mod`.

### Request-Scoped SDK Debug Logging

Agently reuses the `agently-core` request-scoped SDK debug contract. For HTTP
SDK sessions, callers can enable debug logging without turning on global process
debug:

- Go HTTP SDK: `sdk.WithSessionDebug("trace", "conversation", "reactor")`
- TypeScript SDK: `sessionDebug: { level: "trace", components: ["conversation", "reactor"] }`
- iOS SDK: `SessionDebugOptions(level: "trace", components: ["conversation", "reactor"])`
- Android SDK: `SessionDebugOptions(level = "trace", components = listOf("conversation", "reactor"))`

These emit:

- `X-Agently-Debug`
- `X-Agently-Debug-Level`
- `X-Agently-Debug-Components`

If a surface needs mobile-specific behavior:

- create `android/phone`, `android/tablet`, `ios/phone`, or `ios/tablet`
  branches
- keep `web/` as a first-class target
- keep `shared/` minimal and stable

This is especially important for top-level Forge windows such as:

- `chat/new`
- `chat/conversations`
- `schedule`
- `schedule/history`
- `agent`
- `model`
- `oauth`
- `mcp`
- `preferences`
- `tool`
- `workflow`

### Current Migration Direction

The current migration work is tracked in:

- `/Users/awitas/go/src/github.com/viant/agently/multi-platform.md`

That document tracks:

- Forge backend loader work for server-side target-aware metadata selection
- target-aware `$import(...)` resolution
- explicit web / iOS / Android metadata branch migration
- final three-platform verification

## UI Development

```bash
# Build the embedded UI bundle safely
cd ui && npm run build:embed

# Alternative wrapper (also safe)
./e2e/build-ui-embed.sh

# Rebuild binary with updated UI
cd agently && go build -o agently .

# Dev mode (proxies to local server at localhost:9393)
cd ui && npm run dev
```

Notes:
- Do not copy `ui/dist` into `deployment/ui` with a raw `rsync --delete` unless you preserve `deployment/ui/init.go`.
- `npm run build:embed` and `./e2e/build-ui-embed.sh` already handle the safe sync path for the embedded bundle.

## Approval Editors And Callbacks

Tool approvals can now expose generic, selector-driven editors. The same selector
is used both to extract editable data from the original tool request and to
write the user-edited value back into that request before the tool executes.

Supported built-in editor kinds:

- `checkbox_list` — keep/remove items from a collection
- `radio_list` — choose exactly one record from a collection

These editors are supported in:

- prompt approval in the web UI
- prompt approval in the CLI
- queue approval in the web UI

### Workspace Bundle Example

The example below turns `system/os:getEnv` approval into a checkbox list. The
editor reads from `input.names` and writes the filtered list back to the same
path.

```yaml
# $AGENTLY_WORKSPACE/tools/bundles/system_os.yaml
id: system/os
title: System OS
description: OS helpers (e.g. environment variables)
iconRef: builtin:system-os
priority: 60
match:
  - name: "system/os:*"
    approval:
      mode: queue
      prompt:
        acceptLabel: "Allow"
        rejectLabel: "Deny"
        cancelLabel: "Cancel"
      ui:
        editable:
          - name: names
            selector: input.names
            kind: checkbox_list
            label: Environment variables
            description: Choose which environment variables this tool may access.
        forge:
          windowRef: chat/new
          containerRef: approvalEnvPicker
          dataSource: approvalEditor
```

### Record Collection Example

For record collections, use relative selectors for item fields. These selectors
are evaluated against each collection item.

```yaml
approval:
  mode: queue
  ui:
    editable:
      - name: records
        selector: input.records
        kind: radio_list
        label: Records
        itemValueSelector: id
        itemLabelSelector: label
        itemDescriptionSelector: description
```

In this example:

- `selector: input.records` extracts the collection from the tool request
- `itemValueSelector: id` resolves `record.id`
- `itemLabelSelector: label` resolves `record.label`
- the selected record is written back to `records`

### Approval Callback Payload

Approval callbacks are optional. They run inside the active Forge window
context, using the same `lookupHandler(...)` mechanism as other Forge actions.

Callback input shape:

```ts
type ApprovalCallbackPayload = {
  approval?: {
    type?: string
    toolName?: string
    title?: string
    message?: string
    acceptLabel?: string
    rejectLabel?: string
    cancelLabel?: string
    editors?: Array<{
      name: string
      kind: string
      path?: string
      label?: string
      description?: string
      options?: Array<{
        id: string
        label: string
        description?: string
        selected: boolean
      }>
    }>
  }
  editedFields?: Record<string, unknown>
  originalArgs?: Record<string, unknown>
  event?: string
}
```

Callback return shape:

```ts
type ApprovalCallbackResult = {
  editedFields?: Record<string, unknown>
  action?: string
}
```

Callback lifecycle:

1. callbacks run in the order declared under `approval.ui.forge.callbacks`
2. a callback runs only for its matching event, or for all events when `event` is omitted
3. `editedFields` are shallow-merged; later callbacks win on conflicts
4. `action` overrides are also last-wins
5. missing handlers are skipped by the SDK; handler resolution is supplied by the host UI

### Example Forge Handler

The example below keeps callback logic generic and deterministic: it receives
the current edited selection and original request, and returns the normalized
edited field payload that will be written back into the tool request.

```js
// Example Forge action handler
export async function filterEnvNames({ editedFields = {}, originalArgs = {} }) {
  const requested = Array.isArray(originalArgs.names) ? originalArgs.names : [];
  const selected = new Set(
    Array.isArray(editedFields.names) ? editedFields.names : requested
  );

  return {
    editedFields: {
      names: requested.filter((name) => selected.has(name))
    }
  };
}
```

To use that handler end to end:

1. register it in the active Forge window context so `lookupHandler(...)` resolves your handler name
2. reference it from `approval.ui.forge.callbacks`
3. the built-in approval UI renders the editor
4. the callback can normalize or rewrite `editedFields`
5. Agently writes the final edited value back to the same selector path before tool execution

### Current Behavior

What works today:

- built-in approval dialogs render `checkbox_list` and `radio_list`
- queue approvals and prompt approvals both support `editedFields`
- the same selector path is used for extract and write-back
- Forge callbacks can post-process `editedFields` before the approval decision is submitted

What is not implemented yet:

- mounting a fully custom Forge approval container from `windowRef` / `containerRef` / `dataSource`

The current runtime uses the built-in approval editor UI and optional Forge
callbacks together.

### Custom Forge Approval Container

When you need a fully custom approval experience, you can point approval UI at
an existing Forge window/container/data source.

```yaml
approval:
  mode: queue
  ui:
    editable:
      - name: names
        selector: input.names
        kind: checkbox_list
    forge:
      windowRef: chat/new
      containerRef: approvalEnvPicker
      dataSource: approvalEditor
      callbacks:
        - event: approve
          handler: myApproval.normalizeSelection
```

Canonical metadata example included in this repo:

- [approval_editor.yaml](/Users/awitas/go/src/github.com/viant/agently/metadata/window/chat/new/dialog/approval_editor.yaml)
- [approval_editor.yaml](/Users/awitas/go/src/github.com/viant/agently/metadata/window/chat/new/dialog/panel/approval_editor.yaml)
- [approval_editor.yaml](/Users/awitas/go/src/github.com/viant/agently/metadata/window/chat/new/datasource/approval_editor.yaml)

Expected Forge data source shape:

```json
{
  "approval": {
    "type": "tool_approval",
    "toolName": "system/os/getEnv",
    "title": "OS Env Access",
    "message": "The agent wants access to your HOME, SHELL, and PATH environment variables.",
    "editors": [
      {
        "name": "names",
        "kind": "checkbox_list",
        "path": "names",
        "options": [
          { "id": "HOME", "label": "HOME", "selected": true },
          { "id": "SHELL", "label": "SHELL", "selected": true },
          { "id": "PATH", "label": "PATH", "selected": true }
        ]
      }
    ]
  },
  "editedFields": {
    "names": ["HOME", "PATH"]
  },
  "originalArgs": {
    "names": ["HOME", "SHELL", "PATH"]
  }
}
```

The approval container is expected to edit `editedFields`. On approve, Agently:

1. reads the current Forge data source form values
2. takes `editedFields`
3. runs any configured approval callbacks
4. writes the final edited value back to the original tool request using the
   same selector path
5. executes the tool with the rewritten request

Example Forge handler:

```js
export async function normalizeSelection({ editedFields = {}, originalArgs = {} }) {
  const requested = Array.isArray(originalArgs.names) ? originalArgs.names : [];
  const selected = new Set(
    Array.isArray(editedFields.names) ? editedFields.names : requested
  );
  return {
    editedFields: {
      names: requested.filter((name) => selected.has(name))
    }
  };
}
```

### Strict Behavior

If `approval.ui.forge.containerRef` is configured, Agently treats that as an
explicit instruction to use the Forge approval renderer.

There is no silent fallback to the built-in approval editor.

If the Forge window/context/container/data source cannot be resolved:

- the approval dialog shows an explicit Forge error
- approve is disabled
- the user must fix the Forge configuration before continuing

### Example Outcome

For a request like:

```text
What are my HOME, SHELL, and PATH environment variables?
```

if the user deselects `SHELL` in the approval editor, Agently rewrites the tool
request before execution so the final result contains only `HOME` and `PATH`.

## E2E Testing

```bash
cd e2e

# Build binary
endly -t=build

# Run full regression suite (requires MySQL)
endly

# Run just the server regression
endly -t=test
```

## Knowledge / RAG

```yaml
# agent yaml
knowledge:
  - url: knowledge/        # local files
  - embedius:
      config: ~/embedius/config.yaml
      role: user
```

## Resources Tools

The `resources` internal tool provides filesystem and MCP resource discovery:

- `resources:roots` — discover configured roots
- `resources:list` — list files under given locations
- `resources:match` — semantic search via embedder

Configure in workspace `config.yaml`:

```yaml
default:
  resources:
    locations:
      - /path/to/docs
    indexPath: "${runtimeRoot}/index/${user}"
    snapshotPath: "${runtimeRoot}/snapshots"
```

## Scheduler

Run scheduled agent tasks on cron, interval, or ad-hoc basis.

**Serverless** (no scheduler):
```bash
AGENTLY_SCHEDULER_API=false ./agently serve
```

**Dedicated scheduler pod**:
```bash
AGENTLY_SCHEDULER_RUNNER=true AGENTLY_SCHEDULER_API=false ./agently serve
```

## Related Projects

- [agently-core](https://github.com/viant/agently-core) — Embeddable Go runtime (this project's backbone)
- [mcp-sqlkit](https://github.com/viant/mcp-sqlkit) — MCP server for database operations
- [forge](https://github.com/viant/forge) — React UI framework for the web interface
- [datly](https://github.com/viant/datly) — Data access layer for persistence

## License

Apache License 2.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE).

This product includes software developed at Viant (http://viantinc.com/).
