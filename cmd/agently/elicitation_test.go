package agently

import (
	"bytes"
	"encoding/json"
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

func TestAwaitFormElicitationToolApprovalCheckboxes(t *testing.T) {
	meta, _ := json.Marshal(map[string]any{
		"type":        "tool_approval",
		"toolName":    "system/os/getEnv",
		"title":       "OS Env Access",
		"message":     "The agent wants access to your HOME, SHELL, and PATH environment variables.",
		"acceptLabel": "Allow",
		"rejectLabel": "Deny",
		"cancelLabel": "Cancel",
		"editors": []map[string]any{
			{
				"name":        "names",
				"kind":        "checkbox_list",
				"label":       "Environment variables",
				"description": "Choose which environment variables this tool may access.",
				"options": []map[string]any{
					{"id": "HOME", "label": "HOME", "selected": true},
					{"id": "SHELL", "label": "SHELL", "selected": true},
					{"id": "PATH", "label": "PATH", "selected": true},
				},
			},
		},
	})
	req := &coreplan.Elicitation{
		ElicitRequestParams: mcpproto.ElicitRequestParams{
			Message: "The agent wants access to your HOME, SHELL, and PATH environment variables.",
			RequestedSchema: mcpproto.ElicitRequestParamsRequestedSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"_approvalMeta": map[string]interface{}{"type": "string", "const": string(meta)},
				},
			},
		},
	}
	var out bytes.Buffer
	in := strings.NewReader("1,3\na\n")

	result, err := awaitFormElicitation(&out, in, req)
	if err != nil {
		t.Fatalf("awaitFormElicitation() error = %v", err)
	}
	if result.Action != coreplan.ElicitResultActionAccept {
		t.Fatalf("action = %q, want accept", result.Action)
	}
	edited, ok := result.Payload["editedFields"].(map[string]any)
	if !ok {
		t.Fatalf("editedFields missing: %#v", result.Payload)
	}
	got, ok := edited["names"].([]string)
	if ok {
		if strings.Join(got, ",") != "HOME,PATH" {
			t.Fatalf("names = %#v, want HOME,PATH", got)
		}
		return
	}
	gotIface, ok := edited["names"].([]any)
	if !ok {
		t.Fatalf("names missing: %#v", edited["names"])
	}
	if len(gotIface) != 2 || gotIface[0] != "HOME" || gotIface[1] != "PATH" {
		t.Fatalf("names = %#v, want [HOME PATH]", gotIface)
	}
}
