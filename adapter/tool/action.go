package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/fluxor"
)

// RegisterActionAsTool converts the Terminal service that Fluxor adds
// automatically into LLM tools.
func RegisterActionAsTool(service *fluxor.Service, registry *tool.Registry, name string, prefix string) error {

	svc := service.Actions().Lookup(name)
	if svc == nil {
		return fmt.Errorf("failed to find service %q", name)
	}

	for _, sig := range svc.Methods() {
		name := prefix + sig.Name // tool name
		_, ok := registry.GetDefinition(name)
		if ok {
			continue
		}

		def := llm.ToolDefinition{
			Name:        name,
			Description: sig.Description,
			Parameters: map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
			},
		}

		// capture method name for the closure
		methodName := sig.Name
		handler := func(ctx context.Context, args map[string]interface{}) (string, error) {
			exec, err := svc.Method(methodName)
			if err != nil {
				return "", err
			}

			// Terminal expects map[string]interface{} as input,
			// and returns map[string]interface{} or string output;
			// we keep things generic.
			in := args
			out := map[string]interface{}{}

			if err := exec(ctx, &in, &out); err != nil {
				return "", err
			}
			raw, _ := json.Marshal(out) // always return JSON
			return string(raw), nil
		}

		registry.Register(def, handler)
	}
	return nil
}
