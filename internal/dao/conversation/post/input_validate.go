package post

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
	var options = []validator.Option{
		validator.WithLocation("Conversations"),
		validator.WithDB(db),
		validator.WithUnique(true),
		validator.WithRefCheck(true),
		validator.WithCanUseMarkerProvider(i.canUseMarkerProvider)}
	validation := validator.NewValidation()
	err = i.validate(ctx, aValidator, validation, options)
	output.Violations = append(output.Violations, validation.Violations...)
	if err == nil && len(validation.Violations) > 0 {
		validation.Violations.Sort()
	}
	return err
}

func (i *Input) validate(ctx context.Context, aValidator *validator.Service, validation *validator.Validation, options []validator.Option) error {
	_, err := aValidator.Validate(ctx, i.Conversations, append(options, validator.WithValidation(validation))...)
	if err != nil {
		return err
	}
	//todo: add your custom validation logic here
	return err
}

func (i *Input) canUseMarkerProvider(v interface{}) bool {

	switch actual := v.(type) {
	case *Message:
		_, ok := i.CurMessageById[actual.Id]
		return ok
	case *Conversation:
		_, ok := i.CurConversationById[actual.Id]
		return ok

	default:
		return true
	}

}
