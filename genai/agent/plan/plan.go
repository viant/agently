package plan

// Plan represents an ordered strategy composed of one or more steps.
type Plan struct {
	ID          string       `yaml:"id,omitempty" json:"id,omitempty"`                   // Unique identifier for the plan
	Intention   string       `yaml:"intention,omitempty" json:"intention,omitempty"`     // Optional summary of the user’s goal
	Steps       []*Step      `yaml:"steps" json:"steps"`                                 // Ordered list of steps to execute
	Elicitation *Elicitation `yaml:"elicitation,omitempty" json:"elicitation,omitempty"` // Optional elicitation details if user input is needed
}

// IsRefined returns true if the plan has been refined beyond a single noop step.
func (p *Plan) IsRefined() bool {
	if p == nil || len(p.Steps) == 0 {
		return false
	}
	if len(p.Steps) > 1 {
		return true
	}
	return p.Steps[0].Type != "noop"
}
