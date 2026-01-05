package agent

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/afs/file"
	"github.com/viant/afs/url"
	agentmdl "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/prompt"
	"github.com/viant/agently/internal/shared"
	"github.com/viant/agently/internal/workspace"
	meta "github.com/viant/agently/internal/workspace/service/meta"
	yml "github.com/viant/agently/internal/workspace/service/meta/yml"
	"github.com/viant/embedius/matching/option"

	"gopkg.in/yaml.v3"
)

// Ensure Service implements interfaces.Loader so that changes are caught by
// the compiler.
var _ agentmdl.Loader = (*Service)(nil)

const (
	defaultExtension = ".yaml"
)

// Service provides agent data access operations
type Service struct {
	metaService *meta.Service
	agents      shared.Map[string, *agentmdl.Agent] //[ url ] -> [ agent]

	defaultExtension string
}

// LoadAgents loads an agents from the specified URL
func (s *Service) LoadAgents(ctx context.Context, URL string) ([]*agentmdl.Agent, error) {
	candidates, err := s.metaService.List(ctx, URL)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent from %s: %w", URL, err)
	}
	var result []*agentmdl.Agent
	for _, candidate := range candidates {
		anAgent, err := s.Load(ctx, candidate)
		if err != nil {
			return nil, fmt.Errorf("failed to load agent from %s: %w", candidate, err)
		}
		result = append(result, anAgent)
	}
	return result, nil

}

func (s *Service) List() []*agentmdl.Agent {
	result := make([]*agentmdl.Agent, 0)
	s.agents.Range(
		func(key string, value *agentmdl.Agent) bool {
			result = append(result, value)
			return true
		})
	return result
}

// Add adds an agent to the service
func (s *Service) Add(name string, agent *agentmdl.Agent) {
	s.agents.Set(name, agent)
}

// Lookup looks up an agent by name
func (s *Service) Lookup(ctx context.Context, name string) (*agentmdl.Agent, error) {
	anAgent, ok := s.agents.Get(name)
	if !ok {
		return nil, fmt.Errorf("agent %q not found", name)
	}
	return anAgent, nil
}

// Load loads an agent from the specified URL
func (s *Service) Load(ctx context.Context, nameOrLocation string) (*agentmdl.Agent, error) {
	// Resolve relative name (e.g. "chat") to a workspace file path.
	// All other workspace kinds store definitions flat as
	//   <kind>/<name>.yaml
	// so we keep the same convention for agents instead of the previous
	// nested  <kind>/<name>/<name>.yaml layout.
	URL := nameOrLocation
	if !strings.Contains(URL, "/") && filepath.Ext(nameOrLocation) == "" {
		URL = filepath.Join(workspace.KindAgent, nameOrLocation)
	}

	if url.IsRelative(URL) {
		ext := ""
		if filepath.Ext(URL) == "" {
			ext = s.defaultExtension
		}
		if ok, _ := s.metaService.Exists(ctx, URL+ext); ok {
			// Keep relative URL; metaService will resolve against its base.
			URL = URL + ext
		} else {
			candidate := path.Join(URL, nameOrLocation)
			if ok, _ = s.metaService.Exists(ctx, candidate+ext); ok {
				URL = candidate + ext
			}
		}
	}

	var node yaml.Node
	if err := s.metaService.Load(ctx, URL, &node); err != nil {
		return nil, fmt.Errorf("failed to load agent from %s: %w", URL, err)
	}

	anAgent := &agentmdl.Agent{Source: &agentmdl.Source{URL: s.metaService.GetURL(URL)}}
	// Parse the YAML into our agent model
	if err := s.parseAgent((*yml.Node)(&node), anAgent); err != nil {
		return nil, fmt.Errorf("failed to parse agent from %s: %w", URL, err)
	}
	// Normalize parsed agent prior to validation
	normalizeAgent(anAgent)
	if err := anAgent.Validate(); err != nil {
		return nil, fmt.Errorf("invalid agent %s: %w", URL, err)
	}

	// Set agent name based on URL if not set
	if anAgent.Name == "" {
		anAgent.Name = getAgentNameFromURL(URL)
	}

	srcURL := anAgent.Source.URL
	for i := range anAgent.Knowledge {
		knowledge := anAgent.Knowledge[i]
		if knowledge.URL == "" {
			return nil, fmt.Errorf("agent %v knowledge URL is empty", anAgent.Name)
		}
		// If URL has no scheme: if it’s absolute (drive/UNC/POSIX), convert to file:// URL.
		// Otherwise resolve relative to agent source URL.
		if url.Scheme(knowledge.URL, "") == "" {
			u := strings.TrimSpace(knowledge.URL)
			if !url.IsRelative(u) { // absolute OS path
				anAgent.Knowledge[i].URL = url.ToFileURL(u)
			} else if url.IsRelative(u) && !url.IsRelative(srcURL) {
				parentURL, _ := url.Split(srcURL, file.Scheme)
				anAgent.Knowledge[i].URL = url.JoinUNC(parentURL, u)
			}
		}
		// Validate that knowledge path exists
		if ok, _ := s.metaService.Exists(ctx, anAgent.Knowledge[i].URL); !ok {
			return nil, fmt.Errorf("agent %v knowledge path does not exist: %s", anAgent.Name, anAgent.Knowledge[i].URL)
		}
	}

	for i := range anAgent.SystemKnowledge {
		knowledge := anAgent.SystemKnowledge[i]
		if knowledge.URL == "" {
			return nil, fmt.Errorf("agent %v system knowledge URL is empty", anAgent.Name)
		}
		if url.Scheme(knowledge.URL, "") == "" {
			u := strings.TrimSpace(knowledge.URL)
			if !url.IsRelative(u) {
				anAgent.SystemKnowledge[i].URL = url.ToFileURL(u)
			} else if url.IsRelative(u) && !url.IsRelative(srcURL) {
				parentURL, _ := url.Split(srcURL, file.Scheme)
				anAgent.SystemKnowledge[i].URL = url.JoinUNC(parentURL, u)
			}
		}
		// Validate that system knowledge path exists
		if ok, _ := s.metaService.Exists(ctx, anAgent.SystemKnowledge[i].URL); !ok {
			return nil, fmt.Errorf("agent %v system knowledge path does not exist: %s", anAgent.Name, anAgent.SystemKnowledge[i].URL)
		}
	}

	// Ensure that knowledge and systemKnowledge locations are also reflected as
	// resources with semantic match enabled so they participate in tools such as
	// resources.match and resources.roots.
	augmentResourcesWithKnowledge(anAgent)

	// Resolve relative prompt URIs against the agent source location
	s.resolvePromptURIs(anAgent)

	// Validate agent
	if err := anAgent.Validate(); err != nil {
		return nil, fmt.Errorf("invalid agent configuration from %s: %w", URL, err)
	}

	s.agents.Set(anAgent.Name, anAgent)
	return anAgent, nil
}

