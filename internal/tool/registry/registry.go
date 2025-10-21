package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/viant/afs"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	authctx "github.com/viant/agently/internal/auth"
	"github.com/viant/agently/internal/mcp/manager"
	tmatch "github.com/viant/agently/internal/tool/matcher"
	transform "github.com/viant/agently/internal/transform"
	mcprepo "github.com/viant/agently/internal/workspace/repository/mcp"
	mcpnames "github.com/viant/agently/pkg/mcpname"
	mcpschema "github.com/viant/mcp-protocol/schema"
	mcpclient "github.com/viant/mcp/client"

	svc "github.com/viant/agently/genai/tool/service"
	orchplan "github.com/viant/agently/genai/tool/service/orchestration/plan"
	toolExec "github.com/viant/agently/genai/tool/service/system/exec"
	toolPatch "github.com/viant/agently/genai/tool/service/system/patch"
	localmcp "github.com/viant/agently/internal/mcp/localclient"
	mcpproxy "github.com/viant/agently/internal/mcp/proxy"
)

// Registry bridges per-server MCP tools and internal services to the generic
// tool.Registry interface so that callers can use dependency injection.
type Registry struct {
	debugWriter io.Writer

	// virtual tool overlay (id → definition)
	virtualDefs map[string]llm.ToolDefinition
	virtualExec map[string]Handler

	// Optional per-conversation MCP client manager. When set, Execute will
	// inject the appropriate client and auth token into context so that the
	// underlying proxy can use them.
	mgr *manager.Manager

	// in-memory MCP clients for internal services (server name -> client)
	internal map[string]mcpclient.Interface

	// cache: tool name → entry
	cache map[string]*toolCacheEntry

	warnings []string
}

type toolCacheEntry struct {
	def    llm.ToolDefinition
	mcpDef mcpschema.Tool
	exec   Handler
}

// Handler executes a tool call and returns its textual result.
type Handler func(ctx context.Context, args map[string]interface{}) (string, error)

// NewWithManager creates a registry backed by an MCP client manager.
func NewWithManager(mgr *manager.Manager) (*Registry, error) {
	if mgr == nil {
		return nil, fmt.Errorf("adapter/tool: nil mcp manager passed to NewWithManager")
	}
	r := &Registry{
		virtualDefs: map[string]llm.ToolDefinition{},
		virtualExec: map[string]Handler{},
		mgr:         mgr,
		cache:       map[string]*toolCacheEntry{},
		internal:    map[string]mcpclient.Interface{},
	}
	// Register in-memory MCP clients for built-in services using Service.Name().
	r.addInternalMcp()
	// Register orchestrator synthetic tool.
	r.injectOrchestratorVirtualTool()
	return r, nil
}

// WithManager attaches a per-conversation MCP manager used to inject the
// appropriate client and auth token into the context at call-time.
func (r *Registry) WithManager(m *manager.Manager) *Registry { r.mgr = m; return r }

