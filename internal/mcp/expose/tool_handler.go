package expose

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/internal/tool/matcher"
	"github.com/viant/agently/pkg/mcpname"
	"github.com/viant/jsonrpc"
	mcpschema "github.com/viant/mcp-protocol/schema"
)

// ToolHandler exposes executor tool registry via MCP tools/list and tools/call.
type ToolHandler struct {
	exec     *executor.Service
	patterns []string
}

func NewToolHandler(exec *executor.Service, patterns []string) *ToolHandler {
	return &ToolHandler{exec: exec, patterns: append([]string(nil), patterns...)}
}

// ---------------- mcp-protocol/server.Operations ----------------

func (h *ToolHandler) Initialize(_ context.Context, _ *mcpschema.InitializeRequestParams, _ *mcpschema.InitializeResult) {
}

func (h *ToolHandler) ListResources(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.ListResourcesRequest]) (*mcpschema.ListResourcesResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("resources/list not implemented", nil)
}

func (h *ToolHandler) ListResourceTemplates(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.ListResourceTemplatesRequest]) (*mcpschema.ListResourceTemplatesResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("resources/templates/list not implemented", nil)
}

func (h *ToolHandler) ReadResource(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.ReadResourceRequest]) (*mcpschema.ReadResourceResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("resources/read not implemented", nil)
}

func (h *ToolHandler) Subscribe(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.SubscribeRequest]) (*mcpschema.SubscribeResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("subscribe not implemented", nil)
}

func (h *ToolHandler) Unsubscribe(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.UnsubscribeRequest]) (*mcpschema.UnsubscribeResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("unsubscribe not implemented", nil)
}

func (h *ToolHandler) ListTools(ctx context.Context, _ *jsonrpc.TypedRequest[*mcpschema.ListToolsRequest]) (*mcpschema.ListToolsResult, *jsonrpc.Error) {
	defs, err := h.allowedDefinitions(ctx)
	if err != nil {
		return nil, jsonrpc.NewInternalError(err.Error(), nil)
	}
	tools := make([]mcpschema.Tool, 0, len(defs))
	for i := range defs {
		tool := mcpToolFromDefinition(&defs[i])
		if tool == nil {
			continue
		}
		tools = append(tools, *tool)
	}
	return &mcpschema.ListToolsResult{Tools: tools}, nil
}

func (h *ToolHandler) CallTool(ctx context.Context, req *jsonrpc.TypedRequest[*mcpschema.CallToolRequest]) (*mcpschema.CallToolResult, *jsonrpc.Error) {
	if req == nil || req.Request == nil {
		return nil, jsonrpc.NewInvalidRequest("missing request", nil)
	}
	rawName := strings.TrimSpace(req.Request.Params.Name)
	if rawName == "" {
		return nil, jsonrpc.NewInvalidRequest("missing tool name", nil)
	}

	defs, err := h.allowedDefinitions(ctx)
	if err != nil {
		return nil, jsonrpc.NewInternalError(err.Error(), nil)
	}

	name, ok, resolveErr := resolveToolName(rawName, defs)
	if resolveErr != nil {
		return nil, jsonrpc.NewInvalidRequest(resolveErr.Error(), nil)
	}
	if !ok {
		return nil, mcpschema.NewUnknownTool(rawName)
	}

	out, execErr := h.exec.ExecuteTool(ctx, name, req.Request.Params.Arguments, 0)
	if execErr != nil {
		msg := execErr.Error()
		isErr := true
		return &mcpschema.CallToolResult{IsError: &isErr, Content: []mcpschema.CallToolResultContentElem{{Type: "text", Text: msg}}}, nil
	}

	text := marshalText(out)
	var structured map[string]interface{}
	_ = json.Unmarshal([]byte(text), &structured)

	res := &mcpschema.CallToolResult{
		Content: []mcpschema.CallToolResultContentElem{{Type: "text", Text: text}},
	}
	if structured != nil {
		res.StructuredContent = structured
	}
	return res, nil
}

func (h *ToolHandler) ListPrompts(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.ListPromptsRequest]) (*mcpschema.ListPromptsResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("prompts/list not implemented", nil)
}

