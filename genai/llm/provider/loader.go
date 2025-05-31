package provider

import "context"

// ConfigLoader defines the interface for loading model configurations.// It provides methods to list all configurations and load a specific configuration by name.
type ConfigLoader interface {
	Load(ctx context.Context, name string) (*Config, error)
}
