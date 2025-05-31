package provider

// Config is a struct that represents a model with an ID and options.
type Config struct {
	ID          string  `yaml:"id" json:"id"`
	Description string  `yaml:"description" json:"description"`
	Options     Options `yaml:"options"`
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
