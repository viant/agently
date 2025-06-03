package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/agently/genai/llm"
	"io"
	"strings"
)

// Handler is a function that executes a tool call with given arguments.
// It returns the tool's result as a string.
type Handler func(ctx context.Context, args map[string]interface{}) (string, error)

// Registry holds tool definitions and handlers.
type Registry struct {
	definitions map[string]llm.ToolDefinition
	handlers    map[string]Handler

	debugWriter io.Writer `json:"-"`
}

func (r *Registry) MustHaveTools(toolNames []string) ([]llm.Tool, error) {
	if r == nil {
		r = registry
	}
	var tools []llm.Tool
	for _, toolName := range toolNames {
		def, ok := r.GetDefinition(toolName)
		if !ok {
			return nil, fmt.Errorf("tool %q not found in r", toolName)
		}
		tools = append(tools, llm.Tool{Type: "function", Definition: def})
	}
	return tools, nil
}

// Definitions returns all registered tool definitions in this registry.
func (r *Registry) Definitions() []llm.ToolDefinition {
	defs := make([]llm.ToolDefinition, 0, len(r.definitions))
	for _, def := range r.definitions {
		defs = append(defs, def)
	}
	return defs
}

// NewRegistry creates a new empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		definitions: make(map[string]llm.ToolDefinition),
		handlers:    make(map[string]Handler),
	}
}

// registry is the global singleton registry for backward compatibility.
var registry = NewRegistry()

// Register registers a tool definition and handler in this registry.
func (r *Registry) Register(def llm.ToolDefinition, handler Handler) {
	r.definitions[def.Name] = def
	r.handlers[def.Name] = handler
}

// GetDefinition retrieves a tool definition by name from this registry.
// Returns definition and true if found.
func (r *Registry) GetDefinition(name string) (llm.ToolDefinition, bool) {
	def, ok := r.definitions[name]
	return def, ok
}

// Execute invokes a registered tool handler by name with given args.
// Returns an error if the tool is not registered or handler execution fails.
func (r *Registry) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	if r == nil {
		r = registry
	}
	handler, ok := r.handlers[name]
	if !ok {
		if r.debugWriter != nil {
			fmt.Fprintf(r.debugWriter, "[tool] %s not found\n", name)
		}
		if idx := strings.Index(name, "."); idx > 0 {
			name = name[idx+1:]
			handler, ok = r.handlers[name]
			if ok {
				if r.debugWriter != nil {
					fmt.Fprintf(r.debugWriter, "[tool] call %s args=%v\n", name, args)
				}
				res, err := handler(ctx, args)
				if r.debugWriter != nil {
					if err != nil {
						fmt.Fprintf(r.debugWriter, "[tool] error %s: %v\n", name, err)
					} else {
						fmt.Fprintf(r.debugWriter, "[tool] result %s: %s\n", name, res)
					}
				}
				return res, err
			}

		}

		return "", fmt.Errorf("tool %q not registered", name)
	}
	return handler(ctx, args)
}

// SetDebugLogger attaches a writer that will receive every tool call made via
// this registry (name, args, result, error). Passing nil disables logging.
func (r *Registry) SetDebugLogger(w io.Writer) {
	r.debugWriter = w
}

// Register registers a tool definition and handler in the default global registry.
func Register(def llm.ToolDefinition, handler Handler) {
	registry.Register(def, handler)
}

// Definitions returns all registered tool definitions from the default registry.
func Definitions() []llm.ToolDefinition {
	return registry.Definitions()
}

// GetDefinition retrieves a tool definition by name from the default registry.
// Returns definition and true if found.
func GetDefinition(name string) (llm.ToolDefinition, bool) {
	return registry.GetDefinition(name)
}

// Execute invokes a registered tool handler by name from the default registry.
// Returns an error if the tool is not registered or handler fails.
func Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	return registry.Execute(ctx, name, args)
}

// UnmarshalArguments helps parse JSON-encoded arguments into a map.
func UnmarshalArguments(raw json.RawMessage) (map[string]interface{}, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid tool arguments: %w", err)
	}
	return args, nil
}
