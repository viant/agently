package provider

import "context"

// Config represents an embedding model configuration
type Config struct {
	ID      string  `yaml:"id" json:"id"`
	Options Options `yaml:"options" json:"options"`
}

// Configs represents a collection of embedding models
type Configs []*Config

// Find finds a model by its ID
func (m Configs) Find(id string) *Config {
	for _, model := range m {
		if model.ID == id {
			return model
		}
	}
	return nil
}

// ConfigLoader defines the interface for loading model configurations.// It provides methods to list all configurations and load a specific configuration by name.
type ConfigLoader interface {
	Load(ctx context.Context, name string) (*Config, error)
}
