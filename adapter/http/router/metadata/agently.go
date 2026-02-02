package metadata

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/agently"
	"github.com/viant/agently/genai/agent"
	execsvc "github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/llm"
	toolbundle "github.com/viant/agently/genai/tool/bundle"
	authctx "github.com/viant/agently/internal/auth"
	convdao "github.com/viant/agently/internal/service/conversation"
	usersvc "github.com/viant/agently/internal/service/user"
	tmatch "github.com/viant/agently/internal/tool/matcher"
	"github.com/viant/agently/internal/workspace"
	bundlerepo "github.com/viant/agently/internal/workspace/repository/toolbundle"
)

type AgentInfo struct {
	// Name is a human-friendly display name for the agent.
	// Always use agentId for selection and routing; name is UI-only.
	Name  string   `json:"name,omitempty"`
	Tools []string `json:"tools"`
	// Bundles are the configured tool bundle IDs for this agent (if any).
	Bundles []string `json:"bundles,omitempty"`
	Model   string   `json:"model"`
	Chains  []string `json:"chains,omitempty"`
	// UI defaults and capabilities
	ToolCallExposure     string   `json:"toolCallExposure,omitempty"`
	ShowExecutionDetails bool     `json:"showExecutionDetails,omitempty"`
	ShowToolFeed         bool     `json:"showToolFeed,omitempty"`
	AutoSummarize        bool     `json:"autoSummarize,omitempty"`
	ChainsEnabled        bool     `json:"chainsEnabled,omitempty"`
	AllowedChains        []string `json:"allowedChains,omitempty"`
	// Client UX: ring sound when a turn finishes
	RingOnFinish bool `json:"ringOnFinish,omitempty"`
	// Reasoning default (effort)
	ReasoningEffort *string `json:"reasoningEffort,omitempty"`
	// Profile metadata for UI/selection context
	Responsibilities []string `json:"responsibilities,omitempty"`
	InScope          []string `json:"inScope,omitempty"`
	OutOfScope       []string `json:"outOfScope,omitempty"`

	// Elicitation (optional): a schema-driven payload request declared on the agent
	// (struct name ContextInputs; YAML key remains "elicitation").
	Elicitation *agent.ContextInputs `json:"elicitation,omitempty"`
}

type ToolBundleInfo struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description,omitempty"`
	IconRef     string                 `json:"iconRef,omitempty"`
	IconURI     string                 `json:"iconURI,omitempty"`
	Priority    int                    `json:"priority,omitempty"`
	Match       []toolbundle.MatchRule `json:"match,omitempty"`
}

type ToolInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Bundles     []string `json:"bundles,omitempty"`
}

type ModelInfo struct {
	// Name is a human-friendly display name for the model.
	// Always use the model ID for selection/routing; name is UI-only.
	Name string `json:"name,omitempty"`
	// Description is optional additional information for UI/tooltips.
	Description string `json:"description,omitempty"`
}

// Option is a generic select option shape used by Forge/Blueprint controls.
// `Value` is the internal ID used by the backend; `Label` is UI-only.
type Option struct {
	ID    string `json:"id,omitempty"`
	Value string `json:"value"`
	Label string `json:"label"`
}

