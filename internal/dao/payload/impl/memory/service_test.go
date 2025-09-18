package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	api "github.com/viant/agently/internal/dao/payload"
	read "github.com/viant/agently/internal/dao/payload/read"
	write "github.com/viant/agently/pkg/agently/payload"
)

func TestMemoryPayload_List_DataDriven(t *testing.T) {
	type testCase struct {
		name     string
		seed     func(s *Service)
		opts     []read.InputOption
		expected []string
	}
	ts := func(s string) time.Time { v, _ := time.Parse("2006-01-02 15:04:05", s); return v }
	cases := []testCase{
		{
			name: "by tenant",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), func() *write.Payload {
					p := &write.Payload{Has: &write.PayloadHas{}}
					p.SetId("p1")
					p.Kind = "model_request"
					p.Has.Kind = true
					p.SetMimeType("application/json")
					p.SetSizeBytes(10)
					p.SetStorage("inline")
					p.SetCompression("none")
					t0 := ts("2024-01-01 10:00:00")
					p.CreatedAt = &t0
					p.Has.CreatedAt = true
					tenant := "t1"
					p.TenantID = &tenant
					p.Has.TenantID = true
					return p
				}())
			},
			opts:     []read.InputOption{read.WithTenantID("t1")},
			expected: []string{"p1"},
		},
		{
			name: "by kind",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), func() *write.Payload {
					p := &write.Payload{Has: &write.PayloadHas{}}
					p.SetId("p2")
					p.Kind = "tool_request"
					p.Has.Kind = true
					p.SetMimeType("application/json")
					p.SetSizeBytes(9)
					p.SetStorage("inline")
					p.SetCompression("none")
					return p
				}())
			},
			opts:     []read.InputOption{read.WithKind("tool_request")},
			expected: []string{"p2"},
		},
		{
			name: "ids subset",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), func() *write.Payload {
					p := &write.Payload{Has: &write.PayloadHas{}}
					p.SetId("p3")
					p.Kind = "tool_request"
					p.Has.Kind = true
					p.SetMimeType("application/json")
					p.SetSizeBytes(9)
					p.SetStorage("inline")
					p.SetCompression("none")
					return p
				}())
				_, _ = s.Patch(context.Background(), func() *write.Payload {
					p := &write.Payload{Has: &write.PayloadHas{}}
					p.SetId("p4")
					p.Kind = "tool_request"
					p.Has.Kind = true
					p.SetMimeType("application/json")
					p.SetSizeBytes(9)
					p.SetStorage("inline")
					p.SetCompression("none")
					return p
				}())
			},
			opts:     []read.InputOption{read.WithIDs("p4")},
			expected: []string{"p4"},
		},
		{
			name: "since filter",
			seed: func(s *Service) {
				_, _ = s.Patch(context.Background(), func() *write.Payload {
					p := &write.Payload{Has: &write.PayloadHas{}}
					p.SetId("p5")
					p.Kind = "model_response"
					p.Has.Kind = true
					p.SetMimeType("application/json")
					p.SetSizeBytes(20)
					p.SetStorage("inline")
					p.SetCompression("none")
					t0 := ts("2024-01-01 10:00:00")
					p.CreatedAt = &t0
					p.Has.CreatedAt = true
					return p
				}())
				_, _ = s.Patch(context.Background(), func() *write.Payload {
					p := &write.Payload{Has: &write.PayloadHas{}}
					p.SetId("p6")
					p.Kind = "model_response"
					p.Has.Kind = true
					p.SetMimeType("application/json")
					p.SetSizeBytes(22)
					p.SetStorage("inline")
					p.SetCompression("none")
					t1 := ts("2024-01-01 11:00:00")
					p.CreatedAt = &t1
					p.Has.CreatedAt = true
					return p
				}())
			},
			opts:     []read.InputOption{read.WithSince(ts("2024-01-01 10:30:00"))},
			expected: []string{"p6"},
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

func TestMemoryPayload_APICompliance(t *testing.T) {
	var _ api.API = New()
}
