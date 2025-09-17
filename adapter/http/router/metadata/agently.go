package metadata

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	execsvc "github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/llm"
)

// AgentlyResponse is the aggregated workspace metadata payload.
type AgentlyResponse struct {
	Defaults struct {
		Agent    string `json:"agent"`
		Model    string `json:"model"`
		Embedder string `json:"embedder,omitempty"`
	} `json:"defaults"`
	Agents []string `json:"agents"`
	Tools  []string `json:"tools"`
	Models []string `json:"models"`
	// AgentTools lists matched tool names per agent using pattern matching
	// rules derived from the agent's Tool configuration.
	AgentTools map[string][]string `json:"agentTools,omitempty"`
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
		if strings.TrimSpace(d.Name) == "" {
			continue
		}
		out.Tools = append(out.Tools, d.Name)
	}

	// Sort for deterministic output
	sort.Strings(out.Agents)
	sort.Strings(out.Models)
	sort.Strings(out.Tools)

	// Build AgentTools mapping (matched tool names per agent)
	if cfg.Agent != nil && len(cfg.Agent.Items) > 0 && len(defs) > 0 {
		out.AgentTools = make(map[string][]string)
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
			agentName := strings.TrimSpace(a.Name)
			if agentName == "" {
				agentName = strings.TrimSpace(a.ID)
			}
			if agentName == "" {
				continue
			}
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
			if len(patterns) == 0 {
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
			if len(matched) > 0 {
				out.AgentTools[agentName] = matched
			}
		}
		if len(out.AgentTools) == 0 {
			out.AgentTools = nil // omit when empty
		}
	}

	return out, nil
}

func canon(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), "/", "_") }

// NewAgently returns an http.HandlerFunc that writes aggregated workspace
// metadata including defaults, agents, tools and models.
func NewAgently(exec *execsvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
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
