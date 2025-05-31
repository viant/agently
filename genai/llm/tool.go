package llm

// Tool represents a tool that can be used by an LLM.
// It follows the OpenAPI specification for defining tools.
type Tool struct {
	Ref string `json:"ref,omitempty"` // Reference to the tool definition
	// Type is the type of the tool. Currently, only "function" is supported.
	Type string `json:"type"`

	// Function is the function definition for this tool.
	// This follows the OpenAPI schema specification.
	Definition ToolDefinition `json:"definition"`
}

// ToolDefinition represents a function that can be called by an LLM.
// It follows the OpenAPI specification for defining functions.
type ToolDefinition struct {
	// Name is the name of the function to be called.
	Name string `json:"name"`

	// Description is a description of what the function does.
	Description string `json:"description,omitempty"`

	// Parameters is a JSON Schema object that defines the input parameters the function accepts.
	// This follows the OpenAPI schema specification.
	Parameters map[string]interface{} `json:"parameters,omitempty"`

	// Required is a list of required parameters.
	Required []string `json:"required,omitempty"`
}

// NewFunctionTool creates a new Tool representing a callable function.
func NewFunctionTool(definition ToolDefinition) Tool {
	return Tool{
		Type:       "function",
		Definition: definition,
	}
}

// ToolChoice represents a choice of tool to use.
// It can be "none", "auto", or a specific tool.
type ToolChoice struct {
	// Type is the type of the tool choice. It can be "none", "auto", or "function".
	Type string `json:"type"`

	// Function is the function to call if Type is "function".
	Function *ToolChoiceFunction `json:"function,omitempty"`
}

// ToolChoiceFunction represents a function to call in a tool choice.
type ToolChoiceFunction struct {
	// Name is the name of the function to call.
	Name string `json:"name"`
}

// NewAutoToolChoice creates a new ToolChoice with "auto" type.
func NewAutoToolChoice() ToolChoice {
	return ToolChoice{
		Type: "auto",
	}
}

// NewNoneToolChoice creates a new ToolChoice with "none" type.
func NewNoneToolChoice() ToolChoice {
	return ToolChoice{
		Type: "none",
	}
}

// NewFunctionToolChoice creates a new ToolChoice with "function" type and the given function name.
func NewFunctionToolChoice(name string) ToolChoice {
	return ToolChoice{
		Type: "function",
		Function: &ToolChoiceFunction{
			Name: name,
		},
	}
}