// InjectVirtualAgentTools registers synthetic tool definitions that delegate
// execution to another agent. It must be called once during bootstrap *after*
// the agent catalogue is loaded. Domain can be empty to expose all.
func (r *Registry) InjectVirtualAgentTools(agents []*agent.Agent, domain string) {
	for _, ag := range agents {
		if ag == nil {
			continue
		}
		// Prefer Profile.Publish to drive exposure; fallback to legacy Directory.Enabled
		if ag.Profile == nil || !ag.Profile.Publish {
			continue
		}

		// Service/method: default to historical values for compatibility
		service := "agentExec"
		method := ag.ID

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

		dispName := strings.TrimSpace(ag.Name)
		if strings.TrimSpace(ag.Profile.Name) != "" {
			dispName = strings.TrimSpace(ag.Profile.Name)
		}
		desc := strings.TrimSpace(ag.Description)
		if strings.TrimSpace(ag.Profile.Description) != "" {
			desc = strings.TrimSpace(ag.Profile.Description)
		}

		def := llm.ToolDefinition{
			Name:        toolID,
			Description: fmt.Sprintf("Executes the \"%s\" agent – %s", dispName, desc),
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

			// Execute via MCP-backed registry
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
	// Only virtual tools by default; server tools are matched on demand.
	for _, def := range r.virtualDefs {
		defs = append(defs, def)
	}
	// Aggregate discovered server tools across all configured MCP clients.
	servers, err := r.listServers(context.Background())
	if err != nil {
		if r.debugWriter != nil {
			fmt.Fprintf(r.debugWriter, "[tool] list servers error: %v\n", err)
		}
		r.warnf("tools: list servers failed: %v", err)
		return defs
	}
	seen := map[string]struct{}{}
	for _, s := range servers {
		tools, err := r.listServerTools(context.Background(), s)
		if err != nil {
			if r.debugWriter != nil {
				fmt.Fprintf(r.debugWriter, "[tool] list tools %s error: %v\n", s, err)
			}
			r.warnf("tools: list %s failed: %v", s, err)
			continue
		}
		for _, t := range tools {
			disp := s + ":" + t.Name
			if _, ok := seen[disp]; ok {
				continue
			}
			seen[disp] = struct{}{}
			if def := llm.ToolDefinitionFromMcpTool(&t); def != nil {
				// For display, prefer service:method; for internal cache, we still
				// track full internal name elsewhere (match/execute paths).
				def.Name = disp
				defs = append(defs, *def)
				// cache for lookups with display name too
				r.cache[disp] = &toolCacheEntry{def: *def, mcpDef: t}
			}
		}
	}
	return defs
}

func (r *Registry) MatchDefinition(pattern string) []*llm.ToolDefinition {
	var result []*llm.ToolDefinition

	logf(r.debugWriter, "[tool:match] pattern=%q", pattern)
	// Strip suffix selector (e.g., "|root=...;") when present
	if i := strings.Index(pattern, "|"); i != -1 {
		pattern = strings.TrimSpace(pattern[:i])
	}
	// Virtual first: support exact, wildcard, and service-only (no colon) patterns.
	for id, def := range r.virtualDefs {
		if tmatch.Match(pattern, id) {
			copyDef := def
			result = append(result, &copyDef)
			logf(r.debugWriter, "[tool:match] + virtual %s", id)
		}
	}
	// Discover matching server tools when pattern specifies an MCP service prefix.
	if svc := serverFromPattern(pattern); svc != "" {
		logf(r.debugWriter, "[tool:match] service=%q from pattern", svc)
		tools, err := r.listServerTools(context.Background(), svc)
		if err != nil {
			r.warnf("[tool:match] %s list tools failed: %v", svc, err)
		}
		logf(r.debugWriter, "[tool:match] %s tools fetched: %d", svc, len(tools))
		for _, t := range tools {
			full := svc + "/" + t.Name
			if tmatch.Match(pattern, full) {
				def := llm.ToolDefinitionFromMcpTool(&t)
				def.Name = full
				result = append(result, def)
				if _, ok := r.cache[def.Name]; !ok {
					r.cache[def.Name] = &toolCacheEntry{def: *def, mcpDef: t}
				}
				logf(r.debugWriter, "[tool:match] + %s", def.Name)
			}
		}
	}
	logf(r.debugWriter, "[tool:match] pattern=%q matched=%d", pattern, len(result))
	return result
}

func (r *Registry) GetDefinition(name string) (*llm.ToolDefinition, bool) {
	logf(r.debugWriter, "[tool:getdef] name=%q", name)
	if def, ok := r.virtualDefs[name]; ok {
		return &def, true
	}
	// cache hit?
	if e, ok := r.cache[name]; ok {
		def := e.def
		return &def, true
	}
	svc := serverFromName(name)
	if svc == "" {
		logf(r.debugWriter, "[tool:getdef] no service derived for %q", name)
		return nil, false
	}
	logf(r.debugWriter, "[tool:getdef] service=%q", svc)
	tools, err := r.listServerTools(context.Background(), svc)
	if err != nil {
		r.warnf("[tool:getdef] %s list tools failed: %v", svc, err)
		return nil, false
	}
	for _, t := range tools {
		if t.Name == name {
			tool := llm.ToolDefinitionFromMcpTool(&t)
			if tool != nil {
				r.cache[name] = &toolCacheEntry{def: *tool, mcpDef: t}
			}
			return tool, true
		}
	}
	return nil, false
}

func (r *Registry) MustHaveTools(patterns []string) ([]llm.Tool, error) {
	var ret []llm.Tool
	var missing []string
	for _, n := range patterns {
		matchedTools := r.MatchDefinition(n)
		if len(matchedTools) == 0 {
			missing = append(missing, n)
		}
		for _, matchedTool := range matchedTools {
			ret = append(ret, llm.Tool{Type: "function", Definition: *matchedTool})
		}
	}
	if len(missing) > 0 {
		return ret, fmt.Errorf("tools not found: %s", strings.Join(missing, ", "))
	}
	return ret, nil
}

func (r *Registry) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	// Handle selector suffix and base tool name
	var selector string
	baseName := name
	if i := strings.Index(name, "|"); i != -1 {
		baseName = strings.TrimSpace(name[:i])
		selector = strings.TrimSpace(name[i+1:])
	}
	// virtual tool?
	if h, ok := r.virtualExec[baseName]; ok {
		out, err := h(ctx, args)
		if err != nil || selector == "" {
			return out, err
		}
		// Post-filter output when possible (JSON expected)
		return r.applySelector(out, selector)
	}
	// cached executable?
	if e, ok := r.cache[baseName]; ok && e.exec != nil {
		out, err := e.exec(ctx, args)
		if err != nil || selector == "" {
			return out, err
		}
		return r.applySelector(out, selector)
	}
	if r.debugWriter != nil {
		fmt.Fprintf(r.debugWriter, "[tool] call %s args=%v (mcp)\n", baseName, args)
	}

	convID := memory.ConversationIDFromContext(ctx)
	serviceName, _ := splitToolName(baseName)
	server := serviceName
	if server == "" {
		return "", fmt.Errorf("invalid tool name: %s", name)
	}
	var options []mcpclient.RequestOption
	if tok := authctx.Bearer(ctx); tok != "" {
		options = append(options, mcpclient.WithAuthToken(tok))
	}
	// Acquire appropriate client: internal or per-conversation via manager.
	var cli mcpclient.Interface
	var err error
	if c, ok := r.internal[server]; ok && c != nil {
		cli = c
	} else {
		cli, err = r.mgr.Get(ctx, convID, server)
	}
	if err != nil {
		return "", err
	}
	if r.mgr != nil {
		defer r.mgr.Touch(convID, server)
	}

	// Respect context deadline when present; default a generous timeout.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()
	}

	// Use proxy to normalize tool name and execute
	px, _ := mcpproxy.NewProxy(ctx, server, cli)
	res, err := px.CallTool(ctx, baseName, args, options...)
	if err != nil {
		if r.debugWriter != nil {
			fmt.Fprintf(r.debugWriter, "[tool] error %s: %v\n", baseName, err)
		}
		return "", err
	}
	if res.IsError != nil && *res.IsError {
		return "", toolError(res)
	}
	// Compose textual result prioritising structured → json/text → first content
	if res.StructuredContent != nil {
		if data, err := json.Marshal(res.StructuredContent); err == nil {
			out := string(data)
			if selector != "" {
				return r.applySelector(out, selector)
			}
			return out, nil
		}
	}
	for _, c := range res.Content {
		if strings.TrimSpace(c.Text) != "" {
			if selector != "" {
				return r.applySelector(c.Text, selector)
			}
			return c.Text, nil
		}
		if strings.TrimSpace(c.Data) != "" {
			if selector != "" {
				return r.applySelector(c.Data, selector)
			}
			return c.Data, nil
		}
	}
	// Fallback to raw JSON of first element or empty
	if len(res.Content) > 0 {
		raw, _ := json.Marshal(res.Content[0])
		out := string(raw)
		if selector != "" {
			return r.applySelector(out, selector)
		}
		return out, nil
	}
	return "", nil
}

