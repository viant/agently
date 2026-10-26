# Design-system contributor entry point

Before changing UI or its theme contract, read [the canonical usage guide](doc/workspace-typography.md).
Reuse the documented shared components and semantic roles. Extend the owning
shared component with its usage example, states and existing checks when a
reusable capability is missing. Record remaining legacy scope in the change.

# Business-neutral host boundary

Business schemas, producer-specific tool names, and domain validation belong to data producers. Do not install business-specific report gates in the host or rename domain fields to disguise that coupling. Preserve MCP payloads through generic interfaces.
