package shared

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	read "github.com/viant/agently/internal/dao/message/read"
)

func TestBuildTranscript_OrderFilterDedup(t *testing.T) {
	// Prepare messages with sequence/created_at, control, interim, and tool attempts
	ts := func(s string) *time.Time { v, _ := time.Parse("2006-01-02 15:04:05", s); return &v }
	seq := func(i int) *int { v := i; return &v }
	intval := func(i int) *int { v := i; return &v }

	rows := []*read.MessageView{
		{Id: "1", Role: "user", Type: "text", Sequence: seq(1), CreatedAt: ts("2024-01-01 10:00:00"), Content: "a"},
		{Id: "2", Role: "assistant", Type: "text", Sequence: seq(2), CreatedAt: ts("2024-01-01 10:01:00"), Content: "b"},
		{Id: "3", Role: "tool", Type: "tool_op", Sequence: seq(3), CreatedAt: ts("2024-01-01 10:02:00"), ToolCall: &read.ToolCallView{OpID: "op1", Attempt: 1}},
		{Id: "4", Role: "tool", Type: "tool_op", Sequence: seq(4), CreatedAt: ts("2024-01-01 10:03:00"), ToolCall: &read.ToolCallView{OpID: "op1", Attempt: 2}}, // should keep attempt=2 only
		{Id: "5", Role: "assistant", Type: "text", Sequence: seq(5), CreatedAt: ts("2024-01-01 10:04:00"), Interim: intval(1)},                                  // interim excluded
		{Id: "6", Role: "assistant", Type: "control", Sequence: seq(6), CreatedAt: ts("2024-01-01 10:05:00")},                                                   // control excluded
		{Id: "7", Role: "assistant", Type: "text", CreatedAt: ts("2024-01-01 10:06:00")},                                                                        // no sequence
	}

	got := BuildTranscript(rows, true)

	var ids []string
	for _, r := range got {
		ids = append(ids, r.Id)
	}

	// Expect order: 1,2,4 (dedup keeps latest tool attempt), 7 (no sequence uses created_at)
	assert.EqualValues(t, []string{"1", "2", "4", "7"}, ids)
}
