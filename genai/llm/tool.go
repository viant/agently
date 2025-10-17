package llm

import mcpschema "github.com/viant/mcp-protocol/schema"

// Tool represents a tool that can be used by an LLM.
// It follows the OpenAPI specification for defining tools.
type Tool struct {
	Ref     string `json:"ref,omitempty" yaml:"ref"`
	Pattern string `json:"pattern,omitempty" yaml:"pattern"`
	// Type is the type of the tool. Currently, only "function" is supported.
	Type string `json:"type" yaml:"type"`

	// Function is the function definition for this tool.
	// This follows the OpenAPI schema specification.
	Definition ToolDefinition `json:"definition" yaml:"definition"`
}

// ToolDefinition represents a function that can be called by an LLM.
// It follows the OpenAPI specification for defining functions.
type ToolDefinition struct {
	// Name is the name of the function to be called.
	Name string `json:"name" yaml:"name"`

	// Description is a description of what the function does.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Parameters is a JSON Schema object that defines the input parameters the function accepts.
	// This follows the OpenAPI schema specification.
	Parameters map[string]interface{} `json:"parameters,omitempty" yaml:"parameters,omitempty"`

	// Required is a list of required parameters.
	Required []string `json:"required,omitempty" yaml:"required"`

	OutputSchema map[string]interface{} `json:"output_schema,omitempty" yaml:"output_schema,omitempty"` // Output schema for the function
}

// NewFunctionTool creates a new Tool representing a callable function.
func NewFunctionTool(definition ToolDefinition) Tool {
	return Tool{
		Type:       "function",
		Definition: definition,
	}
}

// Normalize ensures provider-agnostic schema validity:
// - parameters is always a JSON object with type=object and properties=object
// - output_schema is always a JSON object with type=object and properties=object
func (d *ToolDefinition) Normalize() {
	// Parameters
	if d.Parameters == nil {
		d.Parameters = map[string]interface{}{}
	}
	if _, ok := d.Parameters["type"]; !ok || d.Parameters["type"] == nil {
		d.Parameters["type"] = "object"
	}
	if props, ok := d.Parameters["properties"]; !ok || props == nil {
		d.Parameters["properties"] = map[string]interface{}{}
	} else {
		if _, ok := props.(map[string]interface{}); !ok {
			// Coerce known variants
			switch m := props.(type) {
			case map[string]map[string]interface{}:
				coerced := make(map[string]interface{}, len(m))
				for k, v := range m {
					coerced[k] = v
				}
				d.Parameters["properties"] = coerced
			case mcpschema.ToolInputSchemaProperties:
				coerced := make(map[string]interface{}, len(m))
				for k, v := range m {
					coerced[k] = v
				}
				d.Parameters["properties"] = coerced
			default:
				d.Parameters["properties"] = map[string]interface{}{}
			}
		}
	}
	// OutputSchema
	if d.OutputSchema == nil {
		d.OutputSchema = map[string]interface{}{}
	}
	if _, ok := d.OutputSchema["type"]; !ok || d.OutputSchema["type"] == nil {
		d.OutputSchema["type"] = "object"
	}
	if oprops, ok := d.OutputSchema["properties"]; !ok || oprops == nil {
		d.OutputSchema["properties"] = map[string]interface{}{}
	} else {
		if _, ok := oprops.(map[string]interface{}); !ok {
			switch m := oprops.(type) {
			case map[string]map[string]interface{}:
				coerced := make(map[string]interface{}, len(m))
				for k, v := range m {
					coerced[k] = v
				}
				d.OutputSchema["properties"] = coerced
			default:
				d.OutputSchema["properties"] = map[string]interface{}{}
			}
		}
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

// ToolDefinitionFromMcpTool convert mcp tool into llm tool
func ToolDefinitionFromMcpTool(tool *mcpschema.Tool) *ToolDefinition {
	description := ""
	if tool.Description != nil {
		description = *tool.Description
	}
	def := ToolDefinition{
		Name:        tool.Name,
		Description: description,
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		OutputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
	def.Parameters["properties"] = tool.InputSchema.Properties
	def.Required = tool.InputSchema.Required
	if tool.OutputSchema != nil {
		def.OutputSchema["properties"] = tool.OutputSchema.Properties
	}
	return &def
}
