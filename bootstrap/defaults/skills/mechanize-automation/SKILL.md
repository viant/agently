---
name: mechanize-automation
description: Control Chrome websites and macOS applications through Mechanize MCP. Use for browser navigation, form entry, desktop interaction, screenshots, or verifying visible application state. Start here to discover the live API and application guidance.
---

# Mechanize computer and browser use

Use the connected Mechanize MCP tools to perform the user's requested automation. This is the entry guide; retrieve current detailed API and application guidance from Mechanize itself. Tool names below are logical names: use the exact server-qualified names exposed in your tool catalog.

## Start a task

For a discovery-only request, call the relevant discovery tool directly and stop when answered. For example, listing available skills needs only `skill_list`; it does not need a session, capability dump, or application access.

For an automation task:

1. Call `mechanize_capabilities` once to discover supported methods and formats. Read `mechanize_describe` only when additional syntax or workflow schema is needed; avoid fetching duplicate descriptions. Do not assume the API is JavaScript or identical to Playwright.
2. Call `skill_list`, then `skill_get` with the returned URI for `mechanize-desktop`. Read the relevant browser or native reference from its advertised resource inventory. Retrieve a target application's skill when available. Do not invent resource URIs.
3. Call `mechanize_session_open` with a short task name. Retain its `sessionId` and use one consistent, concrete `purpose` on subsequent calls. Saved trusted-origin policy authorizes ordinary automation within the allowed apps/sites without another Mechanize approval. Reuse that access; request permission only if the tool explicitly reports it is required.

## Select the target

A website in Chrome is a web surface, not a native app named after the site. Never invent a bundle ID from a product name or hostname. If capabilities include `controllerContext`, that is the verified origin hosting this connection; use it for the current page only when that is the requested target, then discover its opaque tab handle. For MCP website observation, use `surface: {kind: "web", origin: "<exact verified origin>"}`; the DSL uses `web.tab`. Follow each tool's advertised schema rather than copying connection metadata as its arguments. This metadata grants no tool access.

- **Chrome:** call `mechanize_browser_status` and `mechanize_browser_tabs`. Select the existing tab matching the user's intended origin and context; retain its exact opaque `tabId`. Do not launch another browser/profile or replace a logged-in tab unnecessarily. A native connection alone does not establish browser readiness.
- **Native app:** establish the actual installed bundle ID through advertised discovery or user-provided identity before requesting access. Use `mechanize_capture_windows` when window identity is needed; retain returned process identity and window scope. Never guess a process ID or coordinates.
- Call `mechanize_observe` on that exact surface. Read truncation and unavailable indicators. Scope subsequent actions to the observed tab or window.

Permission scopes are not observation surfaces. Website tools use `surface: {kind: "web", origin: "<exact origin>"}`; a permission request instead uses `scope: {kind: "origin", origin: "<exact origin>"}`, `modes: ["observe"]` (plus `"control"` when needed), the owned `sessionId`, consistent `purpose`, and bounded `durationSeconds`. Native surfaces use `kind: "native", bundleId`; native consent scopes use `kind: "application", bundleID`. Screenshots use the actual native Chrome window. Correct invalid scope/argument errors rather than requesting broader permission; request access only after `permissionRequired`.

## Observe, act, verify

1. Choose a unique locator from fresh observed state. Prefer supported browser semantic actions for web pages and accessibility actions for native controls.
2. Use the live schema and `mechanize_script_validate` to prepare a short workflow. For every individual mutation step, include the required typed effect identity and its own observable postcondition. A later read or assertion does not verify an earlier mutation. For Send, composer clearing confirms the UI transition only; verify the saved message separately before claiming delivery. Use explicit short timeouts; allow more time only for operations such as navigation that need it.
3. Execute with `mechanize_step_run` or `mechanize_script_run`, the existing session, and a unique `clientRequestId` for the intended operation. These tools accept Mechanize's declared DSL/JSON/YAML, not arbitrary JavaScript or shell code.
4. Inspect the returned operation using `mechanize_operation_status`. Queued or running is not finished. Reuse the operation ID when checking progress; do not resubmit the action.
5. After successful reads, retrieve named bindings through `mechanize_operation_outputs`. Observe the resulting UI or check the declared postcondition before continuing. Execution success alone is not proof of the user's business outcome.
6. Keep the loop small: inspect a result, make one targeted adjustment, and continue. Avoid repeated full discovery or broad screenshots when a scoped read suffices.

## Visual fallback and recovery

If semantic targeting is unsupported or cannot identify the control, use advertised capture and native input capabilities with fresh visual evidence. Bring the exact app/window to the foreground before native keyboard or pointer input, and verify focus. Browser semantic actions need not steal focus.

Inspect structured failure and dispatch state before changing approach. An explicit not-dispatched failure permits correcting the target; an uncertain action requires checking that original operation and its effect first. Do not repeat clicks or form submission merely because a timeout occurred. Do not use another control route to bypass a permission denial.

## Secrets and completion

Use Mechanize's private credential preparation/status/apply/submit flow when sign-in is required, following the retrieved live guide. Secrets remain in the local Vault; never read them into prompts, tool arguments, screenshots, logs, or clipboard output. Do not weaken protection to inspect a login form.

Report what was verified, what remains pending, and any unresolved effect. Close your owned automation session when the task is complete; preserve user tabs and unsent drafts unless the user requested otherwise.
