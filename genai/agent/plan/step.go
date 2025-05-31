package plan

// Step represents a single atomic action in a Plan.
type Step struct {
	Type           string                 `yaml:"type" json:"type"`                                           // Action family or special case (e.g., "clarify_intent", "abort")
	Name           string                 `yaml:"name,omitempty" json:"name,omitempty"`                       // Specific tool/function name (if applicable)
	Args           map[string]interface{} `yaml:"args,omitempty" json:"args,omitempty"`                       // Tool arguments (must match the schema in tool_definitions)
	Reason         string                 `yaml:"reason,omitempty" json:"reason,omitempty"`                   // Why this step is necessary
	FollowupPrompt string                 `yaml:"followup_prompt,omitempty" json:"followup_prompt,omitempty"` // If clarification is needed from the user

	Content string `yaml:"content,omitempty" json:"content,omitempty"`

	// Elicitation carries clarification details when this step needs user input.
	MissingFields []MissingField `json:"missingFields,omitempty"`

	// Retries specifies how many times to retry this tool on error or empty result
	Retries int `yaml:"retries,omitempty" json:"retries,omitempty"`
}
