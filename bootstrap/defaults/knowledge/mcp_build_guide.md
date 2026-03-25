# Build Guide: MCP Servers With Multiple Capabilities

## 1) Project Shape
- `usecase/`: business logic
- `server/`: MCP registration and handlers
- `cmd/server/`: runtime + transport

## 2) Minimal Go Server (tools)
```go
package server

import (
	"context"
	"fmt"

	"github.com/viant/mcp/server"
	proto "github.com/viant/mcp-protocol/protocol"
	"github.com/viant/mcp-protocol/schema"
)

type EchoIn struct { Message string `json:"message"` }
type EchoOut struct { Echo string `json:"echo"` }

func New() (*server.Service, error) {
	h := proto.WithDefaultHandler(proto.NewHandler())

	if err := proto.RegisterTool[EchoIn, EchoOut](h, "echo", "Echo text", func(ctx context.Context, in *EchoIn, _ proto.CallToolOperations) (*schema.CallToolResult, error) {
		if in.Message == "" {
			return &schema.CallToolResult{IsError: true, Content: []schema.Content{schema.TextContent{Type: "text", Text: "message is required"}}}, nil
		}
		out := EchoOut{Echo: in.Message}
		return &schema.CallToolResult{
			StructuredContent: out,
			Content:           []schema.Content{schema.TextContent{Type: "text", Text: fmt.Sprintf("echo=%s", out.Echo)}},
		}, nil
	}); err != nil {
		return nil, err
	}

	return server.New(server.WithNewHandler(func() (proto.Handler, error) { return h, nil }))
}
```

## 3) Add Resources
```go
h.RegisterResource(schema.Resource{
	URI:         "kb://status",
	Name:        "Status",
	Description: "Current service status",
	MimeType:    "application/json",
}, func(ctx context.Context, req *schema.ReadResourceRequestParams) (*schema.ReadResourceResult, error) {
	return &schema.ReadResourceResult{Contents: []schema.ResourceContents{
		schema.TextResourceContents{URI: req.URI, MimeType: "application/json", Text: `{"ok":true}`},
	}}, nil
})
```

## 4) Add Prompts
```go
h.RegisterPrompts(schema.Prompt{Name: "summarize", Description: "Summarize input"},
	func(ctx context.Context, req *schema.GetPromptRequestParams) (*schema.GetPromptResult, error) {
		topic := req.Arguments["topic"]
		if topic == "" { topic = "general" }
		return &schema.GetPromptResult{Messages: []schema.PromptMessage{{
			Role: "user",
			Content: schema.TextContent{Type: "text", Text: "Summarize: " + topic},
		}}}, nil
	})
```

## 5) Elicitation (missing inputs)
```go
if in.Message == "" {
	res, err := ops.Elicit(ctx, &schema.ElicitationRequestParams{
		Message: "Please provide message",
		RequestedSchema: map[string]any{
			"type": "object",
			"required": []string{"message"},
			"properties": map[string]any{"message": map[string]any{"type": "string"}},
		},
	})
	if err != nil || res.Action != "accept" {
		return &schema.CallToolResult{IsError: true, Content: []schema.Content{schema.TextContent{Type: "text", Text: "message not provided"}}}, nil
	}
	in.Message, _ = res.Content["message"].(string)
}
```

## 6) Sampling Capability Gate
```go
if !ops.ClientCapabilities().Sampling.Enabled {
	return &schema.CallToolResult{IsError: true, Content: []schema.Content{schema.TextContent{Type: "text", Text: "sampling unsupported by client"}}}, nil
}
msg, err := ops.Client().CreateMessage(ctx, &schema.CreateMessageRequestParams{/* ... */})
```

## 7) OOB Elicitation Pattern (browser completion)
Use this when tool execution needs auth/user action outside MCP channel.

```go
type Pending struct {
	ID        string
	Namespace string
	CreatedAt time.Time
}

var (
	pendingMu sync.Mutex
	pending   = map[string]Pending{}
	waiters   = map[string]chan struct{} // key: namespace
)

func startOOB(ns string) string {
	id := uuid.NewString()
	pendingMu.Lock()
	pending[id] = Pending{ID: id, Namespace: ns, CreatedAt: time.Now()}
	if _, ok := waiters[ns]; !ok {
		waiters[ns] = make(chan struct{})
	}
	pendingMu.Unlock()
	return "http://127.0.0.1:8081/oob/start?pending=" + url.QueryEscape(id)
}

func waitForOOB(ctx context.Context, ns string) error {
	pendingMu.Lock()
	ch := waiters[ns]
	pendingMu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
		return nil
	}
}
```

Tool-side usage:
```go
if !hasToken(ns) {
	url := startOOB(ns)
	_, _ = ops.Elicit(ctx, &schema.ElicitationRequestParams{
		Message: "Authentication required",
		RequestedSchema: map[string]any{
			"type": "object",
			"required": []string{"url"},
			"properties": map[string]any{"url": map[string]any{"type": "string", "const": url}},
		},
	})
	if err := waitForOOB(ctx, ns); err != nil {
		return &schema.CallToolResult{IsError: true, Content: []schema.Content{schema.TextContent{Type: "text", Text: "oob auth timeout/cancel"}}}, nil
	}
}
```

HTTP callback completion:
```go
func oobCallback(w http.ResponseWriter, r *http.Request) {
	pid := r.URL.Query().Get("pending")
	token := r.URL.Query().Get("token")
	pendingMu.Lock()
	p, ok := pending[pid]
	if ok {
		setToken(p.Namespace, token)
		delete(pending, pid)
		close(waiters[p.Namespace])        // wake all waiters
		waiters[p.Namespace] = make(chan struct{})
	}
	pendingMu.Unlock()
	w.WriteHeader(http.StatusOK)
}
```

## 8) Transport Boot (stdio + HTTP)
```go
func main() {
	ctx := context.Background()
	srv, _ := server.New()

	if os.Getenv("MCP_MODE") == "stdio" {
		_ = srv.Stdio(ctx).ListenAndServe()
		return
	}

	srv.UseStreamableHTTP(true)
	_ = srv.HTTP(ctx, ":8080", "/mcp").ListenAndServe()
}
```

## 9) Local Replace During Dev
```go
replace github.com/viant/mcp => $GOPATH/github.com/viant/mcp
replace github.com/viant/mcp-protocol => $GOPATH/github.com/viant/mcp-protocol
```

## 10) Version Negotiation Rule
On `initialize`, select highest mutually supported version. If negotiated lower version, do not advertise newer-only capabilities.
