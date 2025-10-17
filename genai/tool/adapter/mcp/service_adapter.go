package mcp

import (
	svc "github.com/viant/agently/genai/tool/service"
	mcpschema "github.com/viant/mcp-protocol/schema"
	"reflect"
	"strings"
)

// FromService converts a service.Service to a slice of MCP Tools.
// Tool.Name is the method name; Input/Output schemas are derived from reflection types.
func FromService(s svc.Service) []mcpschema.Tool {
	sigs := s.Methods()
	out := make([]mcpschema.Tool, 0, len(sigs))
	for _, sig := range sigs {
		inT := sig.Input
		if inT == nil {
			inT = reflect.TypeOf(struct{}{})
		}
		outT := sig.Output
		if outT == nil {
			outT = reflect.TypeOf(struct{}{})
		}
		tool := toolFromTypes(sig.Name, sig.Description, inT, outT)
		out = append(out, *tool)
	}
	return out
}

func toolFromTypes(name, description string, inT, outT reflect.Type) *mcpschema.Tool {
	inProps, inReq := objectSchema(inT)
	outProps, _ := objectSchema(outT)
	if inProps == nil {
		inProps = map[string]map[string]interface{}{}
	}
	if outProps == nil {
		outProps = map[string]map[string]interface{}{}
	}
	if description == "" {
		description = name
	}
	return &mcpschema.Tool{
		Name:         name,
		Description:  &description,
		InputSchema:  mcpschema.ToolInputSchema{Type: "object", Properties: mcpschema.ToolInputSchemaProperties(inProps), Required: inReq},
		OutputSchema: &mcpschema.ToolOutputSchema{Type: "object", Properties: outProps},
	}
}

func objectSchema(t reflect.Type) (map[string]map[string]interface{}, []string) {
	t = indirectType(t)
	switch t.Kind() {
	case reflect.Struct:
		props := map[string]map[string]interface{}{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			if isInternal(f) {
				continue
			}
			name, omitempty := jsonName(f)
			if name == "-" || name == "" {
				continue
			}
			props[name] = schemaForType(f.Type, f)
			if !omitempty && f.Type.Kind() != reflect.Ptr {
				required = append(required, name)
			}
		}
		return props, required
	default:
		return map[string]map[string]interface{}{"value": schemaForType(t, reflect.StructField{})}, nil
	}
}

func schemaForType(t reflect.Type, f reflect.StructField) map[string]interface{} {
	t = indirectType(t)
	switch t.Kind() {
	case reflect.Struct:
		props, req := objectSchema(t)
		out := map[string]interface{}{"type": "object", "properties": props}
		if len(req) > 0 {
			out["required"] = req
		}
		applyMeta(out, f)
		return out
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			out := map[string]interface{}{"type": "string"}
			applyMeta(out, f)
			return out
		}
		out := map[string]interface{}{"type": "array", "items": schemaForType(t.Elem(), reflect.StructField{})}
		applyMeta(out, f)
		return out
	case reflect.Map:
		out := map[string]interface{}{"type": "object"}
		applyMeta(out, f)
		return out
	case reflect.String:
		out := map[string]interface{}{"type": "string"}
		applyMeta(out, f)
		return out
	case reflect.Bool:
		out := map[string]interface{}{"type": "boolean"}
		applyMeta(out, f)
		return out
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		out := map[string]interface{}{"type": "integer"}
		applyMeta(out, f)
		return out
	case reflect.Float32, reflect.Float64:
		out := map[string]interface{}{"type": "number"}
		applyMeta(out, f)
		return out
	default:
		out := map[string]interface{}{"type": "object"}
		applyMeta(out, f)
		return out
	}
}

func applyMeta(m map[string]interface{}, f reflect.StructField) {
	if d := f.Tag.Get("description"); d != "" {
		m["description"] = d
	}
	if t := f.Tag.Get("title"); t != "" {
		m["title"] = t
	}
}

func isInternal(f reflect.StructField) bool { return f.Tag.Get("internal") == "true" }

func indirectType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func jsonName(f reflect.StructField) (name string, omitempty bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "-", false
	}
	parts := strings.Split(tag, ",")
	if len(parts) == 0 || parts[0] == "" {
		name = lowerFirst(f.Name)
	} else {
		name = parts[0]
	}
	for _, p := range parts[1:] {
		if strings.TrimSpace(p) == "omitempty" {
			omitempty = true
			break
		}
	}
	return name, omitempty
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = []rune(strings.ToLower(string(r[0])))[0]
	return string(r)
}
