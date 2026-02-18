# MCP Validation With Official Inspector

## 1) Start Inspector
```bash
npx @modelcontextprotocol/inspector
```

## 2) Run Server for Inspector
```bash
# stdio mode
MCP_MODE=stdio go run ./cmd/server

# streamable HTTP mode
MCP_MODE=http go run ./cmd/server
```

## 3) Required Method Sequence
1. `initialize`
2. `tools/list`
3. `tools/call`
4. `resources/list`
5. `resources/read`
6. `resources/templates/list`
7. `prompts/list`
8. `prompts/get`

## 4) Example JSON-RPC Payloads
```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"inspector","version":"1.0.0"},"capabilities":{}}}
```

```json
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
```

```json
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"message":"hello"}}}
```

## 5) Compatibility Check
Test two client protocol versions:
- latest supported
- one older supported

Expected:
- server downgrades to compatible lower version
- newer-only features are hidden at lower negotiated version

## 6) Pass Criteria
- no method-not-found for advertised methods
- no hangs for unsupported optional capabilities
- result shape always explicit (`content`, `structuredContent`, `isError`)

## 7) OOB Elicitation Validation (required for protected tools)
1. Call a protected tool without token.
2. Confirm response indicates auth required and includes/points to OOB URL.
3. Open OOB URL in browser and complete auth.
4. Re-run same tool call.
5. Confirm success without additional prompt flood.

Example protected call:
```json
{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"repo_read","arguments":{"owner":"acme","repo":"private"}}}
```

Expected sequence:
- first response: `isError=true` or explicit auth-needed text/metadata
- post-callback response: normal success payload
- parallel retries during auth: no duplicate OOB flows for same namespace/scope
