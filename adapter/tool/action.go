package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/tool"
	elog "github.com/viant/agently/internal/log"
	"github.com/viant/fluxor"
	"github.com/viant/mcp-protocol/schema"
	"github.com/viant/structology/conv"
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
			Name:         name,
			Description:  sig.Description,
			Parameters:   map[string]interface{}{},
			OutputSchema: map[string]interface{}{},
		}
		if inputType := ensureStrucType(sig.Input); inputType != nil {
			inputSchema, required := schema.StructToProperties(inputType)
			// OpenAI function parameters must be a JSON schema object with "type: object" wrapper.
			// Populate definition following the expected structure.
			parameters := map[string]interface{}{
				"type":       "object",
				"properties": inputSchema,
			}
			if len(required) > 0 {
				parameters["required"] = required
			}
			def.Parameters = parameters
			def.Required = required
		}

		// Ensure parameters field always has the JSON schema wrapper, even when no input fields are present.
		if len(def.Parameters) == 0 {
			def.Parameters = map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			}
		}
		if outputType := ensureStrucType(sig.Output); outputType != nil {
			outputSchema, _ := schema.StructToProperties(outputType)
			for k, p := range outputSchema {
				def.OutputSchema[k] = p
			}
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
			converter := conv.NewConverter(conv.DefaultOptions())

			input := reflect.New(ensureStrucType(sig.Input)).Interface()
			if err := converter.Convert(in, input); err != nil {
				return "", fmt.Errorf("failed to convert input for %s: %w", name, err)
			}

			// Capture tooling input and output as events
			elog.Publish(elog.Event{Time: time.Now(), EventType: elog.ToolInput, Payload: input})
			output := reflect.New(ensureStrucType(sig.Output)).Interface()
			if err := exec(ctx, input, output); err != nil {
				return "", err
			}
			elog.Publish(elog.Event{Time: time.Now(), EventType: elog.ToolOutput, Payload: output})
			raw, _ := json.Marshal(output) // always return JSON
			return string(raw), nil
		}

		registry.Register(def, handler)
	}
	return nil
}

func ensureStrucType(t reflect.Type) reflect.Type {
	switch t.Kind() {
	case reflect.Pointer:
		return ensureStrucType(t.Elem())
	case reflect.Slice:
		return ensureStrucType(t.Elem())
	case reflect.Struct:
		return t
	}
	return nil
}

// populateDefaultEnv checks if the given pointer-to-struct value has a field
// named "Env" of type map[string]string and if that map is currently nil or
// empty. In that case, it fills the map with the current process environment
// variables so that downstream shell execution inherits common variables such
// as HOME, PATH, USER, etc.
func populateDefaultEnv(v interface{}) {
	if v == nil {
		return
	}
	rv := reflect.ValueOf(v)
	// Dereference pointers
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return
	}
	field := rv.FieldByName("Env")
	if !field.IsValid() {
		return
	}
	// Expect map[string]string
	if field.Kind() != reflect.Map || field.Type().Key().Kind() != reflect.String || field.Type().Elem().Kind() != reflect.String {
		return
	}

	if field.IsNil() || field.Len() == 0 {
		envMap := make(map[string]string)
		for _, kv := range os.Environ() {
			idx := strings.Index(kv, "=")
			if idx <= 0 {
				continue
			}
			k := kv[:idx]
			v := kv[idx+1:]
			envMap[k] = v
		}

		// Create a new map value of correct type
		mapVal := reflect.MakeMapWithSize(field.Type(), len(envMap))
		for k, val := range envMap {
			mapVal.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(val))
		}
		field.Set(mapVal)
	}
}
