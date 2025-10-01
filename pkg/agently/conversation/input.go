package conversation

import (
	"context"
	"reflect"
)

var inputKey = reflect.TypeOf(&ConversationInput{})

func InputFromContext(ctx context.Context) *ConversationInput {
	value := ctx.Value(inputKey)
	if value == nil {
		return nil
	}
	return value.(*ConversationInput)
}
