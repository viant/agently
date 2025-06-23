package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/viant/agently/genai/llm"
	gtool "github.com/viant/agently/genai/tool"
	mcpservice "github.com/viant/fluxor-mcp/mcp"
)

// Registry bridges an individual fluxor-mcp Service instance to the generic
// tool.Registry interface so that callers can choose dependency injection
// over the global SetMCPService() singleton.
type Registry struct {
	svc         *mcpservice.Service
	debugWriter io.Writer
}

// New creates a registry bound to the given orchestration service.
// The pointer must not be nil.
func New(svc *mcpservice.Service) *Registry {
	if svc == nil {
		panic("adapter/tool: nil mcp.Service passed to registry.New")
	}
	return &Registry{
		svc: svc,
	}
}

// ---------------------------------------------------------------------------
// tool.Registry interface implementation
// ---------------------------------------------------------------------------

func (r *Registry) Definitions() []llm.ToolDefinition {
	var defs []llm.ToolDefinition
	// 1. catalogue from MCP service
	for _, d := range r.svc.Tools() {
		def := llm.ToolDefinitionFromMcpTool(&d.Metadata)
		defs = append(defs, *def)
	}
	return defs
}

func (r *Registry) MatchDefinition(pattern string) []*llm.ToolDefinition {
	mcpTools := r.svc.MatchTools(pattern)
	var result []*llm.ToolDefinition
	for _, tool := range mcpTools {
		def := llm.ToolDefinitionFromMcpTool(&tool.Metadata)
		result = append(result, def)
	}
	return result
}

func (r *Registry) GetDefinition(name string) (*llm.ToolDefinition, bool) {
	mcpTool, err := r.svc.LookupTool(name)
	if err != nil {
		return nil, false
	}
	tool := llm.ToolDefinitionFromMcpTool(&mcpTool.Metadata)
	return tool, true
}

func (r *Registry) MustHaveTools(patterns []string) ([]llm.Tool, error) {
	var ret []llm.Tool
	for _, n := range patterns {
		matchedTools := r.MatchDefinition(n)
		if len(matchedTools) == 0 {
			//TODO introduce UI agnostie warning
			fmt.Printf("[WARN] tool %q not found\n", n)
		}
		for _, matchedTool := range matchedTools {
			ret = append(ret, llm.Tool{Type: "function", Definition: *matchedTool})
		}
	}
	return ret, nil
}

func (r *Registry) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	if r.debugWriter != nil {
		fmt.Fprintf(r.debugWriter, "[adapter/tool] call %s args=%v (mcp)\n", name, args)
	}

	// Let the orchestration engine choose its own timeout based on context.
	res, err := r.svc.ExecuteTool(ctx, name, args, time.Duration(15)*time.Minute)
	if err != nil && r.debugWriter != nil {
		fmt.Fprintf(r.debugWriter, "[adapter/tool] error %s: %v\n", name, err)
	}
	var result string
	switch actual := res.(type) {
	case string:
		result = actual
	case []byte:
		result = string(actual)
	default:
		if data, err := json.Marshal(res); err == nil {
			result = string(data)
		} else {
			result = fmt.Sprintf("%v", res)
		}
	}
	return result, err
}

func (r *Registry) SetDebugLogger(w io.Writer) { r.debugWriter = w }

// Verify interface compliance at compile-time.
var _ gtool.Registry = (*Registry)(nil)
