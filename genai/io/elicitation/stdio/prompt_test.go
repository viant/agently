package stdio_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/viant/agently/genai/agent/plan"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/viant/agently/genai/io/elicitation/stdio"
)

type testCase struct {
	name        string
	schemaJSON  string
	userInput   string // newline separated, fed into stdin
	wantPayload map[string]any
	wantAction  plan.ElicitResultAction
	wantErr     bool
}

func TestPrompt(t *testing.T) {
	cases := []testCase{
		{
			name: "single required string",
			schemaJSON: `{
                "type": "object",
                "properties": {
                    "name": {"type": "string"}
                },
                "required": ["name"]
            }`,
			userInput:   "John\n",
			wantPayload: map[string]any{"name": "John"},
			wantAction:  plan.ElicitResultActionAccept,
		},
		{
			name: "default value taken",
			schemaJSON: `{
                "type": "object",
                "properties": {
                    "color": {"type": "string", "default": "blue"}
                }
            }`,
			userInput:   "\n", // hit enter
			wantPayload: map[string]any{"color": jsonRawToAny(`"blue"`)},
			wantAction:  plan.ElicitResultActionAccept,
		},
		{
			name: "enum validation failure → re-prompt → success",
			schemaJSON: `{
                "type": "object",
                "properties": {
                    "size": {"type": "string", "enum": ["S", "M", "L"]}
                },
                "required": ["size"]
            }`,
			userInput:   "XL\nM\n", // first invalid, then valid
			wantPayload: map[string]any{"size": "M"},
			wantAction:  plan.ElicitResultActionAccept,
		},
		{
			name: "optional field omitted",
			schemaJSON: `{
                "type": "object",
                "properties": {
                    "nickname": {"type": "string"}
                }
            }`,
			userInput:   "\n", // skip
			wantPayload: map[string]any{},
			wantAction:  plan.ElicitResultActionAccept,
		},
		{
			name:       "invalid JSON-schema → immediate error",
			schemaJSON: `{`, // malformed JSON
			userInput:  "irrelevant\n",
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := bytes.NewBufferString(tc.userInput)
			var out bytes.Buffer

			p := &plan.Elicitation{Schema: tc.schemaJSON}

			got, err := stdio.Prompt(context.Background(), &out, in, p)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			if !assert.NoError(t, err) {
				return
			}

			assert.EqualValues(t, tc.wantAction, got.Action)
			assert.EqualValues(t, tc.wantPayload, got.Payload)
		})
	}
}

// jsonRawToAny converts a JSON literal to an `any` for easy map construction.
func jsonRawToAny(literal string) any {
	var v any
	_ = json.Unmarshal([]byte(literal), &v)
	return v
}
