package write

import (
	"context"
	"github.com/viant/xdatly/handler"
	"github.com/viant/xdatly/handler/validator"
)

func (i *Input) Validate(ctx context.Context, sess handler.Session, out *Output) error {
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
		validator.WithLocation("Users"),
		validator.WithDB(db),
		validator.WithUnique(true),
		validator.WithRefCheck(true),
		validator.WithCanUseMarkerProvider(i.canUseMarkerProvider),
	}
	validation := validator.NewValidation()
	_, err = v.Validate(ctx, i.Users, append(opts, validator.WithValidation(validation))...)
	out.Violations = append(out.Violations, validation.Violations...)
	if err == nil && len(validation.Violations) > 0 {
		validation.Violations.Sort()
	}
	return err
}

func (i *Input) canUseMarkerProvider(v interface{}) bool {
	switch actual := v.(type) {
	case *User:
		_, ok := i.CurUserById[actual.Id]
		return ok
	default:
		return true
	}
}