func (r *Registry) applySelector(out, selector string) (string, error) {
	spec := transform.ParseSuffix(selector)
	if spec == nil {
		return out, nil
	}
	data := []byte(out)
	filtered, err := spec.Apply(data)
	if err != nil {
		r.warnf("[tool:filter] apply failed: %v", err)
		return out, nil // fallback to original
	}
	return string(filtered), nil
}

func (r *Registry) SetDebugLogger(w io.Writer) { r.debugWriter = w }

// AddInternalService registers a service.Service as an in-memory MCP client under its Service.Name().
func (r *Registry) AddInternalService(s svc.Service) error {
	if s == nil {
		return fmt.Errorf("nil service")
	}
	cli, err := localmcp.NewServiceClient(context.Background(), s)
	if err != nil {
		return err
	}
	if r.internal == nil {
		r.internal = map[string]mcpclient.Interface{}
	}
	r.internal[s.Name()] = cli
	return nil
}

// Initialize attempts to eagerly discover MCP servers and list their tools to
// warm the local cache. It logs warnings for unreachable servers.
func (r *Registry) Initialize(ctx context.Context) {
	if r == nil {
		return
	}
	servers, err := r.listServers(ctx)
	if err != nil {
		r.warnf("[tool:init] list servers failed: %v", err)
		return
	}
	for _, s := range servers {
		tools, err := r.listServerTools(ctx, s)
		if err != nil {
			r.warnf("[tool:init] %s list tools failed: %v", s, err)
			continue
		}
		for _, t := range tools {
			full := s + "/" + t.Name
			if _, ok := r.cache[full]; ok {
				continue
			}
			if def := llm.ToolDefinitionFromMcpTool(&t); def != nil {
				def.Name = full
				r.cache[full] = &toolCacheEntry{def: *def, mcpDef: t}
			}
		}
		logf(r.debugWriter, "[tool:init] %s tools cached: %d", s, len(tools))
	}
}

func logf(w io.Writer, format string, args ...interface{}) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format+"\n", args...)
}

func (r *Registry) warnf(format string, args ...interface{}) {
	logf(r.debugWriter, format, args...)
	r.warnings = append(r.warnings, fmt.Sprintf(format, args...))
}

