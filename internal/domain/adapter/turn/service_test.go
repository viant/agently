package turn

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	turmem "github.com/viant/agently/internal/dao/turn/impl/memory"
	read "github.com/viant/agently/internal/dao/turn/read"
	write "github.com/viant/agently/pkg/agently/turn"
)

func toGraph(rows []*read.TurnView) []map[string]interface{} {
	var out []map[string]interface{}
	for _, v := range rows {
		row := map[string]interface{}{"id": v.Id, "conv": v.ConversationID, "status": v.Status}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i]["id"].(string) < out[j]["id"].(string) })
	return out
}

func TestService_Turns_DataDriven(t *testing.T) {
	type testCase struct {
		name   string
		seed   func(ctx context.Context, svc *Service)
		opts   []read.InputOption
		expect string
	}

	running := "running"

	cases := []testCase{
		{
			name: "start and list by conversation",
			seed: func(ctx context.Context, svc *Service) {
				w := &write.Turn{}
				w.SetId("t1")
				w.SetConversationID("c1")
				w.SetStatus("running")
				_, _ = svc.Start(ctx, w)
			},
			opts:   []read.InputOption{read.WithConversationID("c1")},
			expect: `[{"conv":"c1","id":"t1","status":"running"}]`,
		},
		{
			name: "update status then list by id",
			seed: func(ctx context.Context, svc *Service) {
				w := &write.Turn{}
				w.SetId("t2")
				w.SetConversationID("c2")
				w.SetStatus("pending")
				_, _ = svc.Start(ctx, w)
				u := &write.Turn{}
				u.SetId("t2")
				u.SetStatus("failed")
				_ = svc.Update(ctx, u)
			},
			opts:   []read.InputOption{read.WithIDs("t2")},
			expect: `[{"conv":"c2","id":"t2","status":"failed"}]`,
		},
		{
			name: "filter by status",
			seed: func(ctx context.Context, svc *Service) {
				a := &write.Turn{}
				a.SetId("t3")
				a.SetConversationID("c3")
				a.SetStatus("running")
				_, _ = svc.Start(ctx, a)
				b := &write.Turn{}
				b.SetId("t4")
				b.SetConversationID("c3")
				b.SetStatus("succeeded")
				_, _ = svc.Start(ctx, b)
			},
			opts:   []read.InputOption{read.WithStatus(running)},
			expect: `[{"conv":"c3","id":"t3","status":"running"}]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			dao := turmem.New()
			svc := New(dao)
			if tc.seed != nil {
				tc.seed(ctx, svc)
			}
			rows, err := svc.List(ctx, tc.opts...)
			if !assert.NoError(t, err) {
				return
			}
			gotJSON, _ := json.Marshal(toGraph(rows))
			var gotSlice []map[string]interface{}
			_ = json.Unmarshal(gotJSON, &gotSlice)
			var expSlice []map[string]interface{}
			_ = json.Unmarshal([]byte(tc.expect), &expSlice)
			assert.EqualValues(t, expSlice, gotSlice)
		})
	}
}

func strp(s string) *string { return &s }
