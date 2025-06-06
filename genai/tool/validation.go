package tool

import (
	"encoding/json"
	"strings"

	"github.com/viant/agently/genai/llm"
)

// FieldError captures one missing or invalid parameter detected during
// validation against the tool's JSON schema.
type FieldError struct {
	Name   string // parameter name, e.g. "timeoutMs"
	Reason string // free-text explanation
}

// Problem is kept as an alias for backwards compatibility with older callers
// that referred to validation issues as *Problem*s.
type Problem = FieldError

// ValidateArgs checks the provided args against the JSON-schema stored in the
// given tool definition. Currently it focuses on detecting missing *required*
// fields; type checking can be added later.
//
// It returns:
//  1. a shallow copy of args with any default values from the schema filled in;
//  2. a slice describing the remaining problems (empty slice ⇒ valid).
func ValidateArgs(def llm.ToolDefinition, args map[string]interface{}) (map[string]interface{}, []FieldError) {
	// Always operate on a copy so that callers can mutate safely.
	fixed := map[string]interface{}{}
	for k, v := range args {
		fixed[k] = v
	}

	var problems []FieldError

	// Tool schema: parameters.type == object, parameters.properties == map, parameters.required == []string
	paramsRaw, ok := def.Parameters["required"]
	if !ok {
		// No required list → nothing to validate.
		return fixed, nil
	}

	requiredSlice, ok := paramsRaw.([]interface{})
	if !ok {
		return fixed, nil
	}

	// Prepare map for defaults lookup: properties.<field>.default
	var properties map[string]interface{}
	if propRaw, ok := def.Parameters["properties"]; ok {
		if p, err := json.Marshal(propRaw); err == nil {
			_ = json.Unmarshal(p, &properties)
		}
	}

	for _, r := range requiredSlice {
		field, ok := r.(string)
		if !ok || strings.TrimSpace(field) == "" {
			continue
		}
		if _, found := fixed[field]; found {
			continue // present – fine
		}

		// Not present – see if a default exists.
		if defVal := defaultValue(properties, field); defVal != nil {
			fixed[field] = defVal
			continue
		}

		problems = append(problems, FieldError{Name: field, Reason: "required but missing"})
	}

	return fixed, problems
}

func defaultValue(props map[string]interface{}, field string) interface{} {
	if props == nil {
		return nil
	}
	raw, ok := props[field]
	if !ok {
		return nil
	}
	if pm, ok := raw.(map[string]interface{}); ok {
		if defVal, ok2 := pm["default"]; ok2 {
			return defVal
		}
	}
	return nil
}
