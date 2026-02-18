# MCP Server Knowledge Base (Go)

This is a practical guide for designing and validating MCP servers with different capability sets.

## Files
- `mcp.md` (index)
- `mcp_build_guide.md` — base architecture and implementation flow
- `advenced_pattern.md` — advanced production patterns (OOB auth, locking, namespaces)
- `mcp_inspector_validation.md` — official Inspector validation flow
- `mcp_webdriver_testing.md` — WebDriver automation strategy for Inspector-based testing

## Build Progression
1. Minimal server: initialize + tools/list + tools/call.
2. Add knowledge surfaces: resources and prompts.
3. Add templates: resources/templates/list.
4. Add advanced capabilities: elicitation, sampling, auth.
5. Add compatibility controls: protocol negotiation and feature gating.
6. Validate by transport: HTTP/SSE, streamable HTTP, stdio, and stdio-via-bridge.

## Capability Matrix
- Tools: execute actions, return structured and text content.
- Resources: expose discoverable/readable data URIs.
- Resource templates: parameterized resource discovery.
- Prompts: reusable message templates for client orchestration.
- Elicitation: request missing user input from client.
- Sampling: ask the client/LLM side to generate model output.
- Auth/OOB: external authorization path for protected operations.

## Core Rules
- Keep business logic independent from transport/protocol wiring.
- Advertise only capabilities that are actually implemented.
- If capability is unavailable, fail fast with explicit error output.
- Negotiate protocol version during `initialize` and gate newer features.
- Treat Inspector as protocol truth and automation target.
