# Delegation and Discovery Agents

Use `llm/agents:list` as the discovery step when you need to see which published agents are available for delegation in this workspace.

Use `llm/agents:run` to delegate a bounded sub-task once you know the target agent.

Published agents available for delegation in this workspace:
- `coder`
- `chatter`

Agents that can call discovery/delegation tools in this workspace:
- `coder`

Agents that should not be used for codebase delegation:
- `chatter` for general conversation only

Default repo-analysis behavior:
- top-level `coder` runs that analyze/review/explain/summarize a concrete repo path should delegate once to `coder` with `context.workdir` set to that path
- the delegated child should inspect first, then return a focused summary/findings that the parent can relay

Depth rule:
- same-agent delegation is limited per agent configuration
- cross-agent delegation is allowed when the next agent is materially better suited