// LastWarnings returns any accumulated non-fatal warnings and does not clear them.
func (r *Registry) LastWarnings() []string {
	if len(r.warnings) == 0 {
		return nil
	}
	out := make([]string, len(r.warnings))
	copy(out, r.warnings)
	return out
}

// ClearWarnings clears accumulated warnings.
func (r *Registry) ClearWarnings() { r.warnings = nil }

// ---------------------- helpers ----------------------

// serverFromName extracts the service prefix from a tool name (service/method).
func serverFromName(name string) string { svc, _ := splitToolName(name); return svc }

// serverFromPattern returns service prefix when pattern contains it.
func serverFromPattern(pattern string) string {
	// If explicit method present, derive service from canonical name
	if strings.Contains(pattern, ":") {
		return serverFromName(pattern)
	}
	// Strip trailing wildcard for server name
	if strings.Contains(pattern, "*") {
		return strings.TrimSuffix(pattern, "*")
	}
	// Service-only token
	return pattern
}

// matchPattern supports '*' suffix matching for convenience.
func matchPattern(pattern, name string) bool {
	if pattern == name {
		return true
	}
	if strings.Contains(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(name, prefix)
	}
	// Service-only pattern (no colon, no wildcard): match any method under service
	if noColon(pattern) {
		return serverFromName(name) == pattern
	}
	return false
}

func noColon(s string) bool { return !strings.Contains(s, ":") }

// splitToolName returns service path and method given a name like "service/path:method".
func splitToolName(name string) (service, method string) {
	can := mcpnames.Canonical(name)
	n := mcpnames.Name(can)
	return n.Service(), n.Method()
}

// listServerTools queries the server tool registry via MCP ListTools.
func (r *Registry) listServerTools(ctx context.Context, server string) ([]mcpschema.Tool, error) {
	// Prefer internal client if present
	if c, ok := r.internal[server]; ok && c != nil {
		px, _ := mcpproxy.NewProxy(ctx, server, c)
		return px.ListAllTools(ctx)
	}
	if r.mgr == nil {
		return nil, errors.New("mcp manager not configured")
	}
	cli, err := r.mgr.Get(ctx, "", server)
	if err != nil {
		return nil, err
	}
	px, _ := mcpproxy.NewProxy(ctx, server, cli)
	return px.ListAllTools(ctx)
}

// listServers returns MCP client names from the workspace repository.
func (r *Registry) listServers(ctx context.Context) ([]string, error) {
	repo := mcprepo.New(afs.New())
	names, _ := repo.List(ctx)
	// Merge with internal client names
	seen := map[string]struct{}{}
	var out []string
	for _, n := range names {
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	for n := range r.internal {
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out, nil
}

// toolError converts an error‑flagged MCP result into Go error.
func toolError(res *mcpschema.CallToolResult) error {
	if len(res.Content) == 0 {
		return errors.New("tool returned error without content")
	}
	if msg := res.Content[0].Text; msg != "" {
		return errors.New(msg)
	}
	raw, _ := json.Marshal(res.Content[0])
	return errors.New(string(raw))
}

// addInternalMcp registers built-in services as in-memory MCP clients.
func (r *Registry) addInternalMcp() {
	// system/exec
	{
		svc := toolExec.New()
		if cli, err := localmcp.NewServiceClient(context.Background(), svc); err == nil && cli != nil {
			r.internal[svc.Name()] = cli
		} else if err != nil {
			r.warnf("[tool:init] internal mcp for %s failed: %v", svc.Name(), err)
		}
	}
	// system/patch
	{
		svc := toolPatch.New()
		if cli, err := localmcp.NewServiceClient(context.Background(), svc); err == nil && cli != nil {
			r.internal[svc.Name()] = cli
		} else if err != nil {
			r.warnf("[tool:init] internal mcp for %s failed: %v", svc.Name(), err)
		}
	}
	// orchestration/plan
	{
		s := orchplan.New()
		if cli, err := localmcp.NewServiceClient(context.Background(), s); err == nil && cli != nil {
			r.internal[s.Name()] = cli
		} else if err != nil {
			r.warnf("[tool:init] internal mcp for %s failed: %v", s.Name(), err)
		}
	}
}

// injectOrchestratorVirtualTool registers the orchestration entry point as a virtual tool.
func (r *Registry) injectOrchestratorVirtualTool() {
	def := llm.ToolDefinition{
		Name:        "llm/exec:run_agent",
		Description: "Run an agent by id with a given objective",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{"agentId": map[string]interface{}{"type": "string"}, "objective": map[string]interface{}{"type": "string"}, "context": map[string]interface{}{"type": "object"}}, "required": []string{"agentId", "objective"}},
		OutputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"answer": map[string]interface{}{"type": "string"}},
		},
	}
	r.virtualDefs[def.Name] = def
}
