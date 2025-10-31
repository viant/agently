package run

import (
	"context"
	_ "embed"
	"strings"

	authctx "github.com/viant/agently/internal/auth"
	"github.com/viant/xdatly/codec"
	"github.com/viant/xdatly/types/core"
	checksum "github.com/viant/xdatly/types/custom/dependency/checksum"
	"reflect"
)

func init() {
	core.RegisterType("run", "Filter", reflect.TypeOf(Filter{}), checksum.GeneratedTime)
}

// Filter builds privacy criteria for schedule runs, limiting results to runs whose
// linked conversation is public or owned by the current user. When no identity
// is present, only public-linked runs are returned. Runs without a conversation_id
// are excluded to avoid leaking ownership.
type Filter struct{}

func (f *Filter) Compute(ctx context.Context, _ interface{}) (*codec.Criteria, error) {
	if ctx == nil {
		return &codec.Criteria{Expression: "1=1"}, nil
	}
	// Pull effective user id (subject/email) when available
	userID := strings.TrimSpace(authctx.EffectiveUserID(ctx))

	// Build EXISTS clause against conversation for privacy enforcement.
	// We intentionally exclude runs without conversation_id to avoid ambiguous ownership.
	var expr string
	var args []interface{}
	if userID == "" {
		expr = "t.conversation_id IS NOT NULL AND EXISTS (SELECT 1 FROM conversation c WHERE c.id = t.conversation_id AND COALESCE(c.visibility, '') <> ?)"
		args = append(args, "private")
	} else {
		expr = "t.conversation_id IS NOT NULL AND EXISTS (SELECT 1 FROM conversation c WHERE c.id = t.conversation_id AND (COALESCE(c.visibility, '') <> ? OR c.created_by_user_id = ?))"
		args = append(args, "private", userID)
	}
	return &codec.Criteria{Expression: expr, Placeholders: args}, nil
}
