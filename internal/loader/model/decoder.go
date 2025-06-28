package model

import (
	"fmt"
	"github.com/viant/agently/genai/llm/provider"
	"github.com/viant/fluxor/service/meta/yml"
	"gopkg.in/yaml.v3"
	"strings"
)

func decodeYaml(node *yml.Node, config *provider.Config) error {
	rootNode := node
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		rootNode = (*yml.Node)(node.Content[0])
	}

	// Look for the "config" root node
	var optionsNode *yml.Node
	err := rootNode.Pairs(func(key string, valueNode *yml.Node) error {

		switch strings.ToLower(key) {
		case "options":
			if valueNode.Kind == yaml.MappingNode {
				optionsNode = valueNode
			}
		case "id":
			config.ID = valueNode.Value
		case "description":
			config.Description = valueNode.Value
		}
		return nil
	})

	if err != nil {
		return err
	}

	if optionsNode == nil {
		optionsNode = rootNode // Use the root node if no "config" node is found
	}

	// Parse config properties
	return optionsNode.Pairs(func(key string, valueNode *yml.Node) error {
		lowerKey := strings.ToLower(key)
		switch lowerKey {
		case "id":
			if valueNode.Kind == yaml.ScalarNode {
				config.ID = valueNode.Value
			}
		case "description":
			if valueNode.Kind == yaml.ScalarNode {
				config.Description = valueNode.Value
			}
		case "provider":
			if valueNode.Kind == yaml.ScalarNode {
				config.Options.Provider = valueNode.Value
			}
		case "apikeyurl":
			if valueNode.Kind == yaml.ScalarNode {
				config.Options.APIKeyURL = valueNode.Value
			}
		case "credentialsurl":
			if valueNode.Kind == yaml.ScalarNode {
				config.Options.CredentialsURL = valueNode.Value
			}
		case "url":
			if valueNode.Kind == yaml.ScalarNode {
				config.Options.URL = valueNode.Value
			}
		case "projectid":
			if valueNode.Kind == yaml.ScalarNode {
				config.Options.ProjectID = valueNode.Value
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
				config.Options.Temperature = &temp
			}
		case "maxtokens":
			if valueNode.Kind == yaml.ScalarNode {
				value := valueNode.Interface()
				var tokens int
				switch actual := value.(type) {
				case int:
					tokens = actual
				case int64:
					tokens = int(actual)
				default:
					return fmt.Errorf("invalid max tokens value: %T %v", value, value)
				}
				config.Options.MaxTokens = tokens
			}
		case "model":
			if valueNode.Kind == yaml.ScalarNode {
				config.Options.Model = valueNode.Value
			}
		case "topp":
			if valueNode.Kind == yaml.ScalarNode {
				value := valueNode.Interface()
				topP := 0.0
				switch actual := value.(type) {
				case int:
					topP = float64(actual)
				case float64:
					topP = actual
				default:
					return fmt.Errorf("invalid topP value: %T %v", value, value)
				}
				config.Options.TopP = topP
			}
		case "meta":
			if valueNode.Kind == yaml.MappingNode {
				metadata := make(map[string]interface{})

				err := valueNode.Pairs(func(metaKey string, metaValue *yml.Node) error {
					metadata[metaKey] = metaValue.Interface()
					return nil
				})
				if err != nil {
					return err
				}
				config.Options.Meta = metadata
			}
		case "inputtokenprice":
			if valueNode.Kind == yaml.ScalarNode {
				price := 0.0
				switch v := valueNode.Interface().(type) {
				case int:
					price = float64(v)
				case float64:
					price = v
				default:
					return fmt.Errorf("invalid inputTokenPrice value: %T %v", v, v)
				}
				config.Options.InputTokenPrice = price
			}
		case "outputtokenprice":
			if valueNode.Kind == yaml.ScalarNode {
				price := 0.0
				switch v := valueNode.Interface().(type) {
				case int:
					price = float64(v)
				case float64:
					price = v
				default:
					return fmt.Errorf("invalid outputTokenPrice value: %T %v", v, v)
				}
				config.Options.OutputTokenPrice = price
			}
		case "cachedtokenprice":
			if valueNode.Kind == yaml.ScalarNode {
				price := 0.0
				switch v := valueNode.Interface().(type) {
				case int:
					price = float64(v)
				case float64:
					price = v
				default:
					return fmt.Errorf("invalid cachedTokenPrice value: %T %v", v, v)
				}
				config.Options.CachedTokenPrice = price
			}
		}
		return nil
	})
}
