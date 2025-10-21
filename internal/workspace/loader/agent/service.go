package agent

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
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
		ok, _ := s.metaService.Exists(ctx, URL+ext)
		if ok {
			URL = s.metaService.GetURL(URL + ext)
		} else {
			candidate := path.Join(URL, nameOrLocation)
			if ok, _ = s.metaService.Exists(ctx, candidate+ext); ok {
				URL = s.metaService.GetURL(candidate + ext)
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
		if url.IsRelative(knowledge.URL) && !url.IsRelative(srcURL) {
			parentURL, _ := url.Split(srcURL, file.Scheme)
			anAgent.Knowledge[i].URL = url.JoinUNC(parentURL, knowledge.URL)
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
		if url.IsRelative(knowledge.URL) && !url.IsRelative(srcURL) {
			parentURL, _ := url.Split(srcURL, file.Scheme)
			anAgent.SystemKnowledge[i].URL = url.JoinUNC(parentURL, knowledge.URL)
		}
		// Validate that system knowledge path exists
		if ok, _ := s.metaService.Exists(ctx, anAgent.SystemKnowledge[i].URL); !ok {
			return nil, fmt.Errorf("agent %v system knowledge path does not exist: %s", anAgent.Name, anAgent.SystemKnowledge[i].URL)
		}
	}

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
		case "knowledge":
			if err := s.parseKnowledgeBlock(valueNode, agent); err != nil {
				return err
			}
		case "systemknowledge":
			if err := s.parseSystemKnowledgeBlock(valueNode, agent); err != nil {
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
			if err := s.parseToolBlock(valueNode, agent); err != nil {
				return err
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
					agent.Tool = append(agent.Tool, &llm.Tool{Pattern: name, Type: "function"})

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
					agent.Tool = append(agent.Tool, &llm.Tool{Pattern: t.Pattern, Ref: t.Ref, Type: t.Type, Definition: t.Definition})

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
		case "toolexposure", "toolcallexposure":
			// Accept scalar values: turn | conversation | semantic
			if valueNode.Kind == yaml.ScalarNode {
				agent.ToolCallExposure = agentmdl.ToolCallExposure(strings.ToLower(strings.TrimSpace(valueNode.Value)))
			}
		case "attachment":
			if err := s.parseAttachmentBlock(valueNode, agent); err != nil {
				return err
			}
		case "chains":
			if err := s.parseChainsBlock(valueNode, agent); err != nil {
				return err
			}
		case "mcpresources":
			if err := s.parseMCPResourcesBlock(valueNode, agent); err != nil {
				return err
			}
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
			agent.Tool = append(agent.Tool, &llm.Tool{Pattern: v})
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
				agent.Tool = append(agent.Tool, &t)
			}
		}
	}
	return nil
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
		if c.When == nil {
			for i := 0; i+1 < len(item.Content); i += 2 {
				k := strings.ToLower(strings.TrimSpace(item.Content[i].Value))
				if k == "when" {
					v := item.Content[i+1]
					if whenExpr != "" {
						c.When = &agentmdl.WhenSpec{Expr: whenExpr}
					} else if v.Kind == yaml.ScalarNode && strings.TrimSpace(v.Value) != "" {
						c.When = &agentmdl.WhenSpec{Expr: v.Value}
					}
					break
				}
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

func (s *Service) parseMCPResourcesBlock(valueNode *yml.Node, agent *agentmdl.Agent) error {
	if valueNode.Kind != yaml.MappingNode {
		return fmt.Errorf("mcpResources must be a mapping")
	}
	var cfg agentmdl.MCPResources
	cfg.MaxFiles = 5
	if err := valueNode.Pairs(func(k string, v *yml.Node) error {
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "enabled":
			if v.Kind == yaml.ScalarNode {
				cfg.Enabled = toBool(v.Value)
			}
		case "locations":
			switch v.Kind {
			case yaml.ScalarNode:
				if s := strings.TrimSpace(v.Value); s != "" {
					cfg.Locations = []string{s}
				}
			case yaml.SequenceNode:
				for _, it := range v.Content {
					if it != nil && it.Kind == yaml.ScalarNode && strings.TrimSpace(it.Value) != "" {
						cfg.Locations = append(cfg.Locations, it.Value)
					}
				}
			}
		case "maxfiles":
			if v.Kind == yaml.ScalarNode {
				val := v.Interface()
				switch actual := val.(type) {
				case int:
					cfg.MaxFiles = actual
				case int64:
					cfg.MaxFiles = int(actual)
				case float64:
					cfg.MaxFiles = int(actual)
				case string:
					if n, err := parseInt64(actual); err == nil {
						cfg.MaxFiles = int(n)
					}
				}
			}
		case "trimpath":
			if v.Kind == yaml.ScalarNode {
				cfg.TrimPath = v.Value
			}
		case "match":
			if v.Kind == yaml.MappingNode {
				match := &option.Options{}
				_ = v.Pairs(func(optKey string, optValue *yml.Node) error {
					switch strings.ToLower(optKey) {
					case "exclusions":
						match.Exclusions = asStrings(optValue)
					case "inclusions":
						match.Inclusions = asStrings(optValue)
					case "maxfilesize":
						switch vv := optValue.Interface().(type) {
						case int:
							match.MaxFileSize = vv
						case int64:
							match.MaxFileSize = int(vv)
						case float64:
							match.MaxFileSize = int(vv)
						case string:
							if n, err := parseInt64(vv); err == nil {
								match.MaxFileSize = int(n)
							}
						}
					}
					return nil
				})
				cfg.Match = match
			}
		}
		return nil
	}); err != nil {
		return err
	}
	agent.MCPResources = &cfg
	return nil
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
		case "match":
			if valueNode.Kind == yaml.MappingNode {

				match := &option.Options{}
				// Parse matching options if provided
				_ = valueNode.Pairs(func(optKey string, optValue *yml.Node) error {
					switch strings.ToLower(optKey) {
					case "exclusions":
						match.Exclusions = asStrings(optValue)
					case "inclusions":
						match.Inclusions = asStrings(optValue)
					case "maxfilesize":
						match.MaxFileSize = int(optValue.Interface().(int64))
					}
					return nil
				})
				knowledge.Match = match
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
