package conversation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	gentool "github.com/viant/agently/genai/tool"
)

func TestTranscriptView_computeToolFeed_ExplorerKeepsAllHistoryScopeLast(t *testing.T) {
	traceID := "resp_123"
	now := time.Now()

	feedSpec := &gentool.FeedSpec{
		ID: "explorer",
		Match: gentool.MatchSpec{
			Service: "resources",
			Method:  "*",
		},
		Activation: gentool.ActivationSpec{
			Kind:  "history",
			Scope: "last", // legacy config we still need to aggregate for Explorer
		},
	}
	ctx := context.WithValue(context.Background(), inputKey, &ConversationInput{FeedSpec: []*gentool.FeedSpec{feedSpec}})

	tv := &TranscriptView{
		Id:             "turn1",
		ConversationId: "conv1",
		CreatedAt:      now,
		Message: []*MessageView{
			{
				Id:             "m1",
				ConversationId: "conv1",
				TurnId:         strPtr("turn1"),
				CreatedAt:      now,
				Role:           "tool",
				Type:           "tool_op",
				ToolCall: &ToolCallView{
					ToolName:        "resources-roots",
					TraceId:         &traceID,
					ResponsePayload: &ResponsePayloadView{InlineBody: strPtr(`{"roots":[{"id":"local","path":"/tmp"},{"id":"workspace","uri":"file://localhost/tmp"}]}`)},
				},
			},
			{
				Id:             "m2",
				ConversationId: "conv1",
				TurnId:         strPtr("turn1"),
				CreatedAt:      now,
				Role:           "tool",
				Type:           "tool_op",
				ToolCall: &ToolCallView{
					ToolName:        "resources-read",
					TraceId:         &traceID,
					ResponsePayload: &ResponsePayloadView{InlineBody: strPtr(`{"status":"ok","data":{"Result":"{\"uri\":\"file://localhost/b.txt\",\"path\":\"/b.txt\"}"}}`)},
				},
			},
			{
				Id:             "m3",
				ConversationId: "conv1",
				TurnId:         strPtr("turn1"),
				CreatedAt:      now,
				Role:           "tool",
				Type:           "tool_op",
				ToolCall: &ToolCallView{
					ToolName:        "resources-grepFiles",
					TraceId:         &traceID,
					ResponsePayload: &ResponsePayloadView{InlineBody: strPtr(`{"files":[{"URI":"file://localhost/c.txt","Path":"/c.txt","Matches":3}]}`)},
				},
			},
		},
	}

	feeds, err := tv.computeToolFeed(ctx)
	assert.NoError(t, err)
	if assert.NotEmpty(t, feeds) {
		explorer := feeds[0]
		root, ok := explorer.Data.Data.(map[string]interface{})
		if assert.True(t, ok) {
			ops, _ := root["ops"].([]map[string]interface{})
			if len(ops) == 0 {
				// JSON unmarshal roundtrip may coerce to []interface{}; handle both.
				rawOps, _ := root["ops"].([]interface{})
				ops = make([]map[string]interface{}, 0, len(rawOps))
				for _, it := range rawOps {
					if m, ok := it.(map[string]interface{}); ok {
						ops = append(ops, m)
					}
				}
			}
			// With three matching tool calls, Explorer must show three operations
			// (list/read/search) even if the feed spec uses scope=last.
			assert.EqualValues(t, 3, len(ops))
		}
	}
}
