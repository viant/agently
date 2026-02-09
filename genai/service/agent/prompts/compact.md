## Context Checkpoint Compaction — Knowledge Restoration Handoff

You are preparing a **handoff summary** for another LLM that must resume this work **without loss of critical knowledge**.

Your goal is to preserve **decisions, facts, references, and implicit assumptions** needed to continue accurately.

### Required Sections

1. **Current Progress & Decisions**
    - Work completed so far
    - Key decisions, assumptions, and rationale
    - Things explicitly ruled out or deferred

2. **Essential Context & Constraints**
    - Non-obvious background knowledge
    - User preferences, conventions, and hard constraints
    - Formatting, tone, or structural rules that must be followed

3. **Facts Required for Knowledge Restoration**
    - Domain facts the next LLM must *know* to proceed correctly
    - Definitions, mappings, constants, IDs, versions, or terminology
    - Canonical interpretations (e.g., “X means Y in this context”)
    - Any information that would otherwise require re-discovery

4. **Critical References & Artifacts**
    - Prompts, schemas, examples, documents, or links
    - File names, APIs, tools, or identifiers already in use
    - Previously agreed-upon templates or structures

5. **Outstanding Work & Next Steps**
    - What remains to be done
    - Clear, ordered next actions
    - Known risks or decision points ahead

### Guidelines
- Be concise, factual, and explicit
- Prefer bullet points over prose
- Do **not** re-analyze or re-justify decisions
- Treat this as a **state snapshot**, not a discussion
- Optimize for fast restoration of context and intent
