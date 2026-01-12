package conversation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeriveExplorerOps(t *testing.T) {
	traceID := "resp_123"
	defaultTraceID := "resp_default"

	testCases := []struct {
		name     string
		toolCall []*ToolCallView
		expected []map[string]interface{}
	}{
		{
			name: "aggregates list+roots+read+grep into ops grouped by trace",
			toolCall: []*ToolCallView{
				{
					ToolName:        "resources-roots",
					TraceId:         &traceID,
					ResponsePayload: &ResponsePayloadView{InlineBody: strPtr(`{"roots":[{"id":"local","path":"/tmp"},{"id":"workspace","uri":"file://localhost/tmp"}]}`)},
				},
				{
					ToolName: "resources-list",
					TraceId:  &traceID,
					// Ensure URI-like names are normalized to leaf file name.
					ResponsePayload: &ResponsePayloadView{InlineBody: strPtr(`{"items":[{"uri":"file://localhost/a.txt","path":"/a.txt","name":"file://localhost/a.txt"}]}`)},
				},
				{
					ToolName:        "resources-read",
					TraceId:         &traceID,
					ResponsePayload: &ResponsePayloadView{InlineBody: strPtr(`{"status":"ok","data":{"Result":"{\"uri\":\"file://localhost/b.txt\",\"path\":\"/b.txt\"}"}}`)},
				},
				{
					ToolName:        "resources-grepFiles",
					TraceId:         &traceID,
					ResponsePayload: &ResponsePayloadView{InlineBody: strPtr(`{"files":[{"URI":"file://localhost/c.txt","Path":"/c.txt","Matches":3}]}`)},
				},
			},
			expected: []map[string]interface{}{
				{
					"trace":     "resp_123",
					"traceId":   "resp_123",
					"operation": "list",
					"count":     3,
					"resources": "a.txt, local, workspace",
				},
				{
					"trace":     "resp_123",
					"traceId":   "resp_123",
					"operation": "read",
					"count":     1,
					"resources": "b.txt",
				},
				{
					"trace":     "resp_123",
					"traceId":   "resp_123",
					"operation": "search",
					"count":     1,
					"resources": "c.txt",
				},
			},
		},
		{
			name: "uses default trace when tool call trace missing",
			toolCall: []*ToolCallView{
				{
					ToolName:        "resources-read",
					ResponsePayload: &ResponsePayloadView{InlineBody: strPtr(`{"uri":"file://localhost/x.txt","path":"/x.txt"}`)},
				},
			},
			expected: []map[string]interface{}{
				{
					"trace":     "resp_default",
					"traceId":   "resp_default",
					"operation": "read",
					"count":     1,
					"resources": "x.txt",
				},
			},
		},
	}

	for _, testCase := range testCases {
		actual := deriveExplorerOps(testCase.toolCall, defaultTraceID, nil)
		assert.EqualValues(t, testCase.expected, simplifyOps(actual), testCase.name)
	}
}

func simplifyOps(ops []map[string]interface{}) []map[string]interface{} {
	if len(ops) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(ops))
	for _, op := range ops {
		out = append(out, map[string]interface{}{
			"trace":     op["trace"],
			"traceId":   op["traceId"],
			"operation": op["operation"],
			"count":     op["count"],
			"resources": op["resources"],
		})
	}
	return out
}

func strPtr(s string) *string { return &s }