// AgentlyResponse is the aggregated workspace metadata payload.
type AgentlyResponse struct {
	// WorkspaceRoot is the absolute path to the current workspace root.
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`
	Defaults      struct {
		Agent    string `json:"agent"`
		Model    string `json:"model"`
		Embedder string `json:"embedder,omitempty"`
		// AutoSelectTools reflects workspace defaults for automatic tool bundle selection.
		AutoSelectTools bool `json:"autoSelectTools,omitempty"`
	} `json:"defaults"`
	// Agents is a flat list of agent IDs (selection values).
	// Display names are provided via AgentInfo[id].name.
	Agents []string `json:"agents"`
	Tools  []string `json:"tools"`
	// ToolsTree groups tools by service prefix using ':' as the separator.
	// Example: { "sqlkit": ["dbExec", "dbQuery"], "system/exec": ["execute"] }
	ToolsTree   map[string][]string  `json:"toolsTree,omitempty"`
	ToolBundles []ToolBundleInfo     `json:"toolBundles,omitempty"`
	ToolInfo    map[string]*ToolInfo `json:"toolInfo,omitempty"`
	Models      []string             `json:"models"`
	// ModelOptions is an explicit label/value list for UI selectors.
	// When present, UIs should prefer this over deriving options from `models`.
	ModelOptions []Option `json:"modelOptions,omitempty"`
	// ModelInfo provides UI display metadata keyed by model ID.
	ModelInfo map[string]*ModelInfo `json:"modelInfo,omitempty"`
	// AgentInfo lists matched tool names per agent using pattern matching
	// rules derived from the agent's Tool configuration.
	AgentInfo map[string]*AgentInfo `json:"agentInfo,omitempty"`
	// Version reports compiled application version (from build/ldflags or embedded file).
	Version string `json:"version,omitempty"`
}

// Aggregate builds an AgentlyResponse from executor config and tool definitions.
func Aggregate(cfg *execsvc.Config, defs []llm.ToolDefinition, bundles []*toolbundle.Bundle) (*AgentlyResponse, error) {
	if cfg == nil {
		return nil, ErrNilConfig
	}

	out := &AgentlyResponse{}
	out.Defaults.Agent = cfg.Default.Agent
	out.Defaults.Model = cfg.Default.Model
	out.Defaults.Embedder = cfg.Default.Embedder
	out.Defaults.AutoSelectTools = cfg.Default.ToolAutoSelection.Enabled

	// Agents: list IDs only; UI should use AgentInfo[id].name for display.
	if cfg.Agent != nil {
		for _, a := range cfg.Agent.Items {
			if a == nil {
				continue
			}
			if a.Internal {
				continue
			}
			id := strings.TrimSpace(a.ID)
			if id != "" {
				out.Agents = append(out.Agents, id)
			}
		}
	}

	// Models: publish IDs and additional UI metadata.
	if cfg.Model != nil {
		for _, m := range cfg.Model.Items {
			if m == nil || strings.TrimSpace(m.ID) == "" {
				continue
			}
			id := strings.TrimSpace(m.ID)
			out.Models = append(out.Models, id)
			if out.ModelInfo == nil {
				out.ModelInfo = map[string]*ModelInfo{}
			}
			if _, ok := out.ModelInfo[id]; !ok {
				name := strings.TrimSpace(firstNonEmpty(m.Name, m.Options.Model, id))
				out.ModelInfo[id] = &ModelInfo{
					Name:        name,
					Description: strings.TrimSpace(m.Description),
				}
			}
		}
	}

	// Tool: from llm definitions (hide internal/* services)
	for _, d := range defs {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			continue
		}
		if i := strings.IndexByte(name, ':'); i != -1 {
			if strings.HasPrefix(strings.TrimSpace(name[:i]), "internal/") {
				continue
			}
		}
		out.Tools = append(out.Tools, name)
		if out.ToolInfo == nil {
			out.ToolInfo = map[string]*ToolInfo{}
		}
		out.ToolInfo[name] = &ToolInfo{Name: name, Description: strings.TrimSpace(d.Description)}
		// Build ToolsTree grouping by service prefix using ':'
		if i := strings.IndexByte(name, ':'); i != -1 {
			svc := strings.TrimSpace(name[:i])
			method := strings.TrimSpace(name[i+1:])
			if svc != "" && method != "" {
				if out.ToolsTree == nil {
					out.ToolsTree = map[string][]string{}
				}
				out.ToolsTree[svc] = append(out.ToolsTree[svc], method)
			}
		}
	}

	// Sort for deterministic output
	sort.Strings(out.Agents)
	sort.Strings(out.Models)
	sort.Strings(out.Tools)

	// Build explicit model options after sorting so list ordering is stable.
	if len(out.Models) > 0 {
		out.ModelOptions = make([]Option, 0, len(out.Models))
		for _, id := range out.Models {
			label := id
			if out.ModelInfo != nil && out.ModelInfo[id] != nil && strings.TrimSpace(out.ModelInfo[id].Name) != "" {
				label = strings.TrimSpace(out.ModelInfo[id].Name)
			}
			out.ModelOptions = append(out.ModelOptions, Option{ID: id, Value: id, Label: label})
		}
	}

	// Normalize bundles: when none provided, derive from available tools for UI convenience.
	if len(bundles) == 0 {
		bundles = toolbundle.DeriveBundles(defs)
	}
	if len(bundles) > 0 {
		perBundle := map[string]map[string]struct{}{} // bundleID -> toolName set
		for _, b := range bundles {
			if b == nil || strings.TrimSpace(b.ID) == "" {
				continue
			}
			id := strings.TrimSpace(b.ID)
			perBundle[id] = matchBundleTools(b, out.Tools)
			out.ToolBundles = append(out.ToolBundles, ToolBundleInfo{
				ID:          id,
				Title:       strings.TrimSpace(b.Title),
				Description: strings.TrimSpace(b.Description),
				IconRef:     strings.TrimSpace(b.IconRef),
				IconURI:     strings.TrimSpace(b.IconURI),
				Priority:    b.Priority,
				Match:       append([]toolbundle.MatchRule(nil), b.Match...),
			})
		}
		sort.Slice(out.ToolBundles, func(i, j int) bool {
			if out.ToolBundles[i].Priority != out.ToolBundles[j].Priority {
				return out.ToolBundles[i].Priority > out.ToolBundles[j].Priority
			}
			return out.ToolBundles[i].ID < out.ToolBundles[j].ID
		})
		for bundleID, toolSet := range perBundle {
			for toolName := range toolSet {
				if out.ToolInfo[toolName] == nil {
					continue
				}
				out.ToolInfo[toolName].Bundles = append(out.ToolInfo[toolName].Bundles, bundleID)
			}
		}
		for _, ti := range out.ToolInfo {
			if ti == nil || len(ti.Bundles) == 0 {
				continue
			}
			sort.Strings(ti.Bundles)
		}
	}

	// Build AgentInfo mapping (matched tool names per agent). Always attempt to populate entries.
	if cfg.Agent != nil && len(cfg.Agent.Items) > 0 {
		// Build filtered tool defs excluding internal/* services to avoid exposing them
		filtered := make([]llm.ToolDefinition, 0, len(defs))
		for _, d := range defs {
			n := strings.TrimSpace(d.Name)
			if n == "" {
				continue
			}
			if i := strings.IndexByte(n, ':'); i != -1 {
				if strings.HasPrefix(strings.TrimSpace(n[:i]), "internal/") {
					continue
				}
			}
			filtered = append(filtered, d)
		}
		defs = filtered
		if out.AgentInfo == nil {
			out.AgentInfo = make(map[string]*AgentInfo)
		}
		// Precompute tool names (raw)
		names := make([]string, 0, len(defs))
		for _, d := range defs {
			if d.Name == "" {
				continue
			}
			names = append(names, strings.TrimSpace(d.Name))
		}

		// Index bundle tool sets for fast per-agent expansion.
		bundleIndex := map[string]map[string]struct{}{} // lower(bundleID) -> toolName set
		for _, bi := range out.ToolBundles {
			if strings.TrimSpace(bi.ID) == "" {
				continue
			}
			b := &toolbundle.Bundle{ID: bi.ID, Match: bi.Match}
			bundleIndex[strings.ToLower(strings.TrimSpace(b.ID))] = matchBundleTools(b, names)
		}

		for _, a := range cfg.Agent.Items {
			if a == nil {
				continue
			}
			if a.Internal {
				continue
			}
			agentID := strings.TrimSpace(a.ID)
			if agentID == "" {
				continue
			}
			agentName := strings.TrimSpace(a.Name)
			// Build patterns from agent.Tool (raw; matcher normalizes internally)
			var patterns []string
			for _, t := range a.Tool.Items {
				if t == nil {
					continue
				}
				pat := strings.TrimSpace(t.Pattern)
				if pat == "" {
					pat = strings.TrimSpace(t.Definition.Name)
				}
				if pat == "" {
					continue
				}
				patterns = append(patterns, pat)
			}
			bundlesSel := normalizeStringList(a.Tool.Bundles)
			// Collect chain targets (agent ids)
			var chainTargets []string
			if len(a.Chains) > 0 {
				seen := map[string]struct{}{}
				for _, ch := range a.Chains {
					if ch == nil || ch.Target.AgentID == "" {
						continue
					}
					id := strings.TrimSpace(ch.Target.AgentID)
					if id == "" {
						continue
					}
					if _, ok := seen[id]; ok {
						continue
					}
					seen[id] = struct{}{}
					chainTargets = append(chainTargets, id)
				}
				sort.Strings(chainTargets)
			}

			// Select tool names from bundles + patterns (dedup)
			seenTools := map[string]struct{}{}
			matched := make([]string, 0, len(defs))
			for _, bid := range bundlesSel {
				set := bundleIndex[strings.ToLower(strings.TrimSpace(bid))]
				for name := range set {
					if _, ok := seenTools[name]; ok {
						continue
					}
					seenTools[name] = struct{}{}
					matched = append(matched, name)
				}
			}
			for i, d := range defs {
				name := names[i]
				for _, p := range patterns {
					if tmatch.Match(p, name) {
						if _, ok := seenTools[d.Name]; ok {
							break
						}
						seenTools[d.Name] = struct{}{}
						matched = append(matched, d.Name)
						break
					}
				}
			}
			sort.Strings(matched)
			// Defaults per request
			exposure := strings.TrimSpace(string(a.Tool.CallExposure))
			if exposure == "" {
				exposure = "turn"
			}
			showExec := true
			if a.ShowExecutionDetails != nil {
				showExec = *a.ShowExecutionDetails
			}
			showFeed := true
			if a.ShowToolFeed != nil {
				showFeed = *a.ShowToolFeed
			}
			autoSum := true
			if a.AutoSummarize != nil {
				autoSum = *a.AutoSummarize
			}
			chainsEnabled := true

			info := &AgentInfo{
				Name:                 firstNonEmpty(agentName, agentID),
				Tools:                matched,
				Bundles:              append([]string(nil), bundlesSel...),
				Model:                a.Model,
				Chains:               chainTargets,
				ToolCallExposure:     exposure,
				ShowExecutionDetails: showExec,
				ShowToolFeed:         showFeed,
				AutoSummarize:        autoSum,
				ChainsEnabled:        chainsEnabled,
				AllowedChains:        append([]string(nil), chainTargets...),
				RingOnFinish:         a.RingOnFinish,
				Elicitation:          a.ContextInputs,
			}
			if a.Reasoning != nil && strings.TrimSpace(a.Reasoning.Effort) != "" {
				v := strings.TrimSpace(a.Reasoning.Effort)
				info.ReasoningEffort = &v
			}
			if a.Profile != nil {
				if len(a.Profile.Responsibilities) > 0 {
					info.Responsibilities = append([]string(nil), a.Profile.Responsibilities...)
				}
				if len(a.Profile.InScope) > 0 {
					info.InScope = append([]string(nil), a.Profile.InScope...)
				}
				if len(a.Profile.OutOfScope) > 0 {
					info.OutOfScope = append([]string(nil), a.Profile.OutOfScope...)
				}
			}
			out.AgentInfo[agentID] = info
		}
	}

	return out, nil
}

func canon(s string) string { return tmatch.Canon(s) }

// firstNonEmpty returns the first non-empty string from inputs or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// NewAgently returns an http.HandlerFunc that writes aggregated workspace
// metadata including defaults, agents, tools and models.
func NewAgently(exec *execsvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := exec.Config()
		if cfg == nil {
			http.Error(w, ErrNilConfig.Error(), http.StatusInternalServerError)
			return
		}
		var defs []llm.ToolDefinition
		if exec.LLMCore() != nil {
			defs = exec.LLMCore().ToolDefinitions()
		}
		bundles, berr := loadToolBundles(r.Context())
		if berr != nil {
			http.Error(w, berr.Error(), http.StatusInternalServerError)
			return
		}
		// Transient debug: enable with ?debug=1
		debug := strings.TrimSpace(r.URL.Query().Get("debug")) != ""
		if debug {
			// Filter out internal/* and canon names
			filtered := make([]llm.ToolDefinition, 0, len(defs))
			for _, d := range defs {
				n := strings.TrimSpace(d.Name)
				if n == "" {
					continue
				}
				if i := strings.IndexByte(n, ':'); i != -1 {
					if strings.HasPrefix(strings.TrimSpace(n[:i]), "internal/") {
						continue
					}
				}
				filtered = append(filtered, d)
			}
			names := make([]string, 0, len(filtered))
			for _, d := range filtered {
				n := strings.TrimSpace(d.Name)
				if n != "" {
					names = append(names, n)
				}
			}
			// Per agent, log patterns and matched tool names
			if cfg.Agent != nil {
				for _, a := range cfg.Agent.Items {
					if a == nil || strings.TrimSpace(a.ID) == "" {
						continue
					}
					var patterns []string
					for _, t := range a.Tool.Items {
						if t == nil {
							continue
						}
						p := strings.TrimSpace(t.Pattern)
						if p == "" {
							p = strings.TrimSpace(t.Definition.Name)
						}
						if p == "" {
							continue
						}
						patterns = append(patterns, p)
					}
					matched := []string{}
					seen := map[string]struct{}{}
					for i, d := range filtered {
						nm := names[i]
						for _, p := range patterns {
							if tmatch.Match(p, nm) {
								if _, ok := seen[d.Name]; ok {
									break
								}
								seen[d.Name] = struct{}{}
								matched = append(matched, d.Name)
								break
							}
						}
					}
					log.Printf("[metadata.debug] agent=%s patterns=%v matched=%v defs=%d", strings.TrimSpace(a.ID), patterns, matched, len(filtered))
				}
			}
		}
		resp, err := Aggregate(cfg, defs, bundles)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp.WorkspaceRoot = workspace.Root()
		// Populate compiled version without coupling Aggregate to build details.
		resp.Version = strings.TrimSpace(agently.Version)
		// Override defaults with user preferences when available
		// Best-effort: if anything fails, keep workspace defaults.
		if uname := strings.TrimSpace(authctx.EffectiveUserID(r.Context())); uname != "" {
			if dao, derr := convdao.NewDatly(context.Background()); derr == nil {
				if us, uerr := usersvc.New(context.Background(), dao); uerr == nil {
					if u, ferr := us.FindByUsername(context.Background(), uname); ferr == nil && u != nil {
						if u.DefaultAgentRef != nil && strings.TrimSpace(*u.DefaultAgentRef) != "" {
							resp.Defaults.Agent = strings.TrimSpace(*u.DefaultAgentRef)
						}
						if u.DefaultModelRef != nil && strings.TrimSpace(*u.DefaultModelRef) != "" {
							resp.Defaults.Model = strings.TrimSpace(*u.DefaultModelRef)
						}
						if u.DefaultEmbedderRef != nil && strings.TrimSpace(*u.DefaultEmbedderRef) != "" {
							resp.Defaults.Embedder = strings.TrimSpace(*u.DefaultEmbedderRef)
						}
					}
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Status string           `json:"status"`
			Data   *AgentlyResponse `json:"data"`
		}{
			Status: "ok",
			Data:   resp,
		})
	}
}

func normalizeStringList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func matchBundleTools(b *toolbundle.Bundle, toolNames []string) map[string]struct{} {
	out := map[string]struct{}{}
	if b == nil || len(b.Match) == 0 || len(toolNames) == 0 {
		return out
	}
	for _, rule := range b.Match {
		inc := strings.TrimSpace(rule.Name)
		if inc == "" {
			continue
		}
		excluded := map[string]struct{}{}
		for _, ex := range rule.Exclude {
			ex = strings.TrimSpace(ex)
			if ex == "" {
				continue
			}
			for _, name := range toolNames {
				if tmatch.Match(ex, name) {
					excluded[name] = struct{}{}
				}
			}
		}
		for _, name := range toolNames {
			if !tmatch.Match(inc, name) {
				continue
			}
			if _, ok := excluded[name]; ok {
				continue
			}
			out[name] = struct{}{}
		}
	}
	return out
}

func loadToolBundles(ctx context.Context) ([]*toolbundle.Bundle, error) {
	repo := bundlerepo.New(afs.New())
	return repo.LoadAll(ctx)
}
