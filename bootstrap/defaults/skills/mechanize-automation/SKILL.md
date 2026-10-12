---
name: mechanize-automation
description: Control Chrome websites and macOS applications through connected Mechanize MCP. Load Mechanize's live operating and application skills before browser navigation, form entry, desktop interaction, or screenshot inspection.
---

# Mechanize in Agently

Use the connected Mechanize MCP server for the user's requested automation.
Tool names here are logical names; invoke the exact server-qualified names in
this agent's catalog. Availability depends on workspace MCP configuration.

## Load the shared operating guide

For a request only to list skills, call `skill_list` and answer directly.
For automation, call `mechanize_capabilities` once, then `skill_list` and
`skill_get` using the returned URI for `mechanize-desktop`. Load the relevant
browser/native reference and application skill from its advertised inventory;
never invent a resource URI. Use `mechanize_describe` when additional syntax is
needed, rather than loading duplicate API descriptions up front.

Mechanize's retrieved guide is the canonical source for target discovery,
owned sessions, consent, typed workflows, deadlines, visual fallback, Vault
operations, verification and recovery. Do not assume these MCP tools accept
JavaScript or arbitrary Playwright code. Use their advertised schemas. Saved
trusted-origin policy handles ordinary automation within allowed apps/sites;
request access only when the server explicitly reports `permissionRequired`.

## Read images through Agently artifacts

When a capture returns an artifact reference instead of inline pixels, call
`resources/readImage` with that exact returned URI. This presents the image as
native vision input on the next model call. A URI or textual description alone
is not image inspection. Do not copy encoded image bytes into prompts or tool
arguments.

Use the default screenshot preview first. Request `detail: original` only when
small text or precise visual evidence requires it. Retain the reported image
and coordinate mapping when using a visual click; preview pixels must not be
mistaken for original screen coordinates. Server screenshot classification and
context retention are workspace configuration, not fields the agent should
invent in a capture request.

## Follow one operation to its outcome

A queued or running tool result is not completion. Inspect the original
`operationId` with the advertised status/output tools; do not issue the same
action again to obtain a result. After a timeout, distinguish explicit
`notDispatched` from an uncertain effect before choosing the next action.
Check fresh state and business evidence before claiming success. Preserve user
tabs, unsent drafts and unresolved receipts.

Keep task-specific details in the user's conversation and the relevant private
application guide. This bootstrap contains no website-specific walkthrough.