// resolvePromptURIs updates agent Prompt/SystemPrompt URI when relative by
// resolving them against the agent source URL directory.
func (s *Service) resolvePromptURIs(a *agentmdl.Agent) {
	if a == nil || a.Source == nil || strings.TrimSpace(a.Source.URL) == "" {
		return
	}
	base, _ := url.Split(a.Source.URL, file.Scheme)
	resolvePath := func(p *prompt.Prompt) {
		if p == nil {
			return
		}
		u := strings.TrimSpace(p.URI)
		if u == "" {
			return
		}
		if url.Scheme(u, "") == "" && !strings.HasPrefix(u, "/") {
			p.URI = url.Join(base, u)
		}
	}
	resolvePath(a.Prompt)
	resolvePath(a.SystemPrompt)
	for _, chain := range a.Chains {
		if query := chain.Query; query != nil && query.URI != "" {
			resolvePath(query)
			if when := chain.When; when != nil && when.Query != nil && when.Query.URI != "" {
				resolvePath(when.Query)
			}
		}
	}
}

// (drive/UNC detection is handled by afs/url.IsRelative and ToFileURL)

// normalizeAgent applies generic cleanups that make downstream behavior stable:
// - trims trailing whitespace/newlines from prompt texts (agent and chains)
// - ensures chain.When.Expr is set when a scalar was used
func normalizeAgent(a *agentmdl.Agent) {
	trim := func(p *prompt.Prompt) {
		if p == nil {
			return
		}
		if strings.TrimSpace(p.Text) != "" {
			p.Text = strings.TrimRight(p.Text, "\r\n\t ")
		}
	}
	trim(a.Prompt)
	trim(a.SystemPrompt)
	for _, c := range a.Chains {
		if c == nil {
			continue
		}
		trim(c.Query)
		if c.When != nil {
			trim(c.When.Query)
		}
		// If When exists but Expr is empty and Query is nil, keep as-is (explicit empty)
	}

}

