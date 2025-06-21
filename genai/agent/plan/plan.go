package plan

import (
	"encoding/json"
	"github.com/google/uuid"
)

// Plan represents an ordered strategy composed of one or more steps.
type Plan struct {
	ID          string       `yaml:"id,omitempty" json:"id,omitempty"`                   // Unique identifier for the plan
	Intention   string       `yaml:"intention,omitempty" json:"intention,omitempty"`     // Optional summary of the user’s goal
	Steps       Steps        `yaml:"steps" json:"steps"`                                 // Ordered list of steps to execute
	Elicitation *Elicitation `yaml:"elicitation,omitempty" json:"elicitation,omitempty"` // Optional elicitation details if user input is needed
}

type Outcome struct {
	ID    string         `yaml:"id,omitempty" json:"id,omitempty"`
	Steps []*StepOutcome `yaml:"steps" json:"steps"`
}

type StepOutcome struct {
	ID       string                 `yaml:"id,omitempty" json:"id,omitempty"`
	Tool     string                 `yaml:"tool,omitempty" json:"tool,omitempty"`
	Reason   string                 `yaml:"reason,omitempty" json:"reason,omitempty"`
	Request  json.RawMessage        `yaml:"request,omitempty" json:"request,omitempty"`
	Response json.RawMessage        `yaml:"response,omitempty" json:"response,omitempty"`
	Elicited map[string]interface{} `yaml:"elicitation,omitempty" json:"elicitation,omitempty"`
	// Success mirrors tool call outcome
	Success   bool   `yaml:"success,omitempty" json:"success,omitempty"`
	Error     string `yaml:"error,omitempty" json:"error,omitempty"`
	Elapsed   string `yaml:"elapsed,omitempty" json:"elapsed,omitempty"`
	StartedAt string `yaml:"startedAt,omitempty" json:"startedAt,omitempty"`
	EndedAt   string `yaml:"endedAt,omitempty" json:"endedAt,omitempty"`
}

func New() *Plan {
	return &Plan{ID: uuid.New().String()}
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
