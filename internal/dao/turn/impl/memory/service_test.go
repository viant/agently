package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	api "github.com/viant/agently/internal/dao/turn"
	read "github.com/viant/agently/internal/dao/turn/read"
	write "github.com/viant/agently/pkg/agently/turn"
)

func TestMemoryTurn_List_DataDriven(t *testing.T) {
	type testCase struct {
		name     string
		seed     func(s *Service)
		opts     []read.InputOption
		expected []string
	}
	ts := func(s string) time.Time { v, _ := time.Parse("2006-01-02 15:04:05", s); return v }

	cases := []testCase{
		{
			name: "by status",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), &write.Turn{Id: "t1", ConversationID: "c1", Status: "running", Has: &write.TurnHas{Id: true, ConversationID: true, Status: true}})
				_, _ = s.Patch(context.Background(), &write.Turn{Id: "t2", ConversationID: "c1", Status: "failed", Has: &write.TurnHas{Id: true, ConversationID: true, Status: true}})
			},
			opts:     []read.InputOption{read.WithConversationID("c1"), read.WithStatus("failed")},
			expected: []string{"t2"},
		},
		{
			name: "by conversation",
			seed: func(s *Service) {
				t0 := ts("2024-01-01 10:00:00")
				t1 := ts("2024-01-01 12:00:00")
				_, _ = s.Patch(context.Background(), func() *write.Turn {
					x := &write.Turn{Has: &write.TurnHas{}}
					x.SetId("t3")
					x.SetConversationID("c2")
					x.SetStatus("running")
					x.SetCreatedAt(t0)
					return x
				}())
				_, _ = s.Patch(context.Background(), func() *write.Turn {
					x := &write.Turn{Has: &write.TurnHas{}}
					x.SetId("t4")
					x.SetConversationID("c2")
					x.SetStatus("succeeded")
					x.SetCreatedAt(t1)
					return x
				}())
				_, _ = s.Patch(context.Background(), &write.Turn{Id: "t5", ConversationID: "c3", Status: "running", Has: &write.TurnHas{Id: true, ConversationID: true, Status: true}})
			},
			opts:     []read.InputOption{read.WithConversationID("c2")},
			expected: []string{"t3", "t4"},
		},
		{
			name: "since filter",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), func() *write.Turn {
					x := &write.Turn{Has: &write.TurnHas{}}
					x.SetId("t6")
					x.SetConversationID("c4")
					x.SetStatus("running")
					x.SetCreatedAt(ts("2024-01-01 10:00:00"))
					return x
				}())
				_, _ = s.Patch(context.Background(), func() *write.Turn {
					x := &write.Turn{Has: &write.TurnHas{}}
					x.SetId("t7")
					x.SetConversationID("c4")
					x.SetStatus("running")
					x.SetCreatedAt(ts("2024-01-01 12:00:00"))
					return x
				}())
			},
			opts:     []read.InputOption{read.WithConversationID("c4"), read.WithSince(ts("2024-01-01 11:00:00"))},
			expected: []string{"t7"},
		},
		{
			name: "ids IN",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), &write.Turn{Id: "t8", ConversationID: "c5", Status: "running", Has: &write.TurnHas{Id: true, ConversationID: true, Status: true}})
				_, _ = s.Patch(context.Background(), &write.Turn{Id: "t9", ConversationID: "c5", Status: "running", Has: &write.TurnHas{Id: true, ConversationID: true, Status: true}})
				_, _ = s.Patch(context.Background(), &write.Turn{Id: "t10", ConversationID: "c5", Status: "running", Has: &write.TurnHas{Id: true, ConversationID: true, Status: true}})
			},
			opts:     []read.InputOption{read.WithConversationID("c5"), read.WithIDs("t8", "t10")},
			expected: []string{"t8", "t10"},
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
				ids = append(ids, r.Id)
			}
			assert.EqualValues(t, tc.expected, ids)
		})
	}
}

func TestMemoryTurn_APICompliance(t *testing.T) {
	var _ api.API = New()
}
