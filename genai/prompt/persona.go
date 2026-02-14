package prompt

type Persona struct {
	Role    string `yaml:"role,omitempty"  json:"role,omitempty"`
	Actor   string `yaml:"actor,omitempty" json:"actor,omitempty"`
	Summary string `yaml:"summary,omitempty" json:"summary,omitempty"`
}
