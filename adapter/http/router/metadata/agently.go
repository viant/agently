package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	execsvc "github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/llm"
	authctx "github.com/viant/agently/internal/auth"
	convdao "github.com/viant/agently/internal/service/conversation"
	usersvc "github.com/viant/agently/internal/service/user"
)

type AgentInfo struct {
	Tools  []string `json:"tools"`
	Model  string   `json:"model"`
	Chains []string `json:"chains,omitempty"`
	// UI defaults and capabilities
	ToolCallExposure     string   `json:"toolCallExposure,omitempty"`
	ShowExecutionDetails bool     `json:"showExecutionDetails,omitempty"`
	ShowToolFeed         bool     `json:"showToolFeed,omitempty"`
	AutoSummarize        bool     `json:"autoSummarize,omitempty"`
	ChainsEnabled        bool     `json:"chainsEnabled,omitempty"`
	AllowedChains        []string `json:"allowedChains,omitempty"`
}

// AgentlyResponse is the aggregated workspace metadata payload.
type AgentlyResponse struct {
	Defaults struct {
		Agent    string `json:"agent"`
		Model    string `json:"model"`
		Embedder string `json:"embedder,omitempty"`
	} `json:"defaults"`
	Agents []string `json:"agents"`
	Tools  []string `json:"tools"`
	// ToolsTree groups tools by service prefix using ':' as the separator.
	// Example: { "sqlkit": ["dbExec", "dbQuery"], "system/exec": ["execute"] }
	ToolsTree map[string][]string `json:"toolsTree,omitempty"`
	Models    []string            `json:"models"`
	// AgentInfo lists matched tool names per agent using pattern matching
	// rules derived from the agent's Tool configuration.
	AgentInfo map[string]*AgentInfo `json:"agentInfo,omitempty"`
}

// Aggregate builds an AgentlyResponse from executor config and tool definitions.
func Aggregate(cfg *execsvc.Config, defs []llm.ToolDefinition) (*AgentlyResponse, error) {
	if cfg == nil {
		return nil, ErrNilConfig
	}

	out := &AgentlyResponse{}
	out.Defaults.Agent = cfg.Default.Agent
	out.Defaults.Model = cfg.Default.Model
	out.Defaults.Embedder = cfg.Default.Embedder

	// Agents: prefer Name, fallback to ID when name empty.
	if cfg.Agent != nil {
		for _, a := range cfg.Agent.Items {
			if a == nil {
				continue
			}
			name := strings.TrimSpace(a.Name)
			if name == "" {
				name = strings.TrimSpace(a.ID)
			}
			if name != "" {
				out.Agents = append(out.Agents, name)
			}
		}
	}

	// Models: use ID as display name.
	if cfg.Model != nil {
		for _, m := range cfg.Model.Items {
			if m == nil || strings.TrimSpace(m.ID) == "" {
				continue
			}
			out.Models = append(out.Models, strings.TrimSpace(m.ID))
		}
	}

	// Tools: from llm definitions
	for _, d := range defs {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			continue
		}
		out.Tools = append(out.Tools, name)
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

	// Build AgentInfo mapping (matched tool names per agent)
	if cfg.Agent != nil && len(cfg.Agent.Items) > 0 && len(defs) > 0 {
		out.AgentInfo = make(map[string]*AgentInfo)
		// Precompute canon tool names
		names := make([]string, 0, len(defs))
		for _, d := range defs {
			if d.Name == "" {
				continue
			}
			names = append(names, canon(d.Name))
		}

		for _, a := range cfg.Agent.Items {
			if a == nil {
				continue
			}
			agentID := strings.TrimSpace(a.ID)
			if agentID == "" {
				continue
			}
			agentName := strings.TrimSpace(a.Name)
			// Build patterns from agent.Tool
			var patterns []string
			for _, t := range a.Tool {
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
				patterns = append(patterns, canon(pat))
			}
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

			if len(patterns) == 0 && len(chainTargets) == 0 {
				continue
			}
			// Match tool names
			var matched []string
			for i, d := range defs {
				name := names[i]
				for _, p := range patterns {
					if p == "*" || strings.HasPrefix(name, p) {
						matched = append(matched, d.Name)
						break
					}
				}
			}
			sort.Strings(matched)
			if len(matched) > 0 || len(chainTargets) > 0 {
				// Defaults per request
				exposure := strings.TrimSpace(string(a.ToolCallExposure))
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
					Tools:                matched,
					Model:                a.Model,
					Chains:               chainTargets,
					ToolCallExposure:     exposure,
					ShowExecutionDetails: showExec,
					ShowToolFeed:         showFeed,
					AutoSummarize:        autoSum,
					ChainsEnabled:        chainsEnabled,
					AllowedChains:        append([]string(nil), chainTargets...),
				}
				out.AgentInfo[agentID] = info
				if agentName != "" {
					out.AgentInfo[strings.ToLower(agentName)] = info
				}
			}
		}
		if len(out.AgentInfo) == 0 {
			out.AgentInfo = nil // omit when empty
		}
	}

	return out, nil
}

func canon(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), "/", "_") }

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
		resp, err := Aggregate(cfg, defs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
