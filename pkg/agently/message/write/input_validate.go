package write

import (
	"context"

	"github.com/viant/xdatly/handler"
	"github.com/viant/xdatly/handler/validator"
)

func (i *Input) Validate(ctx context.Context, sess handler.Session, output *Output) error {
	aValidator := sess.Validator()
	sessionDb, err := sess.Db()
	if err != nil {
		return err
	}
	db, err := sessionDb.Db(ctx)
	if err != nil {
		return err
	}
	opts := []validator.Option{
		validator.WithLocation("Messages"),
		validator.WithDB(db),
		validator.WithUnique(true),
		validator.WithRefCheck(true),
		validator.WithCanUseMarkerProvider(i.canUseMarkerProvider),
	}
	validation := validator.NewValidation()
	_, err = aValidator.Validate(ctx, i.Messages, append(opts, validator.WithValidation(validation))...)
	output.Violations = append(output.Violations, validation.Violations...)
	if err == nil && len(validation.Violations) > 0 {
		validation.Violations.Sort()
	}
	return err
}

func (i *Input) canUseMarkerProvider(v interface{}) bool {
	switch actual := v.(type) {
	case *Message:
		_, ok := i.CurMessageById[actual.Id]
		return ok
	default:
		return true
	}
}
