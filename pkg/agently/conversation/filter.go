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

	// If caller explicitly disables default predicate, short-circuit.
	if input.Has.DefaultPredicate && strings.TrimSpace(input.DefaultPredicate) == "1" {
		return trueCriteria, nil
	}
	// If explicitly querying by id, skip visibility/scope filters.
	if input.Has.Id {
		return trueCriteria, nil
	}

	// Limit to top-level, non-scheduled by default (history view),
	// but DO NOT apply when explicitly querying by parent conversation/turn.
	if !(input.Has.Id || input.Has.ParentId || input.Has.ParentTurnId || input.Has.HasScheduleId) {
		exprParts = append(exprParts, "t.conversation_parent_id = '' AND t.schedule_id IS NULL")
	}

	// Enforce visibility: allow public (non-private) or created_by current user.
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