// parseAgent parses agent properties from a YAML node
func (s *Service) parseAgent(node *yml.Node, agent *agentmdl.Agent) error {
	rootNode := node
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		rootNode = (*yml.Node)(node.Content[0])
	}

	// Look for the "agent" root node
	var agentNode *yml.Node
	err := rootNode.Pairs(func(key string, valueNode *yml.Node) error {
		if strings.ToLower(key) == "agent" && valueNode.Kind == yaml.MappingNode {
			agentNode = valueNode
			return nil
		}
		return nil
	})

	if err != nil {
		return err
	}
	if agentNode == nil {
		agentNode = rootNode // Use the root node if no "agent" node is found
	}

	// Parse agent properties
	return agentNode.Pairs(func(key string, valueNode *yml.Node) error {
		lowerKey := strings.ToLower(key)
		switch lowerKey {
		case "id":
			if valueNode.Kind == yaml.ScalarNode {
				agent.ID = valueNode.Value
			}
		case "name":
			if valueNode.Kind == yaml.ScalarNode {
				agent.Name = valueNode.Value
			}
		case "icon":
			if valueNode.Kind == yaml.ScalarNode {
				agent.Icon = valueNode.Value
			}
		case "modelref":
			if valueNode.Kind == yaml.ScalarNode {
				agent.Model = valueNode.Value
			}
		case "model":
			if valueNode.Kind == yaml.ScalarNode {
				agent.Model = strings.TrimSpace(valueNode.Value)
			}
		case "temperature":
			if valueNode.Kind == yaml.ScalarNode {
				value := valueNode.Interface()
				temp := 0.0
				switch actual := value.(type) {
				case int:
					temp = float64(actual)
				case float64:
					temp = actual
				default:
					return fmt.Errorf("invalid temperature value: %T %v", value, value)
				}
				agent.Temperature = temp
			}
		case "description":
			if valueNode.Kind == yaml.ScalarNode {
				agent.Description = valueNode.Value
			}
		case "allowedproviders":
			// Accept either a scalar (single provider) or a sequence of scalars.
			switch valueNode.Kind {
			case yaml.ScalarNode:
				v := strings.TrimSpace(valueNode.Value)
				if v != "" {
					agent.AllowedProviders = []string{v}
				}
			case yaml.SequenceNode:
				providers := make([]string, 0, len(valueNode.Content))
				for _, item := range valueNode.Content {
					if item.Kind != yaml.ScalarNode {
						continue
					}
					v := strings.TrimSpace(item.Value)
					if v != "" {
						providers = append(providers, v)
					}
				}
				if len(providers) > 0 {
					agent.AllowedProviders = providers
				}
			}
		case "allowedmodels":
			// Accept either a scalar (single model id) or a sequence of scalars.
			switch valueNode.Kind {
			case yaml.ScalarNode:
				v := strings.TrimSpace(valueNode.Value)
				if v != "" {
					agent.AllowedModels = []string{v}
				}
			case yaml.SequenceNode:
				models := make([]string, 0, len(valueNode.Content))
				for _, item := range valueNode.Content {
					if item.Kind != yaml.ScalarNode {
						continue
					}
					v := strings.TrimSpace(item.Value)
					if v != "" {
						models = append(models, v)
					}
				}
				if len(models) > 0 {
					agent.AllowedModels = models
				}
			}
		case "paralleltoolcalls":
			if valueNode.Kind == yaml.ScalarNode {
				val := valueNode.Interface()
				switch actual := val.(type) {
				case bool:
					agent.ParallelToolCalls = actual
				case string:
					lv := strings.ToLower(strings.TrimSpace(actual))
					agent.ParallelToolCalls = lv == "true"
				}
			}
		case "autosummarize", "autosumarize":
			// Support correct and common misspelling keys. Accept bool or string values.
			if valueNode.Kind == yaml.ScalarNode {
				val := valueNode.Interface()
				switch actual := val.(type) {
				case bool:
					agent.AutoSummarize = &actual
				case string:
					lv := strings.ToLower(strings.TrimSpace(actual))
					v := lv == "true" || lv == "yes" || lv == "on"
					agent.AutoSummarize = &v
				}
			}
		case "showexecutiondetails":
			if valueNode.Kind == yaml.ScalarNode {
				val := strings.ToLower(strings.TrimSpace(valueNode.Value))
				b := val == "true" || val == "yes" || val == "on" || val == "1"
				agent.ShowExecutionDetails = &b
			}
		case "showtoolfeed":
			if valueNode.Kind == yaml.ScalarNode {
				val := strings.ToLower(strings.TrimSpace(valueNode.Value))
				b := val == "true" || val == "yes" || val == "on" || val == "1"
				agent.ShowToolFeed = &b
			}
		case "ringonfinish", "finishring", "notifyonfinish":
			if valueNode.Kind == yaml.ScalarNode {
				val := strings.ToLower(strings.TrimSpace(valueNode.Value))
				agent.RingOnFinish = val == "true" || val == "yes" || val == "on" || val == "1"
			}
		case "knowledge":
			if err := s.parseKnowledgeBlock(valueNode, agent); err != nil {
				return err
			}
		case "systemknowledge":
			if err := s.parseSystemKnowledgeBlock(valueNode, agent); err != nil {
				return err
			}
		case "resources":
			if err := s.parseResourcesBlock(valueNode, agent); err != nil {
				return err
			}
		case "prompt":
			if agent.Prompt, err = s.getPrompt(valueNode); err != nil {
				return err
			}

		case "systemprompt":
			if agent.SystemPrompt, err = s.getPrompt(valueNode); err != nil {
				return err
			}

		case "tool":
			switch valueNode.Kind {
			case yaml.SequenceNode:
				// Legacy format: tool: [ ... ]
				if err := s.parseToolBlock(valueNode, agent); err != nil {
					return err
				}
			case yaml.MappingNode:
				// New format: tool: { items: [], toolCallExposure: "..." }
				if err := s.parseToolConfig(valueNode, agent); err != nil {
					return err
				}
			default:
				return fmt.Errorf("invalid tool block; expected sequence or mapping")
			}

		case "profile":
			if err := s.parseProfileBlock(valueNode, agent); err != nil {
				return err
			}

		case "serve":
			if err := s.parseServeBlock(valueNode, agent); err != nil {
				return err
			}

		case "exposea2a":
			if err := s.parseExposeA2ABlock(valueNode, agent); err != nil {
				return err
			}

			for _, itemNode := range valueNode.Content {
				switch itemNode.Kind {
				case yaml.ScalarNode:
					name := strings.TrimSpace(itemNode.Value)
					if name == "" {
						continue
					}
					agent.Tool.Items = append(agent.Tool.Items, &llm.Tool{Pattern: name, Type: "function"})

				case yaml.MappingNode:
					var t llm.Tool
					if err := itemNode.Decode(&t); err != nil {
						return fmt.Errorf("invalid tool definition: %w", err)
					}
					// Normalise & defaults ------------------------------------------------
					if t.Pattern == "" {
						t.Pattern = t.Ref // fallback to ref when pattern omitted
					}
					if t.Type == "" {
						t.Type = "function"
					}
					if t.Pattern == "" {
						return fmt.Errorf("tool entry missing pattern/ref")
					}
					agent.Tool.Items = append(agent.Tool.Items, &llm.Tool{Pattern: t.Pattern, Ref: t.Ref, Type: t.Type, Definition: t.Definition})

				default:
					return fmt.Errorf("unsupported YAML node for tool entry: kind=%d", itemNode.Kind)
				}
			}
		case "persona":
			if valueNode.Kind == yaml.MappingNode {
				var p prompt.Persona
				if err := (*yaml.Node)(valueNode).Decode(&p); err != nil {
					return fmt.Errorf("invalid persona definition: %w", err)
				}
				agent.Persona = &p
			}
		case "reasoning":
			if valueNode.Kind == yaml.MappingNode {
				var r llm.Reasoning
				if err := (*yaml.Node)(valueNode).Decode(&r); err != nil {
					return fmt.Errorf("invalid reasoning definition: %w", err)
				}
				agent.Reasoning = &r
			}
		case "callexposure", "toolcallexposure":
			// Accept scalar values: turn | conversation | semantic
			if valueNode.Kind == yaml.ScalarNode {
				exp := agentmdl.ToolCallExposure(strings.ToLower(strings.TrimSpace(valueNode.Value)))
				agent.Tool.CallExposure = exp
				agent.ToolCallExposure = exp
			}
		case "attachment":
			if err := s.parseAttachmentBlock(valueNode, agent); err != nil {
				return err
			}
		case "chains":
			if err := s.parseChainsBlock(valueNode, agent); err != nil {
				return err
			}
			// mcpresources: removed; use generic resources instead
		}
		return nil
	})

}

// parseAgentBasicScalar handles common scalar fields and returns handled=true when processed.
func (s *Service) parseAgentBasicScalar(agent *agentmdl.Agent, key string, valueNode *yml.Node) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "id":
		if valueNode.Kind == yaml.ScalarNode {
			agent.ID = valueNode.Value
		}
		return true, nil
	case "name":
		if valueNode.Kind == yaml.ScalarNode {
			agent.Name = valueNode.Value
		}
		return true, nil
	case "icon":
		if valueNode.Kind == yaml.ScalarNode {
			agent.Icon = valueNode.Value
		}
		return true, nil
	case "modelref", "model":
		if valueNode.Kind == yaml.ScalarNode {
			agent.Model = strings.TrimSpace(valueNode.Value)
		}
		return true, nil
	case "temperature":
		if valueNode.Kind == yaml.ScalarNode {
			value := valueNode.Interface()
			temp := 0.0
			switch actual := value.(type) {
			case int:
				temp = float64(actual)
			case float64:
				temp = actual
			default:
				return true, fmt.Errorf("invalid temperature value: %T %v", value, value)
			}
			agent.Temperature = temp
		}
		return true, nil
	case "description":
		if valueNode.Kind == yaml.ScalarNode {
			agent.Description = valueNode.Value
		}
		return true, nil
	case "paralleltoolcalls":
		if valueNode.Kind == yaml.ScalarNode {
			val := valueNode.Interface()
			switch actual := val.(type) {
			case bool:
				agent.ParallelToolCalls = actual
			case string:
				lv := strings.ToLower(strings.TrimSpace(actual))
				agent.ParallelToolCalls = lv == "true"
			}
		}
		return true, nil
	case "autosummarize", "autosumarize":
		if valueNode.Kind == yaml.ScalarNode {
			val := valueNode.Interface()
			switch actual := val.(type) {
			case bool:
				agent.AutoSummarize = &actual
			case string:
				lv := strings.ToLower(strings.TrimSpace(actual))
				v := lv == "true" || lv == "yes" || lv == "on"
				agent.AutoSummarize = &v
			}
		}
		return true, nil
	case "showexecutiondetails":
		if valueNode.Kind == yaml.ScalarNode {
			val := strings.ToLower(strings.TrimSpace(valueNode.Value))
			b := val == "true" || val == "yes" || val == "on" || val == "1"
			agent.ShowExecutionDetails = &b
		}
		return true, nil
	case "showtoolfeed":
		if valueNode.Kind == yaml.ScalarNode {
			val := strings.ToLower(strings.TrimSpace(valueNode.Value))
			b := val == "true" || val == "yes" || val == "on" || val == "1"
			agent.ShowToolFeed = &b
		}
		return true, nil
	}
	return false, nil
}

