package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	api "github.com/viant/agently/internal/dao/message"
	read "github.com/viant/agently/internal/dao/message/read"
	write "github.com/viant/agently/internal/dao/message/write"
	"time"
)

func TestMemoryMessage_BasicPatchList(t *testing.T) {
	svc := New()

	// insert 2 messages in a conversation
	_, err := svc.Patch(context.Background(), func() *write.Message {
		m := &write.Message{Has: &write.MessageHas{}}
		m.SetId("m1")
		m.SetConversationID("c1")
		m.SetRole("user")
		m.SetType("text")
		m.SetContent("hello")
		return m
	}())
	if !assert.Nil(t, err) {
		return
	}

	_, err = svc.Patch(context.Background(), func() *write.Message {
		m := &write.Message{Has: &write.MessageHas{}}
		m.SetId("m2")
		m.SetConversationID("c1")
		m.SetRole("assistant")
		m.SetType("text")
		m.SetContent("world")
		return m
	}())
	if !assert.Nil(t, err) {
		return
	}

	rows, err := svc.List(context.Background(), read.WithConversationID("c1"))
	if !assert.Nil(t, err) {
		return
	}
	assert.Equal(t, 2, len(rows))

	// transcript (no tool dedup needed since no tool messages)
	rows, err = svc.GetTranscript(context.Background(), "c1", "", read.WithConversationID("c1"))
	if !assert.Nil(t, err) {
		return
	}
	// since we required turnID normally, we passed empty - memory impl filters by turnID, so expect 0
	assert.Equal(t, 0, len(rows))

	// ensure interface compliance
	var _ api.API = svc
}

func TestMemoryMessage_List_DataDriven(t *testing.T) {
	type testCase struct {
		name     string
		seed     func(s *Service)
		opts     []read.InputOption
		expected []string
	}

	// helpers
	ts := func(s string) time.Time { v, _ := time.Parse("2006-01-02 15:04:05", s); return v }

	cases := []testCase{
		{
			name: "by conversation",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), func() *write.Message {
					m := &write.Message{Has: &write.MessageHas{}}
					m.SetId("m1")
					m.SetConversationID("c2")
					m.SetRole("user")
					m.SetType("text")
					m.SetContent("a")
					t0 := ts("2024-01-01 10:00:00")
					m.SetCreatedAt(t0)
					return m
				}())
				_, _ = s.Patch(context.Background(), func() *write.Message {
					m := &write.Message{Has: &write.MessageHas{}}
					m.SetId("m2")
					m.SetConversationID("c2")
					m.SetRole("assistant")
					m.SetType("text")
					m.SetContent("b")
					t1 := ts("2024-01-01 10:01:00")
					m.SetCreatedAt(t1)
					return m
				}())
			},
			opts:     []read.InputOption{read.WithConversationID("c2")},
			expected: []string{"m1", "m2"},
		},
		{
			name: "by role assistant",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), func() *write.Message {
					m := &write.Message{Has: &write.MessageHas{}}
					m.SetId("m3")
					m.SetConversationID("c3")
					m.SetRole("user")
					m.SetType("text")
					m.SetContent("a")
					return m
				}())
				_, _ = s.Patch(context.Background(), func() *write.Message {
					m := &write.Message{Has: &write.MessageHas{}}
					m.SetId("m4")
					m.SetConversationID("c3")
					m.SetRole("assistant")
					m.SetType("text")
					m.SetContent("b")
					return m
				}())
			},
			opts:     []read.InputOption{read.WithConversationID("c3"), read.WithRole("assistant")},
			expected: []string{"m4"},
		},
		{
			name: "by ids subset",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), func() *write.Message {
					m := &write.Message{Has: &write.MessageHas{}}
					m.SetId("m5")
					m.SetConversationID("c4")
					m.SetRole("user")
					m.SetType("text")
					m.SetContent("a")
					return m
				}())
				_, _ = s.Patch(context.Background(), func() *write.Message {
					m := &write.Message{Has: &write.MessageHas{}}
					m.SetId("m6")
					m.SetConversationID("c4")
					m.SetRole("assistant")
					m.SetType("text")
					m.SetContent("b")
					return m
				}())
			},
			opts:     []read.InputOption{read.WithConversationID("c4"), read.WithIDs("m6")},
			expected: []string{"m6"},
		},
		{
			name: "since filter",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), func() *write.Message {
					m := &write.Message{Has: &write.MessageHas{}}
					m.SetId("m7")
					m.SetConversationID("c5")
					m.SetRole("user")
					m.SetType("text")
					m.SetContent("a")
					t0 := ts("2024-01-01 10:00:00")
					m.SetCreatedAt(t0)
					return m
				}())
				_, _ = s.Patch(context.Background(), func() *write.Message {
					m := &write.Message{Has: &write.MessageHas{}}
					m.SetId("m8")
					m.SetConversationID("c5")
					m.SetRole("assistant")
					m.SetType("text")
					m.SetContent("b")
					t1 := ts("2024-01-01 10:02:00")
					m.SetCreatedAt(t1)
					return m
				}())
			},
			opts:     []read.InputOption{read.WithConversationID("c5"), read.WithSince(time.Date(2024, 1, 1, 10, 1, 0, 0, time.UTC))},
			expected: []string{"m8"},
		},
		{
			name: "transcript dedup tool latest attempt",
			seed: func(s *Service) {
				// create messages and then attach ToolCall metadata directly to stored views
				_, _ = s.Patch(context.Background(), func() *write.Message {
					m := &write.Message{Has: &write.MessageHas{}}
					m.SetId("t1")
					m.SetConversationID("c6")
					m.SetTurnID("turnX")
					m.SetRole("tool")
					m.SetType("tool_op")
					m.SetContent("call")
					m.SetSequence(1)
					return m
				}())
				_, _ = s.Patch(context.Background(), func() *write.Message {
					m := &write.Message{Has: &write.MessageHas{}}
					m.SetId("t2")
					m.SetConversationID("c6")
					m.SetTurnID("turnX")
					m.SetRole("tool")
					m.SetType("tool_op")
					m.SetContent("call")
					m.SetSequence(2)
					return m
				}())
				s.messages["t1"].ToolCall = &read.ToolCallView{OpID: "opZ", Attempt: 1}
				s.messages["t2"].ToolCall = &read.ToolCallView{OpID: "opZ", Attempt: 2}
			},
			opts:     nil,
			expected: []string{"t2"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := New()
			tc.seed(svc)
			var ids []string
			if tc.name == "transcript dedup tool latest attempt" {
				rows, err := svc.GetTranscript(context.Background(), "c6", "turnX")
				if !assert.Nil(t, err) {
					return
				}
				for _, r := range rows {
					ids = append(ids, r.Id)
				}
			} else {
				rows, err := svc.List(context.Background(), tc.opts...)
				if !assert.Nil(t, err) {
					return
				}
				for _, r := range rows {
					ids = append(ids, r.Id)
				}
			}
			assert.EqualValues(t, tc.expected, ids)
		})
	}
}
