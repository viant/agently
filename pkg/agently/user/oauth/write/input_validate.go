package write

import (
	"context"
	"strings"

	"github.com/viant/xdatly/handler"
	"github.com/viant/xdatly/handler/validator"
)

func (i *Input) Validate(ctx context.Context, sess handler.Session, out *Output) error {
	if i.Token == nil {
		return nil
	}
	v := sess.Validator()
	sqlx, err := sess.Db()
	if err != nil {
		return err
	}
	db, err := sqlx.Db(ctx)
	if err != nil {
		return err
	}
	opts := []validator.Option{
		validator.WithLocation("Token"),
		validator.WithDB(db),
		validator.WithUnique(true),
		validator.WithRefCheck(true),
		validator.WithCanUseMarkerProvider(i.canUseMarkerProvider),
	}
	validation := validator.NewValidation()
	_, err = v.Validate(ctx, i.Token, append(opts, validator.WithValidation(validation))...)
	out.Violations = append(out.Violations, validation.Violations...)
	if err == nil && len(validation.Violations) > 0 {
		validation.Violations.Sort()
	}
	return err
}

func (i *Input) canUseMarkerProvider(v interface{}) bool {
	actual, ok := v.(*Token)
	if !ok || actual == nil {
		return true
	}
	if i.CurToken == nil {
		return false
	}
	return strings.TrimSpace(i.CurToken.UserID) == strings.TrimSpace(actual.UserID) &&
		strings.TrimSpace(i.CurToken.Provider) == strings.TrimSpace(actual.Provider)
}