func (s *Service) parseKnowledgeBlock(valueNode *yml.Node, agent *agentmdl.Agent) error {
	if valueNode.Kind != yaml.SequenceNode {
		return nil
	}
	for _, itemNode := range valueNode.Content {
		knowledge, err := parseKnowledge((*yml.Node)(itemNode))
		if err != nil {
			return err
		}
		agent.Knowledge = append(agent.Knowledge, knowledge)
	}
	return nil
}

func (s *Service) parseSystemKnowledgeBlock(valueNode *yml.Node, agent *agentmdl.Agent) error {
	if valueNode.Kind != yaml.SequenceNode {
		return nil
	}
	for _, itemNode := range valueNode.Content {
		knowledge, err := parseKnowledge((*yml.Node)(itemNode))
		if err != nil {
			return err
		}
		agent.SystemKnowledge = append(agent.SystemKnowledge, knowledge)
	}
	return nil
}

func (s *Service) parsePromptFields(agent *agentmdl.Agent, key string, valueNode *yml.Node) error {
	var err error
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "prompt":
		agent.Prompt, err = s.getPrompt(valueNode)
	case "systemprompt":
		agent.SystemPrompt, err = s.getPrompt(valueNode)
	}
	return err
}

func (s *Service) parseToolBlock(valueNode *yml.Node, agent *agentmdl.Agent) error {
	if valueNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("tool must be a sequence")
	}
	for _, item := range valueNode.Content {
		if item == nil {
			continue
		}
		switch item.Kind {
		case yaml.ScalarNode:
			v := strings.TrimSpace(item.Value)
			if v == "" {
				continue
			}
			agent.Tool.Items = append(agent.Tool.Items, &llm.Tool{Pattern: v})
		case yaml.MappingNode:
			var t llm.Tool
			var inlineDef llm.ToolDefinition
			var hasInlineDef bool
			if err := (*yml.Node)(item).Pairs(func(k string, v *yml.Node) error {
				lk := strings.ToLower(strings.TrimSpace(k))
				switch lk {
				case "pattern":
					if v.Kind == yaml.ScalarNode {
						t.Pattern = strings.TrimSpace(v.Value)
					}
				case "ref":
					if v.Kind == yaml.ScalarNode {
						t.Ref = strings.TrimSpace(v.Value)
					}
				case "type":
					if v.Kind == yaml.ScalarNode {
						t.Type = strings.TrimSpace(v.Value)
					}
				case "definition":
					if v.Kind == yaml.MappingNode {
						if err := (*yaml.Node)(v).Decode(&t.Definition); err != nil {
							return fmt.Errorf("invalid tool.definition: %w", err)
						}
					}
				case "name":
					if v.Kind == yaml.ScalarNode {
						inlineDef.Name = strings.TrimSpace(v.Value)
						hasInlineDef = true
					}
				case "description":
					if v.Kind == yaml.ScalarNode {
						inlineDef.Description = v.Value
						hasInlineDef = true
					}
				case "parameters":
					if v.Kind == yaml.MappingNode {
						var m map[string]interface{}
						if err := (*yaml.Node)(v).Decode(&m); err != nil {
							return fmt.Errorf("invalid tool.parameters: %w", err)
						}
						inlineDef.Parameters = m
						hasInlineDef = true
					}
				case "required":
					if v.Kind == yaml.SequenceNode {
						var req []string
						if err := (*yaml.Node)(v).Decode(&req); err != nil {
							return fmt.Errorf("invalid tool.required: %w", err)
						}
						inlineDef.Required = req
						hasInlineDef = true
					}
				case "output_schema", "outputschema":
					if v.Kind == yaml.MappingNode {
						var m map[string]interface{}
						if err := (*yaml.Node)(v).Decode(&m); err != nil {
							return fmt.Errorf("invalid tool.output_schema: %w", err)
						}
						inlineDef.OutputSchema = m
						hasInlineDef = true
					}
				}
				return nil
			}); err != nil {
				return err
			}
			if hasInlineDef {
				inlineDef.Normalize()
				t.Definition = inlineDef
				if t.Type == "" {
					t.Type = "function"
				}
			}
			if t.Definition.Name != "" || t.Pattern != "" || t.Ref != "" {
				agent.Tool.Items = append(agent.Tool.Items, &t)
			}
		}
	}
	return nil
}

