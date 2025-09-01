package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	api "github.com/viant/agently/internal/dao/toolcall"
	read "github.com/viant/agently/internal/dao/toolcall/read"
	write "github.com/viant/agently/internal/dao/toolcall/write"
)

func TestMemoryToolCall_BasicPatchList(t *testing.T) {
	svc := New()

	// insert tool call
	out, err := svc.Patch(context.Background(), func() *write.ToolCall {
		tc := &write.ToolCall{Has: &write.ToolCallHas{}}
		tc.SetMessageID("m1")
		tc.SetOpID("op-1")
		tc.SetToolName("search")
		tc.SetToolKind("general")
		tc.SetStatus("completed")
		return tc
	}())
	if !assert.Nil(t, err) {
		return
	}
	assert.NotNil(t, out)

	rows, err := svc.List(context.Background(), read.WithMessageID("m1"))
	if !assert.Nil(t, err) {
		return
	}
	if !assert.Equal(t, 1, len(rows)) {
		return
	}
	assert.Equal(t, "search", rows[0].ToolName)

	// interface compliance
	var _ api.API = svc
}

func TestMemoryToolCall_List_DataDriven(t *testing.T) {
	type testCase struct {
		name     string
		seed     func(s *Service)
		opts     []read.InputOption
		expected []string
	}
	cases := []testCase{
		{
			name: "by tool name",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), &write.ToolCall{MessageID: "m2", OpID: "op-1", ToolName: "search", ToolKind: "general", Status: "completed", Has: &write.ToolCallHas{MessageID: true, OpID: true, ToolName: true, ToolKind: true, Status: true}})
				_, _ = s.Patch(context.Background(), &write.ToolCall{MessageID: "m3", OpID: "op-2", ToolName: "browse", ToolKind: "general", Status: "completed", Has: &write.ToolCallHas{MessageID: true, OpID: true, ToolName: true, ToolKind: true, Status: true}})
			},
			opts:     []read.InputOption{read.WithToolName("search")},
			expected: []string{"m2"},
		},
		{
			name: "ids subset",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), &write.ToolCall{MessageID: "m4", OpID: "op-1", ToolName: "search", ToolKind: "general", Status: "completed", Has: &write.ToolCallHas{MessageID: true, OpID: true, ToolName: true, ToolKind: true, Status: true}})
				_, _ = s.Patch(context.Background(), &write.ToolCall{MessageID: "m5", OpID: "op-2", ToolName: "browse", ToolKind: "general", Status: "completed", Has: &write.ToolCallHas{MessageID: true, OpID: true, ToolName: true, ToolKind: true, Status: true}})
			},
			opts:     []read.InputOption{read.WithMessageIDs("m5")},
			expected: []string{"m5"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := New()
			tc.seed(svc)
			rows, err := svc.List(context.Background(), tc.opts...)
			if !assert.Nil(t, err) {
				return
			}
			var ids []string
			for _, r := range rows {
				ids = append(ids, r.MessageID)
			}
			assert.EqualValues(t, tc.expected, ids)
		})
	}
}
