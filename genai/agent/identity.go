package agent

type Identity struct {
	ID   string `yaml:"id,omitempty" json:"id,omitempty"`
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	Icon string `yaml:"icon,omitempty" json:"icon,omitempty"`
}
