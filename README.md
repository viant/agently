# Agently

Agently is a Go framework for building and interacting with AI agents. It provides a flexible and extensible platform for creating, managing, and communicating with AI agents powered by Large Language Models (LLMs).

## Features

- **Agent-based Architecture**: Create and manage AI agents with different capabilities and personalities
- **Multi-LLM Support**: Integrate with various LLM providers including OpenAI, Vertex AI, Bedrock, and more
- **Conversation Management**: Maintain conversation history and context across interactions
- **Tool Integration**: Extend agent capabilities with custom tools
- **Embeddings**: Support for text embeddings for semantic search and retrieval
- **CLI Interface**: Interact with agents through a command-line interface
- **HTTP Server**: Deploy agents as web services
- **Workflow Engine**: Built on Viant's Fluxor workflow engine for orchestrating complex agent tasks

## Installation

### Prerequisites

- Go 1.23.8 or higher

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
export AGENTLY_ROOT=./repo

# Create the directory
mkdir -p $AGENTLY_ROOT

# Build the application
go build -o agently .

# Check available commands
./agently -h

# Start a chat session
./agently chat
```

### How to run MCP server
To run the MCP (Model Control Protocol) server with SQLKit support:

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
mkdir -p $AGENTLY_ROOT/mcp

# Create the SQLKit configuration file
cat > $AGENTLY_ROOT/mcp/sqlkit.yaml << EOF
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
mkdir -p $AGENTLY_ROOT/agents
cat > $AGENTLY_ROOT/agents/chat.yaml << EOF
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

# Workspace management (new)

Agently stores all editable resources under **`$AGENTLY_ROOT`** (defaults to
`~/.agently`).  Each kind has its own sub-folder:

```
~/.agently/
  agents/      # *.yaml agent definitions
  models/      # LLM or embedder configs
  workflows/   # Fluxor workflow graphs
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
```

### Options

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
`docs/agent_flow.md`.  The document also explains the `$AGENTLY_ROOT` workspace
mechanism introduced in the 2025-06 release.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

This product includes software developed at Viant (http://viantinc.com/).
