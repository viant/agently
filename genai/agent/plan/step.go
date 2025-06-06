package plan

// Step represents a single atomic action in a Plan.
type Step struct {
	Type    string                 `yaml:"type" json:"type"`                         // "tool", "elicitation", "abort" etc.
	Name    string                 `yaml:"name,omitempty" json:"name,omitempty"`     // Tool/function name (if applicable)
	Args    map[string]interface{} `yaml:"args,omitempty" json:"args,omitempty"`     // Tool arguments matching tool schema
	Reason  string                 `yaml:"reason,omitempty" json:"reason,omitempty"` // Explanation of the step
	Content string                 `yaml:"content,omitempty" json:"content,omitempty"`

	// Structured elicitation payload (when Type=="elicitation").
	Elicitation *Elicitation `json:"elicitation,omitempty"`

	// Retries specifies how many times to retry this tool on error or empty result
	Retries int `yaml:"retries,omitempty" json:"retries,omitempty"`
}

type Steps []Step

func (s Steps) Len() int {
	return len(s)
}

func (s Steps) ToolStepCount() int {
	count := 0
	for _, step := range s {
		if step.Type == "tool" {
			count++
		}
	}
	return count
}
