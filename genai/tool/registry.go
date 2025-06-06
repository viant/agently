package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/agently/genai/llm"
	mcpservice "github.com/viant/fluxor-mcp/mcp"
	mcpschema "github.com/viant/mcp-protocol/schema"
	"io"
)

// ---------------------------------------------------------------------------
// Global MCP service pointer – set once by the executor during bootstrap so
// that Registry calls can transparently proxy to the shared tool registry
// managed by github.com/viant/fluxor-mcp/mcp.Service.
// ---------------------------------------------------------------------------

var globalMcpSvc *mcpservice.Service

// SetMCPService stores the orchestrating MCP service so that subsequent calls
// to registry helpers can delegate tool look-ups and executions. It is safe to
// invoke the function more than once – the last non-nil pointer wins.
func SetMCPService(svc *mcpservice.Service) {
	if svc != nil {
		globalMcpSvc = svc
	}
}

// Handler executes a tool call and returns its textual result.
type Handler func(ctx context.Context, args map[string]interface{}) (string, error)

// Registry provides a backward-compatibility wrapper that replicates the old
// in-process tool registry API while internally delegating to the unified tool
// catalogue owned by fluxor-mcp.  Any tools that are *manually* registered via
// Register() are kept in an overlay map so that legacy adapters continue to
// work.
type Registry struct {
	// Manually added entries – preferred over MCP definitions when duplicate
	// names exist (behaviour consistent with the old implementation where the
	// last call to Register won).
	customDefs     map[string]llm.ToolDefinition
	customHandlers map[string]Handler

	debugWriter io.Writer `json:"-"`
}

// --------------------------------------------------------------
// Constructors & helpers
// --------------------------------------------------------------

// NewRegistry returns an empty overlay registry. Callers *must* ensure that
// SetMCPService() has been invoked before they actually use the registry,
// otherwise look-ups will fail with "tool not found".
func NewRegistry() *Registry {
	return &Registry{
		customDefs:     make(map[string]llm.ToolDefinition),
		customHandlers: make(map[string]Handler),
	}
}

// ------------------------------------------------------------------
// Tool metadata helpers
// ------------------------------------------------------------------

// Definitions combines MCP-provided tools with manually registered overlay
// definitions and returns the merged slice.
func (r *Registry) Definitions() []llm.ToolDefinition {
	var defs []llm.ToolDefinition

	// 1. MCP catalogue
	if svc := globalMcpSvc; svc != nil {
		entries := svc.ToolDescriptors()
		for _, d := range entries {
			def := llm.ToolDefinition{
				Name:        d.Name,
				Description: d.Description,
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			}
			if _, schema, ok := svc.ToolMetadata(d.Name); ok {
				if in, ok2 := schema.(mcpschema.ToolInputSchema); ok2 {
					// Convert schema.Properties (map[string]map[string]interface{})
					// to the generic map[string]interface{} expected by LLM code.
					props := make(map[string]interface{}, len(in.Properties))
					for k, v := range in.Properties {
						props[k] = v
					}
					def.Parameters["properties"] = props
					if len(in.Required) > 0 {
						def.Parameters["required"] = in.Required
					}
				}
			}
			defs = append(defs, def)
		}
	}

	// 2. Custom overlay (may replace entries with same name)
	for _, d := range r.customDefs {
		defs = append(defs, d)
	}

	return defs
}

// GetDefinition retrieves a tool definition either from the overlay or the MCP
// catalogue. The second return value indicates presence.
func (r *Registry) GetDefinition(name string) (llm.ToolDefinition, bool) {
	// Overlay takes precedence.
	if def, ok := r.customDefs[name]; ok {
		return def, true
	}

	if svc := globalMcpSvc; svc != nil {
		desc, schemaRaw, ok := svc.ToolMetadata(name)
		if !ok {
			return llm.ToolDefinition{}, false
		}

		def := llm.ToolDefinition{
			Name:        name,
			Description: desc,
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		}

		if schema, ok2 := schemaRaw.(mcpschema.ToolInputSchema); ok2 {
			props := make(map[string]interface{}, len(schema.Properties))
			for k, v := range schema.Properties {
				props[k] = v
			}
			def.Parameters["properties"] = props
			if len(schema.Required) > 0 {
				def.Parameters["required"] = schema.Required
			}
		}
		return def, true
	}
	return llm.ToolDefinition{}, false
}

// MustHaveTools converts a list of tool names into the LLM toolkit slice that
// Generation prompts expect. It returns an error when one of the requested
// names does not resolve.
func (r *Registry) MustHaveTools(toolNames []string) ([]llm.Tool, error) {
	var tools []llm.Tool
	for _, name := range toolNames {
		def, ok := r.GetDefinition(name)
		if !ok {
			return nil, fmt.Errorf("tool %q not found", name)
		}
		tools = append(tools, llm.Tool{Type: "function", Definition: def})
	}
	return tools, nil
}

// ------------------------------------------------------------------
// Execution helpers
// ------------------------------------------------------------------

// Execute tries custom handler overlay first, then falls back to MCP internal
// invocation helper.
func (r *Registry) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	if h, ok := r.customHandlers[name]; ok {
		if r.debugWriter != nil {
			fmt.Fprintf(r.debugWriter, "[tool] call %s args=%v (custom)\n", name, args)
		}
		return h(ctx, args)
	}

	if svc := globalMcpSvc; svc != nil {
		if r.debugWriter != nil {
			fmt.Fprintf(r.debugWriter, "[tool] call %s args=%v (mcp)\n", name, args)
		}
		res, err := svc.ExecuteTool(ctx, name, args)
		if err != nil && r.debugWriter != nil {
			fmt.Fprintf(r.debugWriter, "[tool] error %s: %v\n", name, err)
		}
		return res, err
	}

	return "", fmt.Errorf("tool %q not registered and no MCP service configured", name)
}

// ------------------------------------------------------------------
// Overlay registration – kept for backward compatibility (used by tests and
// some adapters). These do NOT modify the MCP catalogue.
// ------------------------------------------------------------------

// Register adds a custom tool definition and handler to the overlay registry.
func (r *Registry) Register(def llm.ToolDefinition, handler Handler) {
	r.customDefs[def.Name] = def
	r.customHandlers[def.Name] = handler
}

// SetDebugLogger attaches a logger receiving every tool invocation.
func (r *Registry) SetDebugLogger(w io.Writer) { r.debugWriter = w }

// ------------------------------------------------------------------
// Default singleton for convenience (mirrors old behaviour).
// ------------------------------------------------------------------

var singleton = NewRegistry()

// Register adds a custom tool definition/handler to the default registry.
func Register(def llm.ToolDefinition, handler Handler) { singleton.Register(def, handler) }

// Definitions returns the merged set from the default registry.
func Definitions() []llm.ToolDefinition { return singleton.Definitions() }

// GetDefinition fetches a single tool definition from the default registry.
func GetDefinition(name string) (llm.ToolDefinition, bool) { return singleton.GetDefinition(name) }

// Execute forwards the call to the default registry.
func Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	return singleton.Execute(ctx, name, args)
}

// MustHaveTools is a convenience wrapper around the default registry.
func MustHaveTools(toolNames []string) ([]llm.Tool, error) { return singleton.MustHaveTools(toolNames) }

// UnmarshalArguments parses a JSON-encoded map.
func UnmarshalArguments(raw json.RawMessage) (map[string]interface{}, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid tool arguments: %w", err)
	}
	return args, nil
}
