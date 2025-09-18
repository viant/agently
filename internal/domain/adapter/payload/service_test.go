package payload

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	plmem "github.com/viant/agently/internal/dao/payload/impl/memory"
	plread "github.com/viant/agently/internal/dao/payload/read"
	plwrite "github.com/viant/agently/pkg/agently/payload"
)

func toGraph(rows []*plread.PayloadView) []map[string]interface{} {
	var out []map[string]interface{}
	for _, v := range rows {
		row := map[string]interface{}{"id": v.Id, "kind": v.Kind, "mime": v.MimeType, "storage": v.Storage, "size": v.SizeBytes}
		if v.URI != nil {
			row["uri"] = *v.URI
		}
		if v.InlineBody != nil {
			row["inline"] = len(*v.InlineBody)
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i]["id"].(string) < out[j]["id"].(string) })
	return out
}

func TestService_Payloads_DataDriven(t *testing.T) {
	type testCase struct {
		name   string
		seed   []*plwrite.Payload
		list   []plread.InputOption
		expect string
	}

	inline := []byte("hello")

	cases := []testCase{
		{
			name: "patch and list all",
			seed: []*plwrite.Payload{
				func() *plwrite.Payload {
					w := &plwrite.Payload{Id: "p1", Has: &plwrite.PayloadHas{Id: true}}
					w.SetKind("text")
					w.SetMimeType("text/plain")
					w.SetSizeBytes(5)
					w.SetStorage("inline")
					w.SetInlineBody(inline)
					return w
				}(),
				func() *plwrite.Payload {
					w := &plwrite.Payload{Id: "p2", Has: &plwrite.PayloadHas{Id: true}}
					w.SetKind("json")
					w.SetMimeType("application/json")
					w.SetSizeBytes(123)
					w.SetStorage("object")
					uri := "s3://bucket/key"
					w.SetURI(uri)
					return w
				}(),
			},
			expect: `[{"id":"p1","inline":5,"kind":"text","mime":"text/plain","size":5,"storage":"inline"},{"id":"p2","kind":"json","mime":"application/json","size":123,"storage":"object","uri":"s3://bucket/key"}]`,
		},
		{
			name: "filter by kind",
			seed: []*plwrite.Payload{
				func() *plwrite.Payload {
					w := &plwrite.Payload{Id: "p3", Has: &plwrite.PayloadHas{Id: true}}
					w.SetKind("image")
					w.SetMimeType("image/png")
					w.SetSizeBytes(10)
					w.SetStorage("inline")
					return w
				}(),
			},
			list:   []plread.InputOption{plread.WithKind("image")},
			expect: `[{"id":"p3","kind":"image","mime":"image/png","size":10,"storage":"inline"}]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			dao := plmem.New()
			svc := New(dao)
			if len(tc.seed) > 0 {
				if _, err := svc.Patch(ctx, tc.seed...); !assert.NoError(t, err) {
					return
				}
			}
			rows, err := svc.List(ctx, tc.list...)
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
