package conversation

import (
	"context"
	_ "embed"

	"github.com/viant/xdatly/codec"
	"github.com/viant/xdatly/types/core"
	checksum "github.com/viant/xdatly/types/custom/dependency/checksum"

	"reflect"
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

	if input.Has.Id || input.Has.HasScheduleId {
		return trueCriteria, nil
	}
	return &codec.Criteria{
		Expression: "t.conversation_parent_id = '' AND t.schedule_id IS NULL ",
	}, nil
}
