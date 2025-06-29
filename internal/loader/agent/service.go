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
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/internal/shared"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/embedius/matching/option"
	"github.com/viant/fluxor/service/meta"
	"github.com/viant/fluxor/service/meta/yml"

	"gopkg.in/yaml.v3"
)

// Ensure Service implements interfaces.Loader so that changes are caught by
// the compiler.
var _ agent.Loader = (*Service)(nil)

const (
	defaultExtension = ".yaml"
)

// Service provides agent data access operations
type Service struct {
	metaService *meta.Service
	agents      shared.Map[string, *agent.Agent] //[ url ] -> [ agent]

	defaultExtension string
}

// LoadAgents loads an agents from the specified URL
func (s *Service) LoadAgents(ctx context.Context, URL string) ([]*agent.Agent, error) {
	candidates, err := s.metaService.List(ctx, URL)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent from %s: %w", URL, err)
	}
	var result []*agent.Agent
	for _, candidate := range candidates {
		anAgent, err := s.Load(ctx, candidate)
		if err != nil {
			return nil, fmt.Errorf("failed to load agent from %s: %w", candidate, err)
		}
		result = append(result, anAgent)
	}
	return result, nil

}

func (s *Service) List() []*agent.Agent {
	result := make([]*agent.Agent, 0)
	s.agents.Range(
		func(key string, value *agent.Agent) bool {
			result = append(result, value)
			return true
		})
	return result
}

// Add adds an agent to the service
func (s *Service) Add(name string, agent *agent.Agent) {
	s.agents.Set(name, agent)
}

// Lookup looks up an agent by name
func (s *Service) Lookup(ctx context.Context, name string) (*agent.Agent, error) {
	anAgent, ok := s.agents.Get(name)
	if !ok {
		return nil, fmt.Errorf("agent %q not found", name)
	}
	return anAgent, nil
}

// Load loads an agent from the specified URL
func (s *Service) Load(ctx context.Context, nameOrLocation string) (*agent.Agent, error) {
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

	anAgent := &agent.Agent{
		Source: &agent.Source{
			URL: URL,
		},
	}
	// Parse the YAML into our agent model
	if err := s.parseAgent((*yml.Node)(&node), anAgent); err != nil {
		return nil, fmt.Errorf("failed to parse agent from %s: %w", URL, err)
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

	// Validate agent
	if err := anAgent.Validate(); err != nil {
		return nil, fmt.Errorf("invalid agent configuration from %s: %w", URL, err)
	}

	s.agents.Set(anAgent.Name, anAgent)
	return anAgent, nil
}

// parseAgent parses agent properties from a YAML node
func (s *Service) parseAgent(node *yml.Node, agent *agent.Agent) error {
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
		case "orchestrationflow":
			if valueNode.Kind == yaml.ScalarNode {
				agent.OrchestrationFlow = valueNode.Value
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
		case "elicitation":
			// Accept mapping that follows plan.Elicitation schema.
			if valueNode.Kind != yaml.MappingNode {
				return fmt.Errorf("elicitation must be a mapping")
			}
			var elic plan.Elicitation
			if err := (*yaml.Node)(valueNode).Decode(&elic.ElicitRequestParams); err != nil {
				return fmt.Errorf("invalid elicitation definition: %w", err)
			}
			agent.Elicitation = &elic
		}
		return nil
	})
}

// parseKnowledge parses a knowledge entry from a YAML node
func parseKnowledge(node *yml.Node) (*agent.Knowledge, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("knowledge node should be a mapping")
	}

	knowledge := &agent.Knowledge{}

	err := node.Pairs(func(key string, valueNode *yml.Node) error {
		lowerKey := strings.ToLower(key)
		switch lowerKey {
		case "description":
			if valueNode.Kind == yaml.ScalarNode {
				knowledge.Description = valueNode.Value
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
		agents:           shared.NewMap[string, *agent.Agent](),
	}
	for _, opt := range opts {
		opt(ret)
	}
	return ret
}
