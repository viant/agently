package agently

import (
	"bytes"
	"strings"
	"testing"

	coreplan "github.com/viant/agently-core/protocol/agent/plan"
	mcpproto "github.com/viant/mcp-protocol/schema"
)

func testElicitation() *coreplan.Elicitation {
	return &coreplan.Elicitation{
		ElicitRequestParams: mcpproto.ElicitRequestParams{
			Message: "Please provide your favorite color.",
			RequestedSchema: mcpproto.ElicitRequestParamsRequestedSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"color": map[string]interface{}{"type": "string"},
					"shade": map[string]interface{}{"type": "string"},
				},
			},
		},
	}
}

func TestAwaitFormElicitationAccept(t *testing.T) {
	req := testElicitation()
	var out bytes.Buffer
	in := strings.NewReader("blue\nnavy\na\n")

	result, err := awaitFormElicitation(&out, in, req)
	if err != nil {
		t.Fatalf("awaitFormElicitation() error = %v", err)
	}
	if result.Action != coreplan.ElicitResultActionAccept {
		t.Fatalf("action = %q, want accept", result.Action)
	}
	if got := result.Payload["color"]; got != "blue" {
		t.Fatalf("color = %#v, want blue", got)
	}
	if got := result.Payload["shade"]; got != "navy" {
		t.Fatalf("shade = %#v, want navy", got)
	}
}

func TestAwaitFormElicitationSkipAndCancel(t *testing.T) {
	req := testElicitation()
	var out bytes.Buffer
	in := strings.NewReader("next\ncancel\n")

	result, err := awaitFormElicitation(&out, in, req)
	if err != nil {
		t.Fatalf("awaitFormElicitation() error = %v", err)
	}
	if result.Action != coreplan.ElicitResultActionDecline {
		t.Fatalf("action = %q, want decline", result.Action)
	}
	if result.Reason != "cancelled" {
		t.Fatalf("reason = %q, want cancelled", result.Reason)
	}
	if _, ok := result.Payload["color"]; ok {
		t.Fatalf("unexpected skipped color in payload: %#v", result.Payload["color"])
	}
}

func TestAwaitFormElicitationFinalCancel(t *testing.T) {
	req := testElicitation()
	var out bytes.Buffer
	in := strings.NewReader("blue\nnavy\nc\n")

	result, err := awaitFormElicitation(&out, in, req)
	if err != nil {
		t.Fatalf("awaitFormElicitation() error = %v", err)
	}
	if result.Action != coreplan.ElicitResultActionDecline {
		t.Fatalf("action = %q, want decline", result.Action)
	}
	if result.Reason != "cancelled" {
		t.Fatalf("reason = %q, want cancelled", result.Reason)
	}
}
