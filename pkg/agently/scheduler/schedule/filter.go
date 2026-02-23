package schedule

import (
	"context"
	"reflect"
	"strings"

	authctx "github.com/viant/agently/internal/auth"
	"github.com/viant/xdatly/codec"
	"github.com/viant/xdatly/types/core"
	checksum "github.com/viant/xdatly/types/custom/dependency/checksum"
)

func init() {
	core.RegisterType("schedule", "Filter", reflect.TypeOf(Filter{}), checksum.GeneratedTime)
}

// Filter enforces schedule visibility per authenticated user.
// Anonymous callers can see only non-private schedules.
// Authenticated callers can see non-private schedules plus their own private ones.
type Filter struct{}

func (f *Filter) Compute(ctx context.Context, _ interface{}) (*codec.Criteria, error) {
	if ctx == nil {
		return &codec.Criteria{Expression: "1=1"}, nil
	}
	userID := strings.TrimSpace(authctx.EffectiveUserID(ctx))
	if userID == "" {
		return &codec.Criteria{
			Expression:   "COALESCE(t.visibility, '') <> ?",
			Placeholders: []interface{}{"private"},
		}, nil
	}
	return &codec.Criteria{
		Expression:   "(COALESCE(t.visibility, '') <> ? OR t.created_by_user_id = ?)",
		Placeholders: []interface{}{"private", userID},
	}, nil
}
