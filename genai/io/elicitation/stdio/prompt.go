package stdio

// Package stdio provides a simple terminal-based implementation of the
// elicitation wizard that operates on an io.Reader / io.Writer pair.  It is
// primarily intended for unit tests and the reference CLI but can be reused
// by any code that wants a deterministic, non-interactive prompt (e.g. piping
// pre-defined answers in CI).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/viant/agently/genai/agent/plan"

	"github.com/xeipuuv/gojsonschema"
)

// Prompt implements a very small subset of the interactive "schema-based
// elicitation" feature.  For the purpose of Agently’s unit tests we only
// support object-schemas (draft-07) with required/enums/default on top-level
// properties.  The logic is deterministic so that expected prompts can be
// asserted in tests.
//
// w and r are generic so the caller decides whether stdio, pipe or an in-mem
// buffer is used.
func Prompt(ctx context.Context, w io.Writer, r io.Reader, p *plan.Elicitation) (*plan.ElicitResult, error) {
	// ------------------------------------------------------------------
	// 1. Obtain schema (Schema string or fallback to RequestedSchema) and
	//    parse / sanity-check it.
	// ------------------------------------------------------------------
	var schemaSrc []byte

	// Build minimal schema document from RequestedSchema – enough for the
	// interactive prompt implementation that follows.

	// ------------------------------------------------------------------
	// 0.b Display high level message if provided so the user understands the
	//      purpose of the prompt before entering individual fields.
	// ------------------------------------------------------------------
	if strings.TrimSpace(p.Message) != "" {
		fmt.Fprintf(w, "%s\n", p.Message)
	}
	tmp := map[string]interface{}{
		"type":       p.RequestedSchema.Type,
		"properties": p.RequestedSchema.Properties,
	}
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

	// ------------------------------------------------------------------
	// 2. Collect answers property by property
	// ------------------------------------------------------------------
	scanner := bufio.NewScanner(r)
	payload := make(map[string]any)

	orderedProps := s.propertyOrder()

	for _, propName := range orderedProps {
		prop := s.Properties[propName]
		required := contains(s.Required, propName)

		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			// Content label ------------------------------------------------
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

			// Read single line answer -----------------------------------
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					return nil, err
				}
				return nil, io.ErrUnexpectedEOF
			}
			answer := strings.TrimSpace(scanner.Text())

			// Empty answer ----------------------------------------------
			if answer == "" {
				if required {
					fmt.Fprintf(w, "%s is required – please provide a value.\n", propName)
					continue // re-prompt
				}

				if prop.Default != nil {
					var v any
					_ = json.Unmarshal(prop.Default, &v)
					payload[propName] = v
				}
				break // next property
			}

			// Enum validation -------------------------------------------
			if len(prop.Enum) > 0 && !contains(prop.Enum, answer) {
				fmt.Fprintf(w, "invalid value – must be one of [%s]\n", strings.Join(prop.Enum, ", "))
				continue // re-prompt
			}

			// Type-aware casting: if schema says string, keep raw input string.
			// Otherwise, try JSON parse (numbers, bools, objects, arrays),
			// and fall back to the raw string if parsing fails.
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
			break // accepted value
		}
	}

	// ------------------------------------------------------------------
	// 3. Final validation against the full schema -----------------------
	// ------------------------------------------------------------------
	// Use the resolved schemaSrc that has been parsed above instead of relying
	// on p.Schema which might be empty when the caller provided the schema only
	// via the RequestedSchema fields. This ensures that validation works in
	// both cases (explicit Schema string, or inline RequestedSchema).
	//
	// gojsonschema provides a bytes-based loader so we avoid an unnecessary
	// string conversion while guaranteeing that the loader always receives the
	// correct schema document we already parsed successfully.
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

// -----------------------------------------------------------------------------
// Helpers (mostly copied unchanged from original implementation)
// -----------------------------------------------------------------------------

// rawSchema is a trimmed-down representation sufficient for the unit tests.
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

// propertyOrder returns property names in declaration order (best effort).
func (s *rawSchema) propertyOrder() []string {
	if len(s.Properties) == 0 {
		return nil
	}
	var order []string
	dec := json.NewDecoder(bytes.NewReader(s.raw))

	var stack []json.Token
	var inProps bool
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break // fallback later
		}
		switch t := tok.(type) {
		case string:
			if len(stack) == 0 {
				if t == "properties" {
					inProps = true
				}
				continue
			}
			if inProps && len(stack) == 1 {
				order = append(order, t)
			}
		case json.Delim:
			d := t.String()
			if d == "{" || d == "[" {
				stack = append(stack, tok)
			} else {
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				if len(stack) == 0 {
					inProps = false
				}
			}
		}
	}

	if len(order) == 0 {
		for k := range s.Properties {
			order = append(order, k)
		}
	}
	return order
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
