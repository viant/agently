package mcp

import (
	"reflect"
	"strings"

	svc "github.com/viant/agently/genai/tool/service"
	mcpschema "github.com/viant/mcp-protocol/schema"
)

// FromService converts a service.Service to a slice of MCP Tool.
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
			name, omitempty, inline := jsonName(f)
			if name == "-" || name == "" {
				continue
			}

			if inline || f.Anonymous && name == lowerFirst(f.Name) && (f.Type.Kind() == reflect.Struct || (f.Type.Kind() == reflect.Ptr && f.Type.Elem().Kind() == reflect.Struct)) {
				// Determine underlying struct type for the embedded field.
				embeddedType := f.Type
				isPtr := false
				if embeddedType.Kind() == reflect.Ptr {
					isPtr = true
					embeddedType = embeddedType.Elem()
				}
				// Only inline struct types.
				if embeddedType.Kind() == reflect.Struct {
					childProps, childReq := objectSchema(embeddedType)
					// Merge child properties into parent, without overwriting existing keys.
					for k, v := range childProps {
						if _, exists := props[k]; !exists {
							props[k] = v
						}
					}
					// Propagate requireds only if this embedded field itself is required
					// (i.e., not a pointer, not omitempty, and not explicitly optional).
					tag := string(f.Tag)
					isOptional := strings.Contains(tag, "required=false") || strings.Contains(tag, "optional")
					if !isPtr && !omitempty && !isOptional {
						required = append(required, childReq...)
					}
					// Done handling this field.
					continue
				}
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
	// Map repeated `choice:"value"` struct tags to JSON Schema enum
	if enums := enumChoicesFromTag(f.Tag); len(enums) > 0 {
		m["enum"] = enums
	}
}

// enumChoicesFromTag extracts all choice:"..." values from a struct tag.
func enumChoicesFromTag(tag reflect.StructTag) []string {
	raw := string(tag)
	const key = "choice:\""
	out := []string{}
	for {
		i := strings.Index(raw, key)
		if i == -1 {
			break
		}
		rest := raw[i+len(key):]
		j := strings.Index(rest, "\"")
		if j == -1 {
			break
		}
		val := rest[:j]
		if strings.TrimSpace(val) != "" {
			out = append(out, val)
		}
		raw = rest[j+1:]
	}
	return out
}

func isInternal(f reflect.StructField) bool { return f.Tag.Get("internal") == "true" }

func indirectType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func jsonName(f reflect.StructField) (name string, omitempty bool, inline bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "-", false, false
	}
	parts := strings.Split(tag, ",")
	if len(parts) == 0 || parts[0] == "" {
		name = lowerFirst(f.Name)
	} else {
		name = parts[0]
	}
	for _, p := range parts[1:] {
		opt := strings.TrimSpace(p)
		switch opt {
		case "omitempty":
			omitempty = true
		case "inline":
			inline = true
		}
	}
	return name, omitempty, inline
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = []rune(strings.ToLower(string(r[0])))[0]
	return string(r)
}
