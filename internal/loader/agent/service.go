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
	"github.com/viant/embedius/matching/option"
	"github.com/viant/fluxor/service/meta"
	"github.com/viant/fluxor/service/meta/yml"

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

	anAgent := &agentmdl.Agent{
		Source: &agentmdl.Source{
			URL: URL,
		},
	}
	// Parse the YAML into our agent model
	if err := s.parseAgent((*yml.Node)(&node), anAgent); err != nil {
		return nil, fmt.Errorf("failed to parse agent from %s: %w", URL, err)
	}
	// Validate parsed agent
	if err := anAgent.Validate(); err != nil {
		return nil, fmt.Errorf("invalid agent %s: %w", URL, err)
	}

	// Set agent name based on URL if not set
	if anAgent.Name == "" {
		anAgent.Name = getAgentNameFromURL(URL)
	}

	for i := range anAgent.Knowledge {
		knowledge := anAgent.Knowledge[i]
		if knowledge.URL == "" {
			return nil, fmt.Errorf("agent %v knowledge URL is empty", anAgent.Name)
		}
		if url.IsRelative(knowledge.URL) && !url.IsRelative(URL) {
			parentURL, _ := url.Split(URL, file.Scheme)
			anAgent.Knowledge[i].URL = url.Join(parentURL, knowledge.URL)
		}
	}

	for i := range anAgent.SystemKnowledge {
		knowledge := anAgent.SystemKnowledge[i]
		if knowledge.URL == "" {
			return nil, fmt.Errorf("agent %v system knowledge URL is empty", anAgent.Name)
		}
		if url.IsRelative(knowledge.URL) && !url.IsRelative(URL) {
			parentURL, _ := url.Split(URL, file.Scheme)
			anAgent.SystemKnowledge[i].URL = url.Join(parentURL, knowledge.URL)
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
	fix := func(p *prompt.Prompt) {
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
	fix(a.Prompt)
	fix(a.SystemPrompt)
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
		case "attachmentttlsec":
			if valueNode.Kind == yaml.ScalarNode {
				v := valueNode.Interface()
				switch actual := v.(type) {
				case int:
					agent.AttachmentTTLSec = int64(actual)
				case int64:
					agent.AttachmentTTLSec = actual
				case float64:
					agent.AttachmentTTLSec = int64(actual)
				case string:
					if n, err := parseInt64(actual); err == nil {
						agent.AttachmentTTLSec = n
					}
				}
			}
		case "attachmentlimitbytes":
			if valueNode.Kind == yaml.ScalarNode {
				v := valueNode.Interface()
				switch actual := v.(type) {
				case int:
					agent.AttachmentLimitBytes = int64(actual)
				case int64:
					agent.AttachmentLimitBytes = actual
				case float64:
					agent.AttachmentLimitBytes = int64(actual)
				case string:
					lv := strings.TrimSpace(strings.ToLower(actual))
					if strings.HasSuffix(lv, "mb") {
						if n, err := parseInt64(strings.TrimSuffix(lv, "mb")); err == nil {
							agent.AttachmentLimitBytes = n * 1024 * 1024
						}
					} else if n, err := parseInt64(lv); err == nil {
						agent.AttachmentLimitBytes = n
					}
				}
			}
		case "attachmentlimitmb":
			if valueNode.Kind == yaml.ScalarNode {
				v := valueNode.Interface()
				switch actual := v.(type) {
				case int:
					agent.AttachmentLimitBytes = int64(actual) * 1024 * 1024
				case int64:
					agent.AttachmentLimitBytes = actual * 1024 * 1024
				case float64:
					agent.AttachmentLimitBytes = int64(actual * 1024 * 1024)
				case string:
					if n, err := parseInt64(actual); err == nil {
						agent.AttachmentLimitBytes = n * 1024 * 1024
					}
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
		case "knowledge":
			if valueNode.Kind == yaml.SequenceNode {
				for _, itemNode := range valueNode.Content {
					knowledge, err := parseKnowledge((*yml.Node)(itemNode))
					if err != nil {
						return err
					}
					agent.Knowledge = append(agent.Knowledge, knowledge)
				}
			}
		case "systemknowledge":
			if valueNode.Kind == yaml.SequenceNode {
				for _, itemNode := range valueNode.Content {
					knowledge, err := parseKnowledge((*yml.Node)(itemNode))
					if err != nil {
						return err
					}
					agent.SystemKnowledge = append(agent.SystemKnowledge, knowledge)
				}
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
			// Accept either
			//   tool:
			//     - my/tool # scalar → pattern/name reference
			//     - pattern: my/tool # mapping → inline definition (pattern & optional type)
			//     - ref: some/ref    # mapping – legacy/alternative key
			if valueNode.Kind != yaml.SequenceNode {
				return fmt.Errorf("tool must be a sequence")
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
		case "attachmode":
			if valueNode.Kind == yaml.ScalarNode {
				mode := strings.ToLower(strings.TrimSpace(valueNode.Value))
				switch mode {
				case "ref", "inline":
					agent.AttachMode = mode
				default:
					agent.AttachMode = "ref"
				}
			}
		case "chains":
			if valueNode.Kind != yaml.SequenceNode {
				return fmt.Errorf("chains must be a sequence")
			}
			for _, item := range valueNode.Content {
				if item == nil || item.Kind != yaml.MappingNode {
					return fmt.Errorf("invalid chain entry; expected mapping")
				}
				var c agentmdl.Chain
				if err := (*yaml.Node)(item).Decode(&c); err != nil {
					return fmt.Errorf("invalid chain definition: %w", err)
				}
				// Support scalar query by normalizing to prompt.Prompt{text: ...}
				if c.Query == nil {
					// search for 'query' key in the mapping
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
				// Defaults
				if strings.TrimSpace(c.Mode) == "" {
					c.Mode = "sync"
				}
				if strings.TrimSpace(c.Conversation) == "" {
					c.Conversation = "link" // default to child/linked workflow
				}
				// Validate enums
				switch strings.ToLower(strings.TrimSpace(c.Mode)) {
				case "sync", "async":
				case "": // unreachable due to default above
				default:
					return fmt.Errorf("invalid chain.mode: %s", c.Mode)
				}
				switch strings.ToLower(strings.TrimSpace(c.Conversation)) {
				case "reuse", "link":
				case "": // defaulted above
				default:
					return fmt.Errorf("invalid chain.conversation: %s", c.Conversation)
				}
				agent.Chains = append(agent.Chains, &c)
			}
		}
		return nil
	})
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
