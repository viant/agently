# WebDriver Testing For MCP Inspector

## 1) Goal
Automate Inspector UI so MCP validation is repeatable across servers and transports.

## 2) Modes
- Direct HTTP/SSE or streamable HTTP
- Stdio server command
- Stdio bridge to HTTP backend

## 3) Example WebDriver Flow (Go pseudo)
```go
// 1) open inspector page
// 2) fill transport + command/url fields
// 3) click connect
// 4) call initialize/tools/resources/prompts
// 5) assert no UI error and expected payload fields
```

## 4) Using browser-mcp (tooling style)
```bash
# start browser MCP server
$GOPATH/github.com/viant/mcp-toolbox/browser/cmd/browser-mcp/browser-mcp -a ':5002' --headful
```

Then drive these actions through WebDriver calls:
- navigate to Inspector URL
- fill connection form
- click connect
- execute method actions
- capture screenshot/console on failures

## 5) Bridge Mode Example
```bash
# backend server on :8080/mcp
go run $GOPATH/github.com/viant/mcp/bridge -u http://127.0.0.1:8080/mcp --elicit --elicit-listen 127.0.0.1:0
```

Use Inspector in stdio mode and set command to the bridge binary.

## 6) Assertions
- connected session visible
- initialize returns negotiated version
- tools/resources/templates/prompts all callable
- protected flow triggers auth/elicitation then succeeds after completion
- no duplicate parallel OOB prompts under concurrent calls

## 7) OOB WebDriver Scenario (scripted)
```go
// Trigger protected tool -> expect auth-required marker.
runTool("repo_read", map[string]any{"owner":"acme","repo":"private"})
assertContains(lastResponse(), "auth")

// Open OOB URL in second tab/window and submit callback form.
open(oobURLFromResponse())
completeLoginForm()
assertContains(pageText(), "success")

// Return to Inspector and retry tool.
switchToInspector()
runTool("repo_read", map[string]any{"owner":"acme","repo":"private"})
assertNotContains(lastResponse(), "auth required")
assertContains(lastResponse(), "\"content\"")
```

Concurrency check:
- start 3 protected calls in parallel for same namespace
- assert only one OOB URL/pending flow is created
- assert all 3 calls complete after callback

## 8) CI Output
Store per profile:
- transport mode
- client protocol version
- methods executed
- PASS/FAIL
- failure reason + screenshot path
