package conversation

import (
	"context"
	_ "embed"
	"reflect"
	"strings"

	authctx "github.com/viant/agently/internal/auth"
	"github.com/viant/xdatly/codec"
	"github.com/viant/xdatly/types/core"
	checksum "github.com/viant/xdatly/types/custom/dependency/checksum"
)

func init() {
	core.RegisterType("conversation", "Filter", reflect.TypeOf(Filter{}), checksum.GeneratedTime)
}

type Filter struct {
}

var trueCriteria = &codec.Criteria{
	Expression: "1=1",
}

var falseCriteria = &codec.Criteria{
	Expression: "0=1",
}

func (a *Filter) Compute(ctx context.Context, value interface{}) (*codec.Criteria, error) {
	if ctx == nil {
		return trueCriteria, nil
	}
	inputValue := ctx.Value(inputKey)
	if inputValue == nil {
		return falseCriteria, nil
	}
	input, ok := inputValue.(*ConversationInput)
	if !ok {
		return falseCriteria, nil
	}

	var exprParts []string
	var args []interface{}

	// Limit to top-level, non-scheduled by default (history view)
	if !(input.Has.Id || input.Has.HasScheduleId) {
		exprParts = append(exprParts, "t.conversation_parent_id = '' AND t.schedule_id IS NULL")
	}

	// Enforce visibility: allow public (non-private) or created_by current user
	userID := strings.TrimSpace(authctx.EffectiveUserID(ctx))
	if userID == "" {
		// Anonymous: only non-private
		exprParts = append(exprParts, "COALESCE(t.visibility, '') <> ?")
		args = append(args, "private")
	} else {
		exprParts = append(exprParts, "(COALESCE(t.visibility, '') <> ? OR t.created_by_user_id = ?)")
		args = append(args, "private", userID)
	}

	return &codec.Criteria{
		Expression:   strings.Join(exprParts, " AND "),
		Placeholders: args,
	}, nil
}
