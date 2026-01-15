package provider

// Config is a struct that represents a model with an ID and options.
type Config struct {
	ID string `yaml:"id" json:"id"`
	// Name is a human-friendly display name for UI selection (optional).
	Name         string  `yaml:"name" json:"name"`
	Description  string  `yaml:"description" json:"description"`
	Intelligence float64 `yaml:"intelligence" json:"intelligence"`
	Speed        float64 `yaml:"speed" json:"speed"`
	Options      Options `yaml:"options"`
}

type TokenCost struct {
	Input  float64 `yaml:"inputTokenPrice" json:"inputTokenPrice"`
	Output float64 `yaml:"outputTokenPrice" json:"outputTokenPrice"`
	Unit   int
}

// Configs is a slice of Config pointers.
type Configs []*Config

// Find is a method that searches for a model by its ID in the Configs slice.
func (m Configs) Find(id string) *Config {
	for _, model := range m {
		if model.ID == id {
			return model
		}
	}
	return nil
}
