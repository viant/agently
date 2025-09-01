package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	api "github.com/viant/agently/internal/dao/modelcall"
	read "github.com/viant/agently/internal/dao/modelcall/read"
	write "github.com/viant/agently/internal/dao/modelcall/write"
	"time"
)

func TestMemoryModelCall_BasicPatchList(t *testing.T) {
	svc := New()

	// insert model call
	out, err := svc.Patch(context.Background(), func() *write.ModelCall {
		mc := &write.ModelCall{Has: &write.ModelCallHas{}}
		mc.SetMessageID("m1")
		mc.SetProvider("openai")
		mc.SetModel("gpt")
		mc.SetModelKind("chat")
		return mc
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
	assert.Equal(t, "openai", rows[0].Provider)

	// interface compliance
	var _ api.API = svc
}

func TestMemoryModelCall_List_DataDriven(t *testing.T) {
	type testCase struct {
		name     string
		seed     func(s *Service)
		opts     []read.InputOption
		expected []string
	}
	ts := func(s string) *time.Time { v, _ := time.Parse("2006-01-02 15:04:05", s); return &v }

	cases := []testCase{
		{
			name: "by provider",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), &write.ModelCall{MessageID: "m10", Provider: "openai", Model: "gpt", ModelKind: "chat", Has: &write.ModelCallHas{MessageID: true, Provider: true, Model: true, ModelKind: true}})
				_, _ = s.Patch(context.Background(), &write.ModelCall{MessageID: "m11", Provider: "google", Model: "gemini", ModelKind: "chat", Has: &write.ModelCallHas{MessageID: true, Provider: true, Model: true, ModelKind: true}})
			},
			opts:     []read.InputOption{read.WithProvider("openai")},
			expected: []string{"m10"},
		},
		{
			name: "by message ids subset",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), &write.ModelCall{MessageID: "m12", Provider: "openai", Model: "gpt", ModelKind: "chat", Has: &write.ModelCallHas{MessageID: true, Provider: true, Model: true, ModelKind: true}})
				_, _ = s.Patch(context.Background(), &write.ModelCall{MessageID: "m13", Provider: "openai", Model: "gpt", ModelKind: "chat", Has: &write.ModelCallHas{MessageID: true, Provider: true, Model: true, ModelKind: true}})
			},
			opts:     []read.InputOption{read.WithMessageIDs("m12")},
			expected: []string{"m12"},
		},
		{
			name: "since filter",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), &write.ModelCall{MessageID: "m14", Provider: "x", Model: "a", ModelKind: "chat", StartedAt: ts("2024-01-01 10:00:00"), Has: &write.ModelCallHas{MessageID: true, Provider: true, Model: true, ModelKind: true, StartedAt: true}})
				_, _ = s.Patch(context.Background(), &write.ModelCall{MessageID: "m15", Provider: "x", Model: "b", ModelKind: "chat", StartedAt: ts("2024-01-01 11:00:00"), Has: &write.ModelCallHas{MessageID: true, Provider: true, Model: true, ModelKind: true, StartedAt: true}})
			},
			opts:     []read.InputOption{read.WithSince(time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC))},
			expected: []string{"m15"},
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
