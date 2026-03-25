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
| `AGENTLY_SCHEDULER_RUNNER` | `false` | Enable scheduler watchdog in-process |
| `AGENTLY_SCHEDULER_API` | `true` | Mount scheduler HTTP endpoints |
| `AGENTLY_SCHEDULER_RUN_NOW` | `true` | Enable run-now endpoint |

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
./agently list-tools -n system/exec.execute --json
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

## UI Development

```bash
# Build UI and copy to deployment/ui/
./e2e/build-ui-embed.sh

# Rebuild binary with updated UI
cd agently && go build -o agently .

# Dev mode (proxies to local server at localhost:9393)
cd ui && npm run dev
```

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
