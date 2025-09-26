package stdio

// This file brings over the original implementation from genai/io/elicitation/stdio.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	plan "github.com/viant/agently/genai/agent/plan"
	"github.com/xeipuuv/gojsonschema"
	"io"
	"sort"
	"strings"
)

func Prompt(ctx context.Context, w io.Writer, r io.Reader, p *plan.Elicitation) (*plan.ElicitResult, error) {
	var schemaSrc []byte
	if strings.TrimSpace(p.Message) != "" {
		fmt.Fprintf(w, "%s\n", p.Message)
	}
	tmp := map[string]interface{}{"type": p.RequestedSchema.Type, "properties": p.RequestedSchema.Properties}
	if len(p.RequestedSchema.Required) > 0 {
		tmp["required"] = p.RequestedSchema.Required
	}
	if b, _ := json.Marshal(tmp); len(b) > 0 {
		schemaSrc = b
	}
	var s rawSchema
	if err := json.Unmarshal(schemaSrc, &s); err != nil {
		return nil, fmt.Errorf("invalid JSON schema: %w", err)
	}
	if strings.ToLower(s.Type) != "object" {
		return nil, fmt.Errorf("only object schemas are supported, got %q", s.Type)
	}
	scanner := bufio.NewScanner(r)
	payload := make(map[string]any)
	// Order properties by explicit x-ui-order when present
	orderedProps := uiOrderGeneric(p.RequestedSchema.Properties)
	if len(orderedProps) == 0 {
		orderedProps = s.propertyOrder()
	}
	for _, propName := range orderedProps {
		prop := s.Properties[propName]
		required := contains(s.Required, propName)
		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			fmt.Fprintf(w, "%s", propName)
			if prop.Description != "" {
				fmt.Fprintf(w, " – %s", prop.Description)
			}
			if len(prop.Enum) > 0 {
				fmt.Fprintf(w, " (enum: %s)", strings.Join(prop.Enum, ", "))
			}
			if prop.Default != nil {
				fmt.Fprintf(w, " [default: %s]", string(prop.Default))
			}
			fmt.Fprint(w, ": ")
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					return nil, err
				}
				return nil, io.ErrUnexpectedEOF
			}
			answer := strings.TrimSpace(scanner.Text())
			if answer == "" {
				if required {
					fmt.Fprintf(w, "%s is required – please provide a value.\n", propName)
					continue
				}
				if prop.Default != nil {
					var v any
					_ = json.Unmarshal(prop.Default, &v)
					payload[propName] = v
				}
				break
			}
			if len(prop.Enum) > 0 && !contains(prop.Enum, answer) {
				fmt.Fprintf(w, "invalid value – must be one of [%s]\n", strings.Join(prop.Enum, ", "))
				continue
			}
			var v any
			switch strings.ToLower(strings.TrimSpace(prop.Type)) {
			case "string", "":
				v = answer
			default:
				if err := json.Unmarshal([]byte(answer), &v); err != nil {
					v = answer
				}
			}
			payload[propName] = v
			break
		}
	}
	schemaLoader := gojsonschema.NewBytesLoader(schemaSrc)
	docBytes, _ := json.Marshal(payload)
	docLoader := gojsonschema.NewBytesLoader(docBytes)
	result, err := gojsonschema.Validate(schemaLoader, docLoader)
	if err != nil {
		return nil, err
	}
	if !result.Valid() {
		var b bytes.Buffer
		for i, e := range result.Errors() {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(e.String())
		}
		return nil, fmt.Errorf("collected payload does not satisfy schema: %s, payload: %v", b.String(), payload)
	}
	return &plan.ElicitResult{Action: plan.ElicitResultActionAccept, Payload: payload}, nil
}

type rawSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]rawProperty `json:"properties"`
	Required   []string               `json:"required"`
	raw        json.RawMessage
}
type rawProperty struct {
	Type        string          `json:"type"`
	Description string          `json:"description,omitempty"`
	Enum        []string        `json:"enum,omitempty"`
	Default     json.RawMessage `json:"default,omitempty"`
}

func (s *rawSchema) UnmarshalJSON(b []byte) error {
	type alias rawSchema
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*s = rawSchema(a)
	s.raw = append([]byte(nil), b...)
	return nil
}
func (s *rawSchema) propertyOrder() []string {
	if len(s.Properties) == 0 {
		return nil
	}
	var order []string
	dec := json.NewDecoder(bytes.NewReader(s.raw))
	for {
		var tok json.Token
		var err error
		if tok, err = dec.Token(); err != nil {
			break
		}
		if key, ok := tok.(string); ok && key == "properties" {
			var obj map[string]json.RawMessage
			_ = dec.Decode(&obj)
			for k := range obj {
				order = append(order, k)
			}
			break
		}
	}
	sort.Strings(order)
	return order
}
func contains(arr []string, s string) bool {
	for _, v := range arr {
		if v == s {
			return true
		}
	}
	return false
}
func uiOrder(props map[string]map[string]interface{}) []string {
	if len(props) == 0 {
		return nil
	}
	type kv struct {
		k string
		i int
	}
	var items []kv
	for k, v := range props {
		if o, ok := v["x-ui-order"]; ok {
			if idx, ok := o.(float64); ok {
				items = append(items, kv{k, int(idx)})
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].i < items[j].i })
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.k)
	}
	if len(out) == 0 {
		for k := range props {
			out = append(out, k)
		}
		sort.Strings(out)
	}
	return out
}

// uiOrderGeneric supports schemas where Properties is map[string]interface{}.
func uiOrderGeneric(props map[string]interface{}) []string {
	if len(props) == 0 {
		return nil
	}
	type kv struct {
		k string
		i int
	}
	var items []kv
	for k, v := range props {
		if m, ok := v.(map[string]interface{}); ok {
			if o, ok := m["x-ui-order"]; ok {
				switch t := o.(type) {
				case float64:
					items = append(items, kv{k, int(t)})
				case int:
					items = append(items, kv{k, t})
				}
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].i < items[j].i })
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.k)
	}
	if len(out) == 0 {
		for k := range props {
			out = append(out, k)
		}
		sort.Strings(out)
	}
	return out
}
