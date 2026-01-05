package lease

import (
	"context"
	"net/http"
	"reflect"
	"strings"

	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/xdatly/handler"
	"github.com/viant/xdatly/handler/response"
	"github.com/viant/xdatly/types/core"
	"github.com/viant/xdatly/types/custom/dependency/checksum"
)

func init() {
	core.RegisterType("schedule", "ReleaseLeaseInput", reflect.TypeOf(ReleaseLeaseInput{}), checksum.GeneratedTime)
	core.RegisterType("schedule", "ReleaseLeaseOutput", reflect.TypeOf(ReleaseLeaseOutput{}), checksum.GeneratedTime)
}

type ReleaseLeaseInput struct {
	ScheduleID string                `parameter:",kind=body,in=scheduleId"`
	LeaseOwner string                `parameter:",kind=body,in=leaseOwner"`
	Has        *ReleaseLeaseInputHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type ReleaseLeaseInputHas struct {
	ScheduleID bool
	LeaseOwner bool
}

type ReleaseLeaseOutput struct {
	response.Status `parameter:",kind=output,in=status" anonymous:"true"`
	Released        bool `parameter:",kind=body" json:"released"`
}

var ReleaseLeasePathURI = "/v1/api/agently/scheduler/schedule/lease/release"

func DefineReleaseLeaseComponent(ctx context.Context, srv *datly.Service) (*repository.Component, error) {
	return srv.AddHandler(ctx, contract.NewPath(http.MethodPost, ReleaseLeasePathURI), &ReleaseLeaseHandler{},
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(&ReleaseLeaseInput{}),
			reflect.TypeOf(&ReleaseLeaseOutput{}),
			&FS,
		),
	)
}

type ReleaseLeaseHandler struct{}

func (h *ReleaseLeaseHandler) Exec(ctx context.Context, sess handler.Session) (interface{}, error) {
	out := &ReleaseLeaseOutput{}
	out.Status.Status = "ok"
	in := &ReleaseLeaseInput{}
	if err := sess.Stater().Bind(ctx, in); err != nil {
		out.Status.Status = "error"
		out.Status.Message = err.Error()
		return out, err
	}
	if strings.TrimSpace(in.ScheduleID) == "" || strings.TrimSpace(in.LeaseOwner) == "" {
		out.Status.Status = "error"
		out.Status.Message = "scheduleId and leaseOwner are required"
		return out, response.NewError(http.StatusBadRequest, "bad request")
	}
	sqlx, err := sess.Db()
	if err != nil {
		out.Status.Status = "error"
		out.Status.Message = err.Error()
		return out, err
	}
	db, err := sqlx.Db(ctx)
	if err != nil {
		out.Status.Status = "error"
		out.Status.Message = err.Error()
		return out, err
	}
	stmt := `
UPDATE schedule
SET lease_owner = NULL, lease_until = NULL
WHERE id = ?
  AND lease_owner = ?
`
	res, err := db.ExecContext(ctx, stmt, strings.TrimSpace(in.ScheduleID), strings.TrimSpace(in.LeaseOwner))
	if err != nil {
		out.Status.Status = "error"
		out.Status.Message = err.Error()
		return out, err
	}
	affected, _ := res.RowsAffected()
	out.Released = affected > 0
	return out, nil
}
