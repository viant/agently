# Public non-Agently AG-UI interoperability proof

`public-langgraph-session.ts` exercises the production SDK's `AgUiSession` against the official public AG-UI LangGraph Dojo FastAPI service. It is a real remote LangGraph agent using the published AG-UI integration, not a synthetic SSE fixture or a locally simulated backend.

The reviewed upstream [`render.yaml`](https://github.com/ag-ui-protocol/ag-ui/blob/97f789cc1c48eaaf6949c9e9a88abb5cf6e7c5e3/render.yaml) declares `https://ag-ui-dojo-langgraph-fastapi.onrender.com`. The deployed OpenAPI document identifies `LangGraph Dojo Example Server`, version `0.1.0`, and the actual POST route `/agent/agentic_chat`. The inspected [`agentic_chat` source](https://github.com/ag-ui-protocol/ag-ui/blob/97f789cc1c48eaaf6949c9e9a88abb5cf6e7c5e3/integrations/langgraph/python/examples/agents/agentic_chat/agent.py) uses LangGraph `create_agent`, `CopilotKitMiddleware`, and `openai:gpt-4.1-mini`.

The source reference is pinned to `97f789cc1c48eaaf6949c9e9a88abb5cf6e7c5e3`. The hosted service does not expose its deployment Git revision, so the evidence records `deploymentRevision: null`; the source reference is not presented as a proven deployed revision. No fallback self-hosted backend was needed.

Run from the assembly checkout:

```sh
bash dev/ag-ui/interop/run-public-langgraph-session.sh
```

The runner resolves the sibling `agently-core-ag-ui/sdk/ts` checkout and its existing `vite-node` runtime. Install that SDK's normal dependencies first if they are missing. The script exits nonzero when any assertion fails and writes `evidence/public-langgraph-session.json`, including timestamp, reviewed deployment/source references, current OpenAPI version, request metadata, event counts, and explicit acceptance assertions.

The harness sends only synthetic markers and a harmless local frontend tool. Its fetch boundary restricts requests to the reviewed public demo origin, sets `credentials: 'omit'`, allows only Accept and Content-Type headers, and rejects redirects. It does not load credentials, application conversation data, session cookies, or authentication configuration. No Agently discovery or extension envelope is sent. Each remote request has a 45-second timeout and no automatic execution retries.

The observed run passed all of the following:

- A standard streamed initial chat returns `AGUI_WEB_INTEROP_OK`.
- A follow-up in the same thread returns the previous marker.
- A advertised frontend tool receives a single complete JSON argument set and executes exactly once locally.
- Its tool result is returned in the next standard run, producing `AGUI_WEB_INTEROP_TOOL_OK`.
- Four real protocol runs complete with text deltas, message snapshots, state snapshots, and tool-call start/args/end events, reduced by the official upstream client.
- The session refuses durable replay because this public backend has not advertised that capability.

The current evidence contains 4 `RUN_STARTED`, 4 `RUN_FINISHED`, 10 `STATE_SNAPSHOT`, 4 `MESSAGES_SNAPSHOT`, and one complete frontend tool handoff. Exact text chunk counts may vary with the live model. The evidence intentionally records `productionUI.tested: false`: this portable SDK/session proof prepares the same production web UI browser acceptance run, but does not substitute for it. Production browser proof must use this actual endpoint through the approved connection boundary and record the rendered chat/tool experience, network routing, and BFF authentication preservation separately. Interrupt/resume and durable restart/replay are not claimed for this public demo by this harness.

## Authenticated BFF public-demo proof

The optional startup configuration below enables only an explicitly authorized
public demo. Use an isolated workspace and synthetic prompts. This is a text-only,
ephemeral interoperability connection, not production downstream OAuth or durable
external history. Omitted configuration enables no remote endpoints.

```yaml
agUI:
  demoBackends:
    - id: langgraph
      label: Public LangGraph demo
      url: https://ag-ui-dojo-langgraph-fastapi.onrender.com/agent/agentic_chat
      allowedSubjects:
        - <exact-authenticated-subject>
```

The BFF issues a fresh remote thread through
`POST /v1/ag-ui/backends/langgraph/threads`; all streaming requests then use
`POST /v1/ag-ui/backends/langgraph/run`. Cookie/auth/debug headers are not
forwarded. Redirects and repeated run submissions are rejected. Threads expire
and become unavailable after BFF restart; capabilities advertise no durable replay.

`bff-public-langgraph-session.ts` uses the existing SDK BFF session API with a
private, mode-0600 token bundle produced by authorized Scy OOB login. It restricts
credential-bearing client requests to the owned loopback BFF and never prints
cookies or tokens. From the core SDK `sdk/ts` directory:

```sh
AGUI_BFF_TOKEN_FILE=/private/path/to/test-token-bundle.json \
  node ./node_modules/vite-node/vite-node.mjs \
  ../../../agently-ag-ui/dev/ag-ui/interop/bff-public-langgraph-session.ts
```

Set `AGUI_BFF_ORIGIN` for the owned loopback backend port. The token bundle uses
Scy SessionTokenPayload fields `AccessToken`, `IDToken`, `RefreshToken`,
`ExpiresAt`; remove the temporary bundle after testing. The recorded run passed
session creation, authenticated backend discovery, server-issued thread creation,
two real streamed public LangGraph turns, and unchanged BFF identity afterward.
`evidence/bff-public-langgraph-session.json` records only sanitized results and
still explicitly marks the production UI browser gate as untested.
