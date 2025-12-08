package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
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
	toolOS "github.com/viant/agently/genai/tool/service/system/os"
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
	internal        map[string]mcpclient.Interface
	internalTimeout map[string]time.Duration

	// cache: tool name → entry
	cache map[string]*toolCacheEntry

	// guards concurrent access to cache, warnings, and virtual maps
	mu sync.RWMutex

	warnings []string

	// recentResults memoizes identical tool calls per conversation for a short TTL
	recentMu      sync.Mutex
	recentResults map[string]map[string]recentItem // convID -> key -> item
	recentTTL     time.Duration

	// background refresh configuration
	refreshEvery time.Duration // successful refresh cadence
}

type toolCacheEntry struct {
	def    llm.ToolDefinition
	mcpDef mcpschema.Tool
	exec   Handler
}

type recentItem struct {
	when time.Time
	out  string
}

// Handler executes a tool call and returns its textual result.
type Handler func(ctx context.Context, args map[string]interface{}) (string, error)

// NewWithManager creates a registry backed by an MCP client manager.
func NewWithManager(mgr *manager.Manager) (*Registry, error) {
	if mgr == nil {
		return nil, fmt.Errorf("adapter/tool: nil mcp manager passed to NewWithManager")
	}
	r := &Registry{
		virtualDefs:     map[string]llm.ToolDefinition{},
		virtualExec:     map[string]Handler{},
		mgr:             mgr,
		cache:           map[string]*toolCacheEntry{},
		internal:        map[string]mcpclient.Interface{},
		internalTimeout: map[string]time.Duration{},
		recentResults:   map[string]map[string]recentItem{},
		recentTTL:       5 * time.Second,
		refreshEvery:    30 * time.Second,
	}
	// Register in-memory MCP clients for built-in services using Service.Name().
	r.addInternalMcp()
	// Register orchestrator synthetic tool.
	r.injectOrchestratorVirtualTool()
	return r, nil
}

// debugf emits a formatted debug line to the configured debugWriter when present.
func (r *Registry) debugf(format string, args ...interface{}) {
	if r == nil || r.debugWriter == nil {
		return
	}
	_, _ = fmt.Fprintf(r.debugWriter, "[tools] "+format+"\n", args...)
}

// WithManager attaches a per-conversation MCP manager used to inject the
// appropriate client and auth token into the context at call-time.
func (r *Registry) WithManager(m *manager.Manager) *Registry { r.mgr = m; return r }

