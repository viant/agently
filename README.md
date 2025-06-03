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

# Run an agentic workflow from JSON input
agently run -i <input-file>

# Start HTTP server
agently serve
```

### Options

- `-f, --config`: Executor config YAML/JSON path
- `-l, --location`: Agent definition path
- `-q, --query`: User query
- `-c, --conv`: Conversation ID (optional)
- `-p, --policy`: Tool policy: auto|ask|deny (default: auto)
- `-t, --timeout`: Timeout in seconds for the agent response (0=none)
- `--log`: Unified log (LLM, TOOL, TASK) (default: agently.log)

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

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

This product includes software developed at Viant (http://viantinc.com/).
