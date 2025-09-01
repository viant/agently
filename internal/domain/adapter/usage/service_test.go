package usage

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	usagemem "github.com/viant/agently/internal/dao/usage/impl/memory"
	read "github.com/viant/agently/internal/dao/usage/read"
	write "github.com/viant/agently/internal/dao/usage/write"
)

func toGraph(rows []*read.UsageView) []map[string]interface{} {
	var out []map[string]interface{}
	for _, v := range rows {
		row := map[string]interface{}{"conv": v.ConversationID, "provider": v.Provider, "model": v.Model}
		if v.TotalTokens != nil {
			row["tokens"] = *v.TotalTokens
		}
		if v.CallsCount != nil {
			row["calls"] = *v.CallsCount
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i]["conv"].(string) == out[j]["conv"].(string) {
			return out[i]["model"].(string) < out[j]["model"].(string)
		}
		return out[i]["conv"].(string) < out[j]["conv"].(string)
	})
	if out == nil {
		return []map[string]interface{}{}
	}
	return out
}

func TestService_Usage_DataDriven(t *testing.T) {
	ctx := context.Background()
	dao := usagemem.New()
	svc := New(dao)

	// List empty
	rows, err := svc.List(ctx, read.Input{ConversationID: "c1", Has: &read.Has{ConversationID: true}})
	if !assert.NoError(t, err) {
		return
	}
	gotJSON, _ := json.Marshal(toGraph(rows))
	assert.EqualValues(t, "[]", string(gotJSON))

	// Patch totals
	out, err := svc.Patch(ctx, func() *write.Usage {
		u := &write.Usage{}
		u.SetConversationID("c3")
		u.SetUsageInputTokens(1)
		u.SetUsageOutputTokens(2)
		u.SetUsageEmbeddingTokens(3)
		return u
	}())
	if !assert.NoError(t, err) || !assert.NotNil(t, out) {
		return
	}
}
