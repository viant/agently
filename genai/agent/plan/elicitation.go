package plan

// Elicitation captures information that the planner still needs from the user
// before it can continue executing the plan.
type Elicitation struct {
	// Prompt is a human-friendly question to ask the user.
	Prompt string `json:"prompt"`

	// MissingFields lists the exact parameters (and their context) that are
	// required.  UI can use this to build structured forms.
	MissingFields []MissingField `json:"missingFields,omitempty"`
}

func (e *Elicitation) IsEmpty() bool {
	return e.Prompt == "" && len(e.MissingFields) == 0
}

// MissingField describes one piece of information that needs user input.
type MissingField struct {
	// Name of the parameter, e.g. "city".
	Name string `json:"name"`

	// Tool is an optional name of the tool/function that requires the value.
	Tool string `json:"tool,omitempty"`

	// ArgPath is the dotted path within the tool args where this value belongs.
	ArgPath string `json:"argPath,omitempty"`

	// Options gives a set of suggested values (when the LLM can enumerate).
	Options []string `json:"options,omitempty"`
}