// InjectVirtualAgentTools registers synthetic tool definitions that delegate
// execution to another agent. It must be called once during bootstrap *after*
// the agent catalogue is loaded. Domain can be empty to expose all.
func (r *Registry) InjectVirtualAgentTools(agents []*agent.Agent, domain string) {
	r.mu.Lock()
	defer r.mu.Unlock()
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

		// If agent declares an elicitation block, append an autogenerated
		// section to Description to reuse existing calling conventions.
		if ag.ContextInputs != nil && ag.ContextInputs.Enabled {
			var b strings.Builder
			base := strings.TrimSpace(def.Description)
			if base != "" {
				b.WriteString(base)
			}
			b.WriteString("\n\nWhen calling this agent, include the following fields in args.context (auxiliary inputs):\n")
			reqSet := map[string]struct{}{}
			for _, r := range ag.ContextInputs.RequestedSchema.Required {
				reqSet[r] = struct{}{}
			}
			// Render properties in a stable order
			names := make([]string, 0, len(ag.ContextInputs.RequestedSchema.Properties))
			for n := range ag.ContextInputs.RequestedSchema.Properties {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, name := range names {
				typ := ""
				descText := ""
				if m, ok := ag.ContextInputs.RequestedSchema.Properties[name].(map[string]interface{}); ok {
					if v, ok := m["type"].(string); ok {
						typ = strings.TrimSpace(v)
					}
					if v, ok := m["description"].(string); ok {
						descText = strings.TrimSpace(v)
					}
				}
				if typ == "" {
					typ = "any"
				}
				_, isReq := reqSet[name]
				// Format: - context.<name> (type, required): description
				b.WriteString("- context.")
				b.WriteString(name)
				b.WriteString(" (" + typ)
				if isReq {
					b.WriteString(", required")
				}
				b.WriteString(")")
				if descText != "" {
					b.WriteString(": ")
					b.WriteString(descText)
				}
				b.WriteString("\n")
			}
			def.Description = b.String()
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
	// Always include virtual tools.
	r.mu.RLock()
	for _, def := range r.virtualDefs {
		defs = append(defs, def)
	}
	r.mu.RUnlock()

	// Build a set of entries we've already included to avoid duplicates.
	seen := map[string]struct{}{}

	// Include any cached MCP tools so they remain visible even if servers are offline.
	r.mu.RLock()
	for _, e := range r.cache {
		// Display using service:method for consistency
		svc, method := splitToolName(e.def.Name)
		if svc == "" || method == "" {
			continue
		}
		disp := svc + ":" + method
		if _, ok := seen[disp]; ok {
			continue
		}
		def := e.def
		def.Name = disp
		defs = append(defs, def)
		seen[disp] = struct{}{}
	}
	r.mu.RUnlock()

	// Try to aggregate current server tools; merge with cache, but never remove on failure.
	servers, err := r.listServers(context.Background())
	if err != nil {
		r.warnf("tools: list servers failed: %v", err)
		return defs
	}
	for _, s := range servers {
		tools, err := r.listServerTools(context.Background(), s)
		if err != nil {
			// Keep cached entries; just warn on failure.
			r.warnf("tools: list %s failed: %v", s, err)
			continue
		}
		for _, t := range tools {
			disp := s + ":" + t.Name
			if _, ok := seen[disp]; ok {
				continue
			}
			if def := llm.ToolDefinitionFromMcpTool(&t); def != nil {
				def.Name = disp
				defs = append(defs, *def)
				seen[disp] = struct{}{}
				// Update cache for lookup by display name
				r.mu.Lock()
				r.cache[disp] = &toolCacheEntry{def: *def, mcpDef: t}
				r.mu.Unlock()
			}
		}
	}
	return defs
}

func (r *Registry) MatchDefinition(pattern string) []*llm.ToolDefinition {
	var result []*llm.ToolDefinition

	// removed noisy debug logging
	// Strip suffix selector (e.g., "|root=...;") when present
	if i := strings.Index(pattern, "|"); i != -1 {
		pattern = strings.TrimSpace(pattern[:i])
	}
	// Virtual first: support exact, wildcard, and service-only (no colon) patterns.
	r.mu.RLock()
	for id, def := range r.virtualDefs {
		if tmatch.Match(pattern, id) {
			copyDef := def
			result = append(result, &copyDef)
		}
	}
	r.mu.RUnlock()
	// Discover matching server tools when pattern specifies an MCP service prefix.
	if svc := serverFromPattern(pattern); svc != "" {
		tools, err := r.listServerTools(context.Background(), svc)
		if err != nil {
			r.warnf("list tools failed for %s: %v", svc, err)
		}
		for _, t := range tools {
			full := svc + "/" + t.Name
			if tmatch.Match(pattern, full) {
				def := llm.ToolDefinitionFromMcpTool(&t)
				def.Name = full
				result = append(result, def)
				r.mu.Lock()
				if _, ok := r.cache[def.Name]; !ok {
					entry := &toolCacheEntry{def: *def, mcpDef: t}
					r.cache[def.Name] = entry
					// also cache colon alias
					colon := svc + ":" + t.Name
					r.cache[colon] = entry
				}
				r.mu.Unlock()
			}
		}
	}
	return result
}

func (r *Registry) GetDefinition(name string) (*llm.ToolDefinition, bool) {
	// Lightweight debug hook to trace how tool definitions are resolved.
	r.debugf("GetDefinition: name=%s", strings.TrimSpace(name))
	r.mu.RLock()
	if def, ok := r.virtualDefs[name]; ok {
		r.mu.RUnlock()
		r.debugf("GetDefinition: hit virtual tool %s", name)
		return &def, true
	}
	// cache hit?
	if e, ok := r.cache[name]; ok {
		def := e.def
		r.mu.RUnlock()
		r.debugf("GetDefinition: hit cache %s (service=%s)", name, serverFromName(name))
		return &def, true
	}
	r.mu.RUnlock()
	svc := serverFromName(name)
	if svc == "" {
		r.debugf("GetDefinition: no service resolved for %s", name)
		return nil, false
	}
	tools, err := r.listServerTools(context.Background(), svc)
	if err != nil {
		r.warnf("list tools failed for %s: %v", svc, err)
		return nil, false
	}
	// Compare by method part; add both aliases on hit
	_, method := splitToolName(name)
	for _, t := range tools {
		if strings.TrimSpace(t.Name) == strings.TrimSpace(method) {
			tool := llm.ToolDefinitionFromMcpTool(&t)
			if tool != nil {
				r.mu.Lock()
				// Normalise tool name to include service prefix so downstream
				// callers (agents, adapters) never see bare method names like
				// "read" without their service context.
				fullSlash := svc + "/" + t.Name
				tool.Name = fullSlash
				entry := &toolCacheEntry{def: *tool, mcpDef: t}
				// cache both aliases and the exact name used
				colon := svc + ":" + t.Name
				r.cache[fullSlash] = entry
				r.cache[colon] = entry
				r.cache[name] = entry
				r.mu.Unlock()
				r.debugf("GetDefinition: populated cache for %s (service=%s, method=%s)", name, svc, method)
			}
			return tool, true
		}
	}
	r.debugf("GetDefinition: tool not found (service=%s, method=%s, name=%s)", svc, method, name)
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
	convID := memory.ConversationIDFromContext(ctx)
	r.debugf("Execute: name=%s base=%s selector=%s convID=%s", strings.TrimSpace(name), strings.TrimSpace(baseName), strings.TrimSpace(selector), convID)

	// virtual tool?
	if h, ok := r.virtualExec[baseName]; ok {
		r.debugf("Execute: using virtual tool %s", baseName)
		out, err := h(ctx, args)
		if err != nil || selector == "" {
			return out, err
		}
		// Post-filter output when possible (JSON expected)
		return r.applySelector(out, selector)
	}
	// cached executable?
	r.mu.RLock()
	if e, ok := r.cache[baseName]; ok && e.exec != nil {
		r.mu.RUnlock()
		r.debugf("Execute: hit cached exec for %s (service=%s)", baseName, serverFromName(baseName))
		out, err := e.exec(ctx, args)
		if err != nil || selector == "" {
			return out, err
		}
		return r.applySelector(out, selector)
	}
	r.mu.RUnlock()

	serviceName, _ := splitToolName(baseName)
	server := serviceName
	if server == "" {
		r.debugf("Execute: invalid tool name (no server): %s", baseName)
		return "", fmt.Errorf("invalid tool name: %s", name)
	}
	r.debugf("Execute: resolving via MCP server=%s method=%s (base=%s, convID=%s)", server, mcpnames.Name(mcpnames.Canonical(baseName)).Method(), baseName, convID)
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

	// Deduplicate rapid identical calls per conversation (memoization)
	// Key uses fully qualified tool name and a stable JSON for args.
	keyArgs, _ := json.Marshal(args)
	recentKey := baseName + "|" + string(keyArgs)
	if r.recentTTL > 0 {
		r.recentMu.Lock()
		if m := r.recentResults[convID]; m != nil {
			if it, ok := m[recentKey]; ok && time.Since(it.when) <= r.recentTTL {
				r.recentMu.Unlock()
				return it.out, nil
			}
		}
		r.recentMu.Unlock()
	}

	// Use proxy to normalize tool name and execute with reconnect-aware retry.
	px, _ := mcpproxy.NewProxy(ctx, server, cli)
	const maxAttempts = 3 // initial + 2 retries
	var res *mcpschema.CallToolResult
	for attempt := 0; attempt < maxAttempts; attempt++ {
		res, err = px.CallTool(ctx, baseName, args, options...)
		if err == nil {
			if res.IsError != nil && *res.IsError {
				terr := toolError(res)
				if r.mgr != nil && isReconnectableError(terr) && attempt < maxAttempts-1 {
					// reconnect and retry
					if _, rerr := r.mgr.Reconnect(ctx, convID, server); rerr == nil {
						if ncli, gerr := r.mgr.Get(ctx, convID, server); gerr == nil {
							px, _ = mcpproxy.NewProxy(ctx, server, ncli)
							continue
						}
					}
				}
				return "", terr
			}
			break
		}
		// Transport/client-level error
		if r.mgr != nil && isReconnectableError(err) && attempt < maxAttempts-1 {
			if _, rerr := r.mgr.Reconnect(ctx, convID, server); rerr == nil {
				if ncli, gerr := r.mgr.Get(ctx, convID, server); gerr == nil {
					px, _ = mcpproxy.NewProxy(ctx, server, ncli)
					continue
				}
			}
		}
		// Non-reconnectable or reconnect failed
		return "", err
	}
	// Compose textual result prioritising structured → json/text → first content
	if res.StructuredContent != nil {
		if data, err := json.Marshal(res.StructuredContent); err == nil {
			out := string(data)
			if selector != "" {
				return r.applySelector(out, selector)
			}
			if r.recentTTL > 0 {
				r.recentMu.Lock()
				if r.recentResults[convID] == nil {
					r.recentResults[convID] = map[string]recentItem{}
				}
				r.recentResults[convID][recentKey] = recentItem{when: time.Now(), out: out}
				r.recentMu.Unlock()
			}
			return out, nil
		}
	}
	for _, c := range res.Content {
		if strings.TrimSpace(c.Text) != "" {
			if selector != "" {
				return r.applySelector(c.Text, selector)
			}
			if r.recentTTL > 0 {
				r.recentMu.Lock()
				if r.recentResults[convID] == nil {
					r.recentResults[convID] = map[string]recentItem{}
				}
				r.recentResults[convID][recentKey] = recentItem{when: time.Now(), out: c.Text}
				r.recentMu.Unlock()
			}
			return c.Text, nil
		}
		if strings.TrimSpace(c.Data) != "" {
			if selector != "" {
				return r.applySelector(c.Data, selector)
			}
			if r.recentTTL > 0 {
				r.recentMu.Lock()
				if r.recentResults[convID] == nil {
					r.recentResults[convID] = map[string]recentItem{}
				}
				r.recentResults[convID][recentKey] = recentItem{when: time.Now(), out: c.Data}
				r.recentMu.Unlock()
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
		r.warnf("selector apply failed: %v", err)
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
	r.mu.Lock()
	if r.internal == nil {
		r.internal = map[string]mcpclient.Interface{}
	}
	if r.internalTimeout == nil {
		r.internalTimeout = map[string]time.Duration{}
	}
	r.internal[s.Name()] = cli
	// Capture service-provided timeout when available
	if tt, ok := any(s).(interface{ ToolTimeout() time.Duration }); ok {
		if d := tt.ToolTimeout(); d > 0 {
			r.internalTimeout[s.Name()] = d
		}
	}
	r.mu.Unlock()
	return nil
}

// ToolTimeout returns a suggested timeout for a given tool name.
func (r *Registry) ToolTimeout(name string) (time.Duration, bool) {
	server := serverFromName(name)
	if server == "" {
		return 0, false
	}
	// Internal service timeout
	if d, ok := r.internalTimeout[server]; ok && d > 0 {
		return d, true
	}
	// MCP client config timeout
	if r.mgr != nil {
		if opts, err := r.mgr.Options(context.Background(), server); err == nil && opts != nil {
			if opts.ToolTimeoutSec > 0 {
				return time.Duration(opts.ToolTimeoutSec) * time.Second, true
			}
		}
	}
	return 0, false
}

// Initialize attempts to eagerly discover MCP servers and list their tools to
// warm the local cache. It logs warnings for unreachable servers.
func (r *Registry) Initialize(ctx context.Context) {
	if r == nil {
		return
	}
	servers, err := r.listServers(ctx)
	if err != nil {
		r.warnf("list servers failed: %v", err)
		return
	}
	for _, s := range servers {
		tools, err := r.listServerTools(ctx, s)
		if err != nil {
			r.warnf("list tools failed for %s: %v", s, err)
			continue
		}
		for _, t := range tools {
			full := s + "/" + t.Name
			r.mu.RLock()
			_, ok := r.cache[full]
			r.mu.RUnlock()
			if ok {
				continue
			}
			if def := llm.ToolDefinitionFromMcpTool(&t); def != nil {
				def.Name = full
				r.mu.Lock()
				r.cache[full] = &toolCacheEntry{def: *def, mcpDef: t}
				r.mu.Unlock()
			}
		}
	}
	// Start background refresh monitors to auto-register tools when servers come online
	r.startAutoRefresh(ctx)

}

// startAutoRefresh launches a monitor per known server that periodically attempts
// to refresh its tool list and update the cache when connectivity is restored.
func (r *Registry) startAutoRefresh(ctx context.Context) {
	servers, err := r.listServers(ctx)
	if err != nil {
		r.warnf("refresh: list servers failed: %v", err)
		return
	}
	for _, s := range servers {
		srv := s
		go r.monitorServer(ctx, srv)
	}
}

func (r *Registry) monitorServer(ctx context.Context, server string) {
	// Exponential backoff on errors; steady cadence on success.
	backoff := time.Second
	maxBackoff := 60 * time.Second
	jitter := time.Millisecond * 200
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// Attempt refresh
		if err := r.refreshServerTools(ctx, server); err != nil {
			// wait with backoff + jitter
			d := backoff
			if d < time.Second {
				d = time.Second
			}
			if d > maxBackoff {
				d = maxBackoff
			}
			// add small jitter
			d += time.Duration(time.Now().UnixNano() % int64(jitter))
			timer := time.NewTimer(d)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			// increase backoff up to cap
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			continue
		}
		// success: reset backoff and wait for steady refresh interval
		backoff = time.Second
		interval := r.refreshEvery
		if interval <= 0 {
			interval = 30 * time.Second
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// refreshServerTools lists tools for a server and atomically replaces its cache entries.
func (r *Registry) refreshServerTools(ctx context.Context, server string) error {
	tools, err := r.listServerTools(ctx, server)
	if err != nil {
		return err
	}
	r.replaceServerTools(server, tools)
	return nil
}

// replaceServerTools atomically replaces cache entries for a given server.
func (r *Registry) replaceServerTools(server string, tools []mcpschema.Tool) {
	// Build new map for server
	newEntries := make(map[string]*toolCacheEntry, len(tools)*2)
	for _, t := range tools {
		full := server + "/" + t.Name
		if def := llm.ToolDefinitionFromMcpTool(&t); def != nil {
			def.Name = full
			entry := &toolCacheEntry{def: *def, mcpDef: t}
			newEntries[full] = entry
			// also colon alias
			colon := server + ":" + t.Name
			newEntries[colon] = entry
		}
	}
	// If refresh returns no tools, retain previous cache for this server.
	if len(newEntries) == 0 {
		r.warnf("refresh: %s returned no tools; retaining previous cache", server)
		return
	}
	// Swap entries under lock: remove old for server, then add new
	r.mu.Lock()
	for k := range r.cache {
		if serverFromName(k) == server {
			delete(r.cache, k)
		}
	}
	for k, v := range newEntries {
		r.cache[k] = v
	}
	r.mu.Unlock()
}

func (r *Registry) warnf(format string, args ...interface{}) {
	r.mu.Lock()
	r.warnings = append(r.warnings, fmt.Sprintf(format, args...))
	r.mu.Unlock()
}

// LastWarnings returns any accumulated non-fatal warnings and does not clear them.
func (r *Registry) LastWarnings() []string {
	r.mu.RLock()
	if len(r.warnings) == 0 {
		r.mu.RUnlock()
		return nil
	}
	out := make([]string, len(r.warnings))
	copy(out, r.warnings)
	r.mu.RUnlock()
	return out
}

// ClearWarnings clears accumulated warnings.
func (r *Registry) ClearWarnings() { r.mu.Lock(); r.warnings = nil; r.mu.Unlock() }

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
	// Optional override to force discovery of specific servers (comma-separated)
	if extra := strings.TrimSpace(os.Getenv("AGENTLY_MCP_SERVERS")); extra != "" {
		for _, s := range strings.Split(extra, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			names = append(names, s)
		}
	}
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

// isReconnectableError heuristically classifies transport/stream errors that
// are likely to be resolved by reconnecting the MCP client and retrying.
func isReconnectableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "stream error"),
		strings.Contains(msg, "internal_error; received from peer"),
		strings.Contains(msg, "rst_stream"),
		strings.Contains(msg, "goaway"),
		strings.Contains(msg, "http2"),
		strings.Contains(msg, "trip not found"),
		strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "broken pipe"),
		strings.Contains(msg, "eof"),
		strings.Contains(msg, "failed to parse response: trip not found"),
		strings.Contains(msg, "server closed idle connection"),
		strings.Contains(msg, "no cached connection"):
		return true
	}
	return false
}

// addInternalMcp registers built-in services as in-memory MCP clients.
func (r *Registry) addInternalMcp() {
	// system/exec
	{
		svc := toolExec.New()
		if cli, err := localmcp.NewServiceClient(context.Background(), svc); err == nil && cli != nil {
			r.internal[svc.Name()] = cli
		} else if err != nil {
			r.warnf("internal mcp for %s failed: %v", svc.Name(), err)
		}
	}
	// system/patch
	{
		svc := toolPatch.New()
		if cli, err := localmcp.NewServiceClient(context.Background(), svc); err == nil && cli != nil {
			r.internal[svc.Name()] = cli
		} else if err != nil {
			r.warnf("internal mcp for %s failed: %v", svc.Name(), err)
		}
	}
	// system/os
	{
		svc := toolOS.New()
		if cli, err := localmcp.NewServiceClient(context.Background(), svc); err == nil && cli != nil {
			r.internal[svc.Name()] = cli
		} else if err != nil {
			r.warnf("internal mcp for %s failed: %v", svc.Name(), err)
		}
	}
	// orchestration/plan
	{
		s := orchplan.New()
		if cli, err := localmcp.NewServiceClient(context.Background(), s); err == nil && cli != nil {
			r.internal[s.Name()] = cli
		} else if err != nil {
			r.warnf("internal mcp for %s failed: %v", s.Name(), err)
		}
	}

}

// injectOrchestratorVirtualTool registers the orchestration entry point as a virtual tool.
func (r *Registry) injectOrchestratorVirtualTool() {
	r.mu.Lock()
	defer r.mu.Unlock()
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
