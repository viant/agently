package model

import (
	"github.com/viant/agently/genai/embedder/provider"
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
		case "model":
			if valueNode.Kind == yaml.ScalarNode {
				config.Options.Model = valueNode.Value
			}
		}
		return nil
	})
}