// parseToolConfig parses the new tool mapping contract:
// tool:
//
//	items: [ ... ]
//	toolCallExposure: turn|conversation
func (s *Service) parseToolConfig(valueNode *yml.Node, agent *agentmdl.Agent) error {
	if valueNode.Kind != yaml.MappingNode {
		return fmt.Errorf("tool must be a mapping")
	}
	var cfg agentmdl.Tool
	// Collect nested nodes first to preserve order-independent parsing.
	var itemsNode *yml.Node
	if err := valueNode.Pairs(func(k string, v *yml.Node) error {
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "items":
			itemsNode = v
		case "bundles", "bundle", "toolsets", "toolset", "connectors", "connector":
			// Accept a single scalar or a sequence of scalars.
			switch v.Kind {
			case yaml.ScalarNode:
				id := strings.TrimSpace(v.Value)
				if id != "" {
					cfg.Bundles = append(cfg.Bundles, id)
				}
			case yaml.SequenceNode:
				cfg.Bundles = append(cfg.Bundles, asStrings(v)...)
			}
		case "toolcallexposure", "toolexposure", "callexposure":
			if v.Kind == yaml.ScalarNode {
				exp := agentmdl.ToolCallExposure(strings.ToLower(strings.TrimSpace(v.Value)))
				cfg.CallExposure = exp
				// Also map to top-level for backward compatibility.
				agent.Tool.CallExposure = exp
				agent.ToolCallExposure = exp
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// Parse items with legacy logic for consistency
	if itemsNode != nil {
		if err := s.parseToolBlock(itemsNode, agent); err != nil {
			return err
		}
		cfg.Items = agent.Tool.Items
	}
	cfg.Bundles = normalizeStrings(cfg.Bundles)
	agent.Tool = cfg
	return nil
}

func normalizeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
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

// parseInt parses a base-10 integer from string, ignoring spaces.
func parseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	var n int64
	var err error
	// Support plain numbers only.
	n, err = parseInt64(s)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (s *Service) parseProfileBlock(valueNode *yml.Node, agent *agentmdl.Agent) error {
	if valueNode.Kind != yaml.MappingNode {
		return fmt.Errorf("profile must be a mapping")
	}
	prof := &agentmdl.Profile{}
	if err := valueNode.Pairs(func(k string, v *yml.Node) error {
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "publish":
			if v.Kind == yaml.ScalarNode {
				prof.Publish = toBool(v.Value)
			}
		case "name":
			if v.Kind == yaml.ScalarNode {
				prof.Name = v.Value
			}
		case "description":
			if v.Kind == yaml.ScalarNode {
				prof.Description = v.Value
			}
		case "tags":
			if v != nil {
				prof.Tags = asStrings(v)
			}
		case "rank":
			if v.Kind == yaml.ScalarNode {
				val := v.Interface()
				switch actual := val.(type) {
				case int:
					prof.Rank = actual
				case int64:
					prof.Rank = int(actual)
				case float64:
					prof.Rank = int(actual)
				case string:
					if n, err := parseInt64(actual); err == nil {
						prof.Rank = int(n)
					}
				}
			}
		case "capabilities":
			if v.Kind == yaml.MappingNode {
				var m map[string]interface{}
				_ = (*yaml.Node)(v).Decode(&m)
				prof.Capabilities = m
			}
		case "responsibilities":
			if v != nil {
				prof.Responsibilities = asStrings(v)
			}
		case "inscope":
			if v != nil {
				prof.InScope = asStrings(v)
			}
		case "conversationscope":
			if v != nil {
				prof.ConversationScope = v.Value
			}
		case "outofscope":
			if v != nil {
				prof.OutOfScope = asStrings(v)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	agent.Profile = prof
	return nil
}

func (s *Service) parseServeBlock(valueNode *yml.Node, agent *agentmdl.Agent) error {
	if valueNode.Kind != yaml.MappingNode {
		return fmt.Errorf("serve must be a mapping")
	}
	var srv agentmdl.Serve
	if err := valueNode.Pairs(func(k string, v *yml.Node) error {
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "a2a":
			if v.Kind != yaml.MappingNode {
				return fmt.Errorf("serve.a2a must be a mapping")
			}
			a2a := &agentmdl.ServeA2A{}
			if err := v.Pairs(func(ak string, av *yml.Node) error {
				switch strings.ToLower(strings.TrimSpace(ak)) {
				case "enabled":
					if av.Kind == yaml.ScalarNode {
						a2a.Enabled = toBool(av.Value)
					}
				case "port":
					if av.Kind == yaml.ScalarNode {
						val := av.Interface()
						switch actual := val.(type) {
						case int:
							a2a.Port = actual
						case int64:
							a2a.Port = int(actual)
						case float64:
							a2a.Port = int(actual)
						case string:
							if n, err := parseInt64(actual); err == nil {
								a2a.Port = int(n)
							}
						}
					}
				case "streaming":
					if av.Kind == yaml.ScalarNode {
						a2a.Streaming = toBool(av.Value)
					}
				case "auth":
					if av.Kind != yaml.MappingNode {
						return fmt.Errorf("serve.a2a.auth must be a mapping")
					}
					a := &agentmdl.A2AAuth{}
					_ = av.Pairs(func(k2 string, v2 *yml.Node) error {
						switch strings.ToLower(strings.TrimSpace(k2)) {
						case "enabled":
							if v2.Kind == yaml.ScalarNode {
								a.Enabled = toBool(v2.Value)
							}
						case "resource":
							if v2.Kind == yaml.ScalarNode {
								a.Resource = v2.Value
							}
						case "scopes":
							a.Scopes = asStrings(v2)
						case "useidtoken":
							if v2.Kind == yaml.ScalarNode {
								a.UseIDToken = toBool(v2.Value)
							}
						case "excludeprefix":
							if v2.Kind == yaml.ScalarNode {
								a.ExcludePrefix = v2.Value
							}
						}
						return nil
					})
					a2a.Auth = a
				}
				return nil
			}); err != nil {
				return err
			}
			srv.A2A = a2a
		}
		return nil
	}); err != nil {
		return err
	}
	agent.Serve = &srv
	return nil
}

func (s *Service) parseExposeA2ABlock(valueNode *yml.Node, agent *agentmdl.Agent) error {
	if valueNode.Kind != yaml.MappingNode {
		return fmt.Errorf("exposeA2A must be a mapping")
	}
	exp := &agentmdl.ExposeA2A{}
	if err := valueNode.Pairs(func(k string, v *yml.Node) error {
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "enabled":
			if v.Kind == yaml.ScalarNode {
				exp.Enabled = toBool(v.Value)
			}
		case "port":
			if v.Kind == yaml.ScalarNode {
				val := v.Interface()
				switch actual := val.(type) {
				case int:
					exp.Port = actual
				case int64:
					exp.Port = int(actual)
				case float64:
					exp.Port = int(actual)
				case string:
					if n, err := parseInt64(actual); err == nil {
						exp.Port = int(n)
					}
				}
			}
		case "basepath":
			if v.Kind == yaml.ScalarNode {
				exp.BasePath = strings.TrimSpace(v.Value)
			}
		case "streaming":
			if v.Kind == yaml.ScalarNode {
				exp.Streaming = toBool(v.Value)
			}
		case "auth":
			if v.Kind != yaml.MappingNode {
				return fmt.Errorf("exposeA2A.auth must be a mapping")
			}
			a := &agentmdl.A2AAuth{}
			if err := v.Pairs(func(ak string, av *yml.Node) error {
				switch strings.ToLower(strings.TrimSpace(ak)) {
				case "enabled":
					if av.Kind == yaml.ScalarNode {
						a.Enabled = toBool(av.Value)
					}
				case "resource":
					if av.Kind == yaml.ScalarNode {
						a.Resource = av.Value
					}
				case "scopes":
					if av != nil {
						a.Scopes = asStrings(av)
					}
				case "useidtoken":
					if av.Kind == yaml.ScalarNode {
						a.UseIDToken = toBool(av.Value)
					}
				case "excludeprefix":
					if av.Kind == yaml.ScalarNode {
						a.ExcludePrefix = av.Value
					}
				}
				return nil
			}); err != nil {
				return err
			}
			exp.Auth = a
		}
		return nil
	}); err != nil {
		return err
	}
	agent.ExposeA2A = exp
	if agent.Serve == nil {
		agent.Serve = &agentmdl.Serve{}
	}
	agent.Serve.A2A = &agentmdl.ServeA2A{Enabled: exp.Enabled, Port: exp.Port, Streaming: exp.Streaming, Auth: exp.Auth}
	return nil
}

func (s *Service) parseAttachmentBlock(valueNode *yml.Node, agent *agentmdl.Agent) error {
	if valueNode.Kind != yaml.MappingNode {
		return fmt.Errorf("attachment must be a mapping")
	}
	var cfg agentmdl.Attachment
	if err := valueNode.Pairs(func(k string, v *yml.Node) error {
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "limit", "limitbytes", "limitbytesmb", "limitmb":
			if v.Kind == yaml.ScalarNode {
				val := v.Interface()
				switch a := val.(type) {
				case int:
					cfg.LimitBytes = int64(a)
					if strings.Contains(strings.ToLower(k), "mb") {
						cfg.LimitBytes *= 1024 * 1024
					}
				case int64:
					cfg.LimitBytes = a
					if strings.Contains(strings.ToLower(k), "mb") {
						cfg.LimitBytes *= 1024 * 1024
					}
				case float64:
					cfg.LimitBytes = int64(a)
					if strings.Contains(strings.ToLower(k), "mb") {
						cfg.LimitBytes *= 1024 * 1024
					}
				case string:
					lv := strings.TrimSpace(strings.ToLower(a))
					mul := int64(1)
					if strings.HasSuffix(lv, "mb") {
						lv = strings.TrimSuffix(lv, "mb")
						mul = 1024 * 1024
					}
					if n, err := parseInt64(lv); err == nil {
						cfg.LimitBytes = n * mul
					}
				}
			}
		case "mode":
			if v.Kind == yaml.ScalarNode {
				m := strings.ToLower(strings.TrimSpace(v.Value))
				if m != "inline" {
					m = "ref"
				}
				cfg.Mode = m
			}
		case "ttlsec":
			if v.Kind == yaml.ScalarNode {
				val := v.Interface()
				switch a := val.(type) {
				case int:
					cfg.TTLSec = int64(a)
				case int64:
					cfg.TTLSec = a
				case float64:
					cfg.TTLSec = int64(a)
				case string:
					if n, err := parseInt64(a); err == nil {
						cfg.TTLSec = n
					}
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	agent.Attachment = &cfg
	return nil
}

func (s *Service) parseChainsBlock(valueNode *yml.Node, agent *agentmdl.Agent) error {
	if valueNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("chains must be a sequence")
	}
	for _, item := range valueNode.Content {
		if item == nil || item.Kind != yaml.MappingNode {
			return fmt.Errorf("invalid chain entry; expected mapping")
		}
		var c agentmdl.Chain
		var whenExpr string
		for i := 0; i+1 < len(item.Content); i += 2 {
			k := strings.ToLower(strings.TrimSpace(item.Content[i].Value))
			if k == "when" {
				v := item.Content[i+1]
				if v != nil && v.Kind == yaml.ScalarNode {
					whenExpr = v.Value
					// Replace scalar with empty mapping so YAML can decode into WhenSpec.
					item.Content[i+1] = &yaml.Node{Kind: yaml.MappingNode}
				}
				break
			}
		}
		if err := (*yaml.Node)(item).Decode(&c); err != nil {
			return fmt.Errorf("invalid chain definition: %w", err)
		}
		if c.Query == nil {
			for i := 0; i+1 < len(item.Content); i += 2 {
				k := strings.ToLower(strings.TrimSpace(item.Content[i].Value))
				if k == "query" {
					v := item.Content[i+1]
					if v.Kind == yaml.ScalarNode {
						c.Query = &prompt.Prompt{Text: v.Value}
					}
					break
				}
			}
		}
		// Normalize scalar when: into WhenSpec.Expr when present.
		if strings.TrimSpace(whenExpr) != "" {
			if c.When == nil {
				c.When = &agentmdl.WhenSpec{Expr: whenExpr}
			} else if strings.TrimSpace(c.When.Expr) == "" && c.When.Query == nil && c.When.Expect == nil && strings.TrimSpace(c.When.Model) == "" {
				c.When.Expr = whenExpr
			}
		}
		if strings.TrimSpace(c.Conversation) == "" {
			c.Conversation = "link"
		}
		switch strings.ToLower(strings.TrimSpace(c.Conversation)) {
		case "reuse", "link":
		case "":
		default:
			return fmt.Errorf("invalid chain.conversation: %s", c.Conversation)
		}
		agent.Chains = append(agent.Chains, &c)
	}
	return nil
}

// parseMCPResourcesBlock removed — agent.resources replaces it.

// parseResourcesBlock parses agent.resources entries (list of mappings)
func (s *Service) parseResourcesBlock(valueNode *yml.Node, agent *agentmdl.Agent) error {
	switch valueNode.Kind {
	case yaml.SequenceNode:
		for _, it := range valueNode.Content {
			if it == nil || it.Kind != yaml.MappingNode {
				continue
			}
			entry, err := parseResourceEntry((*yml.Node)(it))
			if err != nil {
				return err
			}
			if entry != nil {
				agent.Resources = append(agent.Resources, entry)
			}
		}
	case yaml.MappingNode:
		// Support single entry mapping for convenience
		entry, err := parseResourceEntry((*yml.Node)(valueNode))
		if err != nil {
			return err
		}
		if entry != nil {
			agent.Resources = append(agent.Resources, entry)
		}
	default:
		return fmt.Errorf("resources must be a sequence or mapping")
	}
	return nil
}

func parseResourceEntry(node *yml.Node) (*agentmdl.Resource, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("resource entry must be a mapping")
	}
	re := &agentmdl.Resource{Role: "user"}
	roleExplicit := false
	systemFlagSet := false
	systemRole := "user"
	err := node.Pairs(func(key string, v *yml.Node) error {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "id":
			if v.Kind == yaml.ScalarNode {
				re.ID = strings.TrimSpace(v.Value)
			}
		case "uri":
			if v.Kind == yaml.ScalarNode {
				re.URI = strings.TrimSpace(v.Value)
			}
		case "role":
			if v.Kind == yaml.ScalarNode {
				re.Role = strings.ToLower(strings.TrimSpace(v.Value))
				roleExplicit = true
			}
		case "system":
			enabled, handled, err := parseSystemFlag(v)
			if err != nil {
				return err
			}
			if handled {
				systemFlagSet = true
				if enabled {
					systemRole = "system"
				} else {
					systemRole = "user"
				}
			}
		case "binding":
			if v.Kind == yaml.ScalarNode {
				re.Binding = toBool(v.Value)
			}
		case "description":
			if v.Kind == yaml.ScalarNode {
				re.Description = strings.TrimSpace(v.Value)
			}
		case "allowsemanticmatch":
			if v.Kind == yaml.ScalarNode {
				b := toBool(v.Value)
				re.AllowSemanticMatch = &b
			}
		case "allowgrep":
			if v.Kind == yaml.ScalarNode {
				b := toBool(v.Value)
				re.AllowGrep = &b
			}
		case "maxfiles":
			if v.Kind == yaml.ScalarNode {
				val := v.Interface()
				switch actual := val.(type) {
				case int:
					re.MaxFiles = actual
				case int64:
					re.MaxFiles = int(actual)
				case float64:
					re.MaxFiles = int(actual)
				case string:
					if n, err := parseInt64(actual); err == nil {
						re.MaxFiles = int(n)
					}
				}
			}
		case "trimpath":
			if v.Kind == yaml.ScalarNode {
				re.TrimPath = v.Value
			}
		case "match":
			if v.Kind == yaml.MappingNode {
				m := &option.Options{}
				_ = v.Pairs(func(optKey string, optValue *yml.Node) error {
					switch strings.ToLower(optKey) {
					case "exclusions":
						m.Exclusions = asStrings(optValue)
					case "inclusions":
						m.Inclusions = asStrings(optValue)
					case "maxfilesize":
						switch vv := optValue.Interface().(type) {
						case int:
							m.MaxFileSize = vv
						case int64:
							m.MaxFileSize = int(vv)
						case float64:
							m.MaxFileSize = int(vv)
						case string:
							if n, err := parseInt64(vv); err == nil {
								m.MaxFileSize = int(n)
							}
						}
					}
					return nil
				})
				re.Match = m
			}
		case "minscore":
			if v.Kind == yaml.ScalarNode {
				val := v.Interface()
				var f float64
				switch actual := val.(type) {
				case float64:
					f = actual
				case int:
					f = float64(actual)
				case int64:
					f = float64(actual)
				case string:
					var parsed float64
					if _, err := fmt.Sscan(strings.TrimSpace(actual), &parsed); err == nil {
						f = parsed
					}
				}
				re.MinScore = &f
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if systemFlagSet {
		if !roleExplicit {
			re.Role = systemRole
		} else if !strings.EqualFold(strings.TrimSpace(re.Role), systemRole) {
			return nil, fmt.Errorf("resource entry role %q conflicts with system=%v", re.Role, systemRole == "system")
		}
	}
	re.URI = expandUserHome(re.URI)
	if strings.TrimSpace(re.URI) == "" {
		return nil, fmt.Errorf("resource entry missing uri")
	}
	return re, nil
}

func parseSystemFlag(node *yml.Node) (enabled bool, handled bool, err error) {
	if node == nil {
		return false, false, nil
	}
	val := node.Interface()
	switch actual := val.(type) {
	case bool:
		return actual, true, nil
	case string:
		return toBool(actual), true, nil
	default:
		return false, false, fmt.Errorf("system flag must be a boolean or string")
	}
}

// expandUserHome expands leading ~ in filesystem-like URIs to the current
// user's home directory. It supports forms such as:
//   - "~/path"
//   - "file://localhost/~/path"
//   - "file:///~/path"
//
// For non-file schemes (e.g. mcp:), the input is returned unchanged.
func expandUserHome(u string) string {
	trimmed := strings.TrimSpace(u)
	if trimmed == "" {
		return u
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return u
	}
	// Direct ~/path use
	if strings.HasPrefix(trimmed, "~/") || trimmed == "~" {
		return filepath.Join(home, strings.TrimPrefix(trimmed, "~"))
	}
	// file:// URI forms
	if strings.HasPrefix(trimmed, "file://") {
		prefix := "file://localhost"
		rest := strings.TrimPrefix(trimmed, prefix)
		if rest == trimmed { // no localhost prefix
			prefix = "file://"
			rest = strings.TrimPrefix(trimmed, prefix)
		}
		if rest == "" {
			return u
		}
		// Normalize leading slash and expand ~/...
		// Accept both /~/... and ~/...
		rest = strings.TrimLeft(rest, "/")
		if strings.HasPrefix(rest, "~") {
			rel := strings.TrimPrefix(rest, "~") // leading / remains trimmed
			abs := filepath.Join(home, rel)
			// Reconstruct as file://localhost/abs or file://abs
			return prefix + "/" + filepath.ToSlash(strings.TrimLeft(abs, "/"))
		}
	}
	return u
}

// augmentResourcesWithKnowledge ensures that every knowledge URL is reflected
// as a Resource entry with semantic match enabled. When a matching resource
// already exists (URI match after normalization), its AllowSemanticMatch flag
// is forced to true to guarantee that knowledge roots are always eligible for
// semantic search via tools.
func augmentResourcesWithKnowledge(agent *agentmdl.Agent) {
	if agent == nil {
		return
	}
	// helper to upsert a resource for the given URL and role
	ensure := func(url, role string) {
		u := strings.TrimSpace(url)
		if u == "" {
			return
		}
		norm := strings.TrimRight(u, "/")
		// Try to find existing resource by URI
		for _, r := range agent.Resources {
			if r == nil || strings.TrimSpace(r.URI) == "" {
				continue
			}
			cur := strings.TrimRight(strings.TrimSpace(r.URI), "/")
			if cur == norm {
				// Force semantic match enabled for knowledge-backed resources
				b := true
				r.AllowSemanticMatch = &b
				return
			}
		}
		// No existing resource; create a new one with semantic match enabled.
		b := true
		agent.Resources = append(agent.Resources, &agentmdl.Resource{
			URI:                u,
			Role:               role,
			AllowSemanticMatch: &b,
		})
	}
	for _, k := range agent.Knowledge {
		if k == nil || strings.TrimSpace(k.URL) == "" {
			continue
		}
		ensure(k.URL, "user")
	}
	for _, k := range agent.SystemKnowledge {
		if k == nil || strings.TrimSpace(k.URL) == "" {
			continue
		}
		ensure(k.URL, "system")
	}
}

// parseInt64 parses an integer from string, trimming spaces; returns error on failure.
func parseInt64(s string) (int64, error) {
	s = strings.TrimSpace(s)
	var n int64
	var err error
	// yaml already converts numeric scalars to int/float, but we support strings too
	_, err = fmt.Sscan(s, &n)
	return n, err
}

func toBool(s string) bool {
	lv := strings.ToLower(strings.TrimSpace(s))
	return lv == "true" || lv == "yes" || lv == "on" || lv == "1"
}

func (s *Service) getPrompt(valueNode *yml.Node) (*prompt.Prompt, error) {
	var aPrompt *prompt.Prompt

	if valueNode.Kind == yaml.ScalarNode {
		aPrompt = &prompt.Prompt{
			Text: valueNode.Value,
		}
		inferPromptEngine(aPrompt)

	} else if valueNode.Kind == yaml.MappingNode {
		var err error
		if aPrompt, err = parsePrompt((*yml.Node)(valueNode)); err != nil {
			return nil, err
		}
	}

	return aPrompt, nil
}

func parsePrompt(y *yml.Node) (*prompt.Prompt, error) {
	if y == nil {
		return &prompt.Prompt{}, nil
	}
	if y.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("prompt node should be a mapping")
	}

	p := &prompt.Prompt{}
	// Collect primary fields with a forgiving schema: text/content, uri/url/path, engine/type
	if err := y.Pairs(func(key string, v *yml.Node) error {
		k := strings.ToLower(strings.TrimSpace(key))
		switch k {
		case "text", "content":
			if v.Kind == yaml.ScalarNode {
				p.Text = v.Value
			}
		case "uri", "url", "path", "file":
			if v.Kind == yaml.ScalarNode {
				p.URI = v.Value
			}
		case "engine", "type":
			if v.Kind == yaml.ScalarNode {
				p.Engine = strings.ToLower(strings.TrimSpace(v.Value))
			}
		default:
			// tolerate unknown keys for forward compatibility
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Infer engine via shared helper to avoid duplication.
	inferPromptEngine(p)
	return p, nil
}

// inferPromptEngine sets prompt.Engine if empty using URI suffixes or inline
// text markers. Recognizes .vm => "vm" and .gotmpl/.tmpl => "go". As a
// fallback, detects "{{ ... }}" => go and "$" => vm.
func inferPromptEngine(p *prompt.Prompt) {
	if p == nil || strings.TrimSpace(p.Engine) != "" {
		return
	}
	if u := strings.TrimSpace(p.URI); u != "" {
		cand := u
		if strings.HasPrefix(cand, "$path(") && strings.HasSuffix(cand, ")") {
			cand = strings.TrimSuffix(strings.TrimPrefix(cand, "$path("), ")")
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(cand), "."))
		switch ext {
		case "vm":
			p.Engine = "vm"
		case "gotmpl", "tmpl":
			p.Engine = "go"
		}
	}
	if strings.TrimSpace(p.Engine) == "" {
		if strings.Contains(p.Text, "{{") && strings.Contains(p.Text, "}}") {
			p.Engine = "go"
		} else if strings.Contains(p.Text, "$") {
			p.Engine = "vm"
		}
	}
}

// parseKnowledge parses a knowledge entry from a YAML node
func parseKnowledge(node *yml.Node) (*agentmdl.Knowledge, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("knowledge node should be a mapping")
	}

	knowledge := &agentmdl.Knowledge{}

	err := node.Pairs(func(key string, valueNode *yml.Node) error {
		lowerKey := strings.ToLower(key)
		switch lowerKey {
		case "description":
			if valueNode.Kind == yaml.ScalarNode {
				knowledge.Description = valueNode.Value
			}
		case "inclusionmode":
			if valueNode.Kind == yaml.ScalarNode {
				knowledge.InclusionMode = valueNode.Value
			}
		case "locations", "url":
			switch valueNode.Kind {
			case yaml.ScalarNode:
				knowledge.URL = valueNode.Value
			case yaml.SequenceNode:
				var locations []string
				for _, locNode := range valueNode.Content {
					if locNode.Kind == yaml.ScalarNode {
						locations = append(locations, locNode.Value)
					}
				}
				if len(locations) > 0 {
					// For backward compatibility, store the first location in URL field
					knowledge.URL = locations[0]
				}
			}
		case "filter", "match":
			if valueNode.Kind == yaml.MappingNode {
				opts := &option.Options{}
				// Parse filtering options if provided
				_ = valueNode.Pairs(func(optKey string, optValue *yml.Node) error {
					switch strings.ToLower(optKey) {
					case "exclusions":
						opts.Exclusions = asStrings(optValue)
					case "inclusions":
						opts.Inclusions = asStrings(optValue)
					case "maxfilesize":
						opts.MaxFileSize = int(optValue.Interface().(int64))
					}
					return nil
				})
				knowledge.Filter = opts
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return knowledge, nil
}

func asStrings(optValue *yml.Node) []string {
	value := optValue.Interface()
	switch actual := value.(type) {
	case []string:
		return actual
	case []interface{}:
		var result = make([]string, 0)
		for _, item := range actual {
			result = append(result, fmt.Sprintf("%v", item))
		}
		return result
	}
	return nil
}

// getAgentNameFromURL extracts agent name from URL (file name without extension)
func getAgentNameFromURL(URL string) string {
	base := filepath.Base(URL)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// New creates a new agent service instance
func New(opts ...Option) *Service {
	ret := &Service{
		metaService:      meta.New(afs.New(), ""),
		defaultExtension: defaultExtension,
		agents:           shared.NewMap[string, *agentmdl.Agent](),
	}
	for _, opt := range opts {
		opt(ret)
	}
	return ret
}
