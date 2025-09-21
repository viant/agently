package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/viant/agently/adapter/mcp/manager"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/conversation"
	"github.com/viant/agently/genai/llm"
	gtool "github.com/viant/agently/genai/tool"
	authctx "github.com/viant/agently/internal/auth"
	mcpservice "github.com/viant/fluxor-mcp/mcp"
	mcontext "github.com/viant/fluxor-mcp/mcp/context"
	mcptool "github.com/viant/fluxor-mcp/mcp/tool"
)

// Registry bridges an individual fluxor-mcp Service instance to the generic
// tool.Registry interface so that callers can choose dependency injection
// over the global SetMCPService() singleton.
type Registry struct {
	svc         *mcpservice.Service
	debugWriter io.Writer

	// virtual tool overlay (id → definition)
	virtualDefs map[string]llm.ToolDefinition
	virtualExec map[string]gtool.Handler

	// Optional per-conversation MCP client manager. When set, Execute will
	// inject the appropriate client and auth token into context so that the
	// underlying proxy can use them.
	mgr *manager.Manager
}

// New creates a registry bound to the given orchestration service.
// The pointer must not be nil.
func New(svc *mcpservice.Service) *Registry {
	if svc == nil {
		panic("adapter/tool: nil mcp.Service passed to registry.New")
	}
	return &Registry{
		svc:         svc,
		virtualDefs: map[string]llm.ToolDefinition{},
		virtualExec: map[string]gtool.Handler{},
	}
}

// WithManager attaches a per-conversation MCP manager used to inject the
// appropriate client and auth token into the context at call-time.
func (r *Registry) WithManager(m *manager.Manager) *Registry { r.mgr = m; return r }

// InjectVirtualAgentTools registers synthetic tool definitions that delegate
// execution to another agent. It must be called once during bootstrap *after*
// the agent catalogue is loaded. Domain can be empty to expose all.
func (r *Registry) InjectVirtualAgentTools(agents []*agent.Agent, domain string) {
	for _, ag := range agents {
		if ag == nil || ag.ToolExport == nil || !ag.ToolExport.Expose {
			continue
		}

		// Domain filter when requested
		if len(ag.ToolExport.Domains) > 0 && domain != "" {
			if !contains(ag.ToolExport.Domains, domain) {
				continue
			}
		}

		service := ag.ToolExport.Service
		if service == "" {
			service = "agentExec"
		}
		method := ag.ToolExport.Method
		if method == "" {
			method = ag.ID
		}

		toolID := fmt.Sprintf("%s/%s", service, method)

		// Build parameter schema once
		params := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"objective": map[string]interface{}{
					"type":        "string",
					"description": "Concise goal for the agent",
				},
				"context": map[string]interface{}{
					"type":        "object",
					"description": "Optional shared context",
				},
			},
			"required": []string{"objective"},
		}

		def := llm.ToolDefinition{
			Name:        toolID,
			Description: fmt.Sprintf("Executes the \"%s\" agent – %s", ag.Name, strings.TrimSpace(ag.Description)),
			Parameters:  params,
			Required:    []string{"objective"},
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"answer": map[string]interface{}{"type": "string"},
				},
			},
		}

		// Handler closure captures agent pointer
		handler := func(ctx context.Context, args map[string]interface{}) (string, error) {
			// Merge agentId into args for downstream executor
			if args == nil {
				args = map[string]interface{}{}
			}
			args["agentId"] = ag.ID

			// Reuse underlying MCP service to execute llm/exec:run_agent
			result, err := r.Execute(ctx, "llm/exec:run_agent", args)
			return result, err
		}

		r.virtualDefs[toolID] = def
		r.virtualExec[toolID] = handler
	}
}

func contains(arr []string, s string) bool {
	for _, v := range arr {
		if v == s {
			return true
		}
	}
	return false
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

	// 2. virtual agent tools
	for _, def := range r.virtualDefs {
		defs = append(defs, def)
	}
	return defs
}

func (r *Registry) MatchDefinition(pattern string) []*llm.ToolDefinition {
	mcpTools := r.svc.MatchTools(pattern)
	var result []*llm.ToolDefinition

	// virtual first (simple contains or wildcard match?) Use strings.HasSuffix? We'll simple exact match or '*' wildcard.
	for id, def := range r.virtualDefs {
		matched := false
		if pattern == id {
			matched = true
		} else if strings.Contains(pattern, "*") {
			// naive glob: replace * with anything
			glob := strings.ReplaceAll(pattern, "*", "")
			if strings.HasPrefix(id, glob) {
				matched = true
			}
		}
		if matched {
			copyDef := def
			result = append(result, &copyDef)
		}
	}
	for _, tool := range mcpTools {
		def := llm.ToolDefinitionFromMcpTool(&tool.Metadata)
		result = append(result, def)
	}
	return result
}

func (r *Registry) GetDefinition(name string) (*llm.ToolDefinition, bool) {
	if def, ok := r.virtualDefs[name]; ok {
		return &def, true
	}
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
	// virtual tool?
	if h, ok := r.virtualExec[name]; ok {
		return h(ctx, args)
	}
	if r.debugWriter != nil {
		fmt.Fprintf(r.debugWriter, "[adapter/tool] call %s args=%v (mcp)\n", name, args)
	}

	// Inject per-conversation MCP client and token into context for proxy.
	if r.mgr != nil {
		convID := conversation.ID(ctx)
		// Infer server (service name) from tool name using fluxor-mcp naming.
		server := mcptool.Name(mcptool.Canonical(name)).Service()
		if server != "" && convID != "" {
			if cli, err := r.mgr.Get(ctx, convID, server); err == nil && cli != nil {
				ctx = mcontext.WithClient(ctx, cli)
				// Optionally attach token from context if present (HTTP layer).
				if tok := authctx.Bearer(ctx); tok != "" {
					ctx = mcontext.WithAuthToken(ctx, tok)
				}
				// Update last-used timestamp after the call completes.
				defer r.mgr.Touch(convID, server)
			}
		}
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