func (h *ToolHandler) GetPrompt(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.GetPromptRequest]) (*mcpschema.GetPromptResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("prompts/get not implemented", nil)
}

func (h *ToolHandler) Complete(_ context.Context, _ *jsonrpc.TypedRequest[*mcpschema.CompleteRequest]) (*mcpschema.CompleteResult, *jsonrpc.Error) {
	return nil, jsonrpc.NewMethodNotFound("complete not implemented", nil)
}

// ---------------- mcp-protocol/server.Handler ----------------

func (h *ToolHandler) OnNotification(_ context.Context, _ *jsonrpc.Notification) {}

func (h *ToolHandler) Implements(method string) bool {
	switch method {
	case mcpschema.MethodToolsList, mcpschema.MethodToolsCall:
		return true
	default:
		return false
	}
}

// ---------------- helpers ----------------

func (h *ToolHandler) allowedDefinitions(_ context.Context) ([]llm.ToolDefinition, error) {
	if h == nil || h.exec == nil || h.exec.LLMCore() == nil {
		return nil, fmt.Errorf("mcp server: executor not initialised")
	}
	defs := h.exec.LLMCore().ToolDefinitions()
	if len(h.patterns) == 0 {
		return nil, nil
	}
	var out []llm.ToolDefinition
	for i := range defs {
		if toolAllowed(h.patterns, defs[i].Name) {
			out = append(out, defs[i])
		}
	}
	return out, nil
}

func toolAllowed(patterns []string, name string) bool {
	for _, p := range patterns {
		if matcher.Match(p, name) {
			return true
		}
	}
	return false
}

func resolveToolName(raw string, defs []llm.ToolDefinition) (string, bool, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", false, fmt.Errorf("missing tool name")
	}

	// Full name fast-path: list tools returns names like "service/path:method".
	if strings.ContainsAny(name, "/:-.") {
		for i := range defs {
			if defs[i].Name == name {
				return defs[i].Name, true, nil
			}
			if strings.EqualFold(defs[i].Name, name) {
				return defs[i].Name, true, nil
			}
		}
		return "", false, nil
	}

	// Method-only fallback: match a single allowed tool by method name.
	var matched string
	for i := range defs {
		can := mcpname.Canonical(defs[i].Name)
		method := mcpname.Name(can).Method()
		if strings.EqualFold(method, name) {
			if matched != "" && matched != defs[i].Name {
				return "", false, fmt.Errorf("ambiguous tool %q: provide fully qualified name", raw)
			}
			matched = defs[i].Name
		}
	}
	if matched == "" {
		return "", false, nil
	}
	return matched, true, nil
}

func marshalText(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return t
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", v)
	}
}

func mcpToolFromDefinition(def *llm.ToolDefinition) *mcpschema.Tool {
	if def == nil {
		return nil
	}
	normalized := *def
	normalized.Normalize()

	desc := normalized.Description
	inProps := extractProperties(normalized.Parameters["properties"])
	outProps := extractProperties(normalized.OutputSchema["properties"])

	return &mcpschema.Tool{
		Name:        normalized.Name,
		Description: &desc,
		InputSchema: mcpschema.ToolInputSchema{
			Type:       "object",
			Properties: mcpschema.ToolInputSchemaProperties(inProps),
			Required:   normalized.Required,
		},
		OutputSchema: &mcpschema.ToolOutputSchema{
			Type:       "object",
			Properties: outProps,
		},
	}
}

func extractProperties(v interface{}) map[string]map[string]interface{} {
	switch m := v.(type) {
	case nil:
		return map[string]map[string]interface{}{}
	case map[string]interface{}:
		out := make(map[string]map[string]interface{}, len(m))
		for k, val := range m {
			if vv, ok := val.(map[string]interface{}); ok && vv != nil {
				out[k] = vv
				continue
			}
			out[k] = map[string]interface{}{}
		}
		return out
	case map[string]map[string]interface{}:
		return m
	case mcpschema.ToolInputSchemaProperties:
		out := make(map[string]map[string]interface{}, len(m))
		for k, vv := range m {
			out[k] = vv
		}
		return out
	default:
		return map[string]map[string]interface{}{}
	}
}
