package localclient

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/viant/jsonrpc"
	"github.com/viant/jsonrpc/transport"
	mcpclientproto "github.com/viant/mcp-protocol/client"
	mcplogger "github.com/viant/mcp-protocol/logger"
	mcpschema "github.com/viant/mcp-protocol/schema"
	mcpserverproto "github.com/viant/mcp-protocol/server"
	mcpclient "github.com/viant/mcp/client"
	mcpserver "github.com/viant/mcp/server"

	mcpadapter "github.com/viant/agently/genai/tool/adapter/mcp"
	svc "github.com/viant/agently/genai/tool/service"
)

// serviceHandler adapts a genai Service to an MCP server handler implementing tools/list and tools/call.
type serviceHandler struct {
	service svc.Service
	methods map[string]svc.Signature // method name → signature (preserve case)
	tools   []mcpschema.Tool
}

// NewServiceClient returns an mcp client.Interface exposing the given service methods.
func NewServiceClient(ctx context.Context, s svc.Service) (mcpclient.Interface, error) {
	if s == nil {
		return nil, fmt.Errorf("local mcp: nil service")
	}
	h := &serviceHandler{service: s}
	h.init()
	// Build in-process server and expose as a client via adapter
	srv, err := mcpserver.New(mcpserver.WithNewHandler(func(_ context.Context, _ transport.Notifier, _ mcplogger.Logger, _ mcpclientproto.Operations) (mcpserverproto.Handler, error) {
		return h, nil
	}))
	if err != nil {
		return nil, err
	}
	return srv.AsClient(ctx), nil
}

func (h *serviceHandler) init() {
	h.methods = make(map[string]svc.Signature)
	for _, sig := range h.service.Methods() {
		h.methods[sig.Name] = sig
	}
	h.tools = mcpadapter.FromService(h.service) // preserve original Names
}

// ---------------- mcp-protocol/server.Operations ----------------

func (h *serviceHandler) Initialize(_ context.Context, _ *mcpschema.InitializeRequestParams, _ *mcpschema.InitializeResult) {
}

func (h *serviceHandler) ListResources(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.ListResourcesRequest]) (*mcpschema.ListResourcesResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("resources/list not implemented", nil)
}

func (h *serviceHandler) ListResourceTemplates(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.ListResourceTemplatesRequest]) (*mcpschema.ListResourceTemplatesResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("resources/templates/list not implemented", nil)
}

func (h *serviceHandler) ReadResource(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.ReadResourceRequest]) (*mcpschema.ReadResourceResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("resources/read not implemented", nil)
}

func (h *serviceHandler) Subscribe(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.SubscribeRequest]) (*mcpschema.SubscribeResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("subscribe not implemented", nil)
}

func (h *serviceHandler) Unsubscribe(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.UnsubscribeRequest]) (*mcpschema.UnsubscribeResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("unsubscribe not implemented", nil)
}

func (h *serviceHandler) ListTools(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.ListToolsRequest]) (*mcpschema.ListToolsResult, *jsonrpc.Error) {
	// Single page listing of all methods
	return &mcpschema.ListToolsResult{Tools: h.tools}, nil
}

func (h *serviceHandler) CallTool(ctx context.Context, req *jsonrpc.TypedRequest[*mcpschema.CallToolRequest]) (*mcpschema.CallToolResult, *jsonrpc.Error) {
	if req == nil || req.Request == nil {
		return nil, jsonrpc.NewInvalidRequest("missing request", nil)
	}
	name := strings.TrimSpace(req.Request.Params.Name)
	// Accept service/method or plain method; use last token after ':' or '/'
	if i := strings.LastIndexAny(name, ":/"); i != -1 {
		name = name[i+1:]
	}
	// Exact match first, then case-insensitive fallback
	sig, ok := h.methods[name]
	if !ok {
		for k, v := range h.methods {
			if strings.EqualFold(k, name) {
				sig, ok = v, true
				break
			}
		}
	}
	if !ok {
		return nil, mcpschema.NewUnknownTool(name)
	}
	exec, err := h.service.Method(sig.Name)
	if err != nil || exec == nil {
		return nil, jsonrpc.NewInternalError(fmt.Sprintf("method %s resolve: %v", sig.Name, err), nil)
	}
	// Build input/output based on signature types
	var inVal interface{}
	if sig.Input != nil {
		inVal = reflect.New(indirectType(sig.Input)).Interface()
	} else {
		inVal = &struct{}{}
	}
	if len(req.Request.Params.Arguments) > 0 {
		raw, err := json.Marshal(req.Request.Params.Arguments)
		if err != nil {
			return nil, jsonrpc.NewInvalidParamsError(fmt.Sprintf("unable to marshal tool arguments %q due to: %v", name, err), nil)
		}
		err = json.Unmarshal(raw, inVal)
		if err != nil {
			return nil, jsonrpc.NewInvalidParamsError(fmt.Sprintf("unable to unmarshal tool input %q due to: %v", name, err), nil)
		}
	}
	var outVal interface{}
	if sig.Output != nil {
		outVal = reflect.New(indirectType(sig.Output)).Interface()
	} else {
		outVal = &struct{}{}
	}
	if err := exec(ctx, inVal, outVal); err != nil {
		msg := err.Error()
		isErr := true
		return &mcpschema.CallToolResult{IsError: &isErr, Content: []mcpschema.CallToolResultContentElem{{Type: "text", Text: msg}}}, nil
	}
	var (
		structured map[string]interface{}
		textJSON   string
	)
	if b, err := json.Marshal(outVal); err == nil {
		textJSON = string(b)
		_ = json.Unmarshal(b, &structured)
	}
	// Per MCP guidance: when returning structured content, also include a text block
	// with the serialized JSON for compatibility with clients that only read content.
	return &mcpschema.CallToolResult{
		StructuredContent: structured,
		Content:           []mcpschema.CallToolResultContentElem{{Type: "text", Text: textJSON}},
	}, nil
}

func (h *serviceHandler) ListPrompts(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.ListPromptsRequest]) (*mcpschema.ListPromptsResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("prompts/list not implemented", nil)
}

func (h *serviceHandler) GetPrompt(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.GetPromptRequest]) (*mcpschema.GetPromptResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("prompts/get not implemented", nil)
}

func (h *serviceHandler) Complete(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.CompleteRequest]) (*mcpschema.CompleteResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("complete not implemented", nil)
}

// ---------------- mcp-protocol/server.Handler ----------------

func (h *serviceHandler) OnNotification(_ context.Context, _ *jsonrpc.Notification) {}

func (h *serviceHandler) Implements(method string) bool {
	switch method {
	case mcpschema.MethodToolsList, mcpschema.MethodToolsCall:
		return true
	default:
		return false
	}
}

// ---------------- helpers ----------------

func indirectType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}
