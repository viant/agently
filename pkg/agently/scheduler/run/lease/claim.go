package lease

import (
	"context"
	"embed"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/xdatly/handler"
	"github.com/viant/xdatly/handler/response"
	"github.com/viant/xdatly/types/core"
	"github.com/viant/xdatly/types/custom/dependency/checksum"
)

func init() {
	core.RegisterType("run", "ClaimLeaseInput", reflect.TypeOf(ClaimLeaseInput{}), checksum.GeneratedTime)
	core.RegisterType("run", "ClaimLeaseOutput", reflect.TypeOf(ClaimLeaseOutput{}), checksum.GeneratedTime)
}

//go:embed sql/*.sql
var FS embed.FS

type ClaimLeaseInput struct {
	RunID      string              `parameter:",kind=body,in=runId"`
	LeaseOwner string              `parameter:",kind=body,in=leaseOwner"`
	LeaseUntil time.Time           `parameter:",kind=body,in=leaseUntil"`
	Now        time.Time           `parameter:",kind=body,in=now"`
	Has        *ClaimLeaseInputHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type ClaimLeaseInputHas struct {
	RunID      bool
	LeaseOwner bool
	LeaseUntil bool
	Now        bool
}

type ClaimLeaseOutput struct {
	response.Status `parameter:",kind=output,in=status" anonymous:"true"`
	Claimed         bool `parameter:",kind=body" json:"claimed"`
}

var ClaimLeasePathURI = "/v1/api/agently/scheduler/run/lease/claim"

func DefineClaimLeaseComponent(ctx context.Context, srv *datly.Service) (*repository.Component, error) {
	return srv.AddHandler(ctx, contract.NewPath(http.MethodPost, ClaimLeasePathURI), &ClaimLeaseHandler{},
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(&ClaimLeaseInput{}),
			reflect.TypeOf(&ClaimLeaseOutput{}),
			&FS,
		),
	)
}

type ClaimLeaseHandler struct{}

func (h *ClaimLeaseHandler) Exec(ctx context.Context, sess handler.Session) (interface{}, error) {
	out := &ClaimLeaseOutput{}
	out.Status.Status = "ok"
	in := &ClaimLeaseInput{}
	if err := sess.Stater().Bind(ctx, in); err != nil {
		out.Status.Status = "error"
		out.Status.Message = err.Error()
		return out, err
	}
	if strings.TrimSpace(in.RunID) == "" || strings.TrimSpace(in.LeaseOwner) == "" {
		out.Status.Status = "error"
		out.Status.Message = "runId and leaseOwner are required"
		return out, response.NewError(http.StatusBadRequest, "bad request")
	}
	if in.LeaseUntil.IsZero() {
		out.Status.Status = "error"
		out.Status.Message = "leaseUntil is required"
		return out, response.NewError(http.StatusBadRequest, "bad request")
	}
	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
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
UPDATE schedule_run
SET lease_owner = ?, lease_until = ?
WHERE id = ?
  AND completed_at IS NULL
  AND (lease_until IS NULL OR lease_until < ? OR lease_owner = ?)
`
	res, err := db.ExecContext(ctx, stmt,
		strings.TrimSpace(in.LeaseOwner),
		in.LeaseUntil.UTC(),
		strings.TrimSpace(in.RunID),
		in.Now.UTC(),
		strings.TrimSpace(in.LeaseOwner),
	)
	if err != nil {
		out.Status.Status = "error"
		out.Status.Message = err.Error()
		return out, err
	}
	affected, _ := res.RowsAffected()
	out.Claimed = affected > 0
	if !out.Claimed {
		// MySQL reports RowsAffected=0 when an UPDATE matches but doesn't change any values.
		// This can happen when two claims happen within the same second and TIMESTAMP truncation
		// makes lease_until identical. Treat "already leased by me and unexpired" as claimed.
		checkStmt := `
SELECT 1
FROM schedule_run
WHERE id = ?
  AND completed_at IS NULL
  AND lease_owner = ?
  AND lease_until IS NOT NULL
  AND lease_until >= ?
`
		var ok int
		if qErr := db.QueryRowContext(ctx, checkStmt,
			strings.TrimSpace(in.RunID),
			strings.TrimSpace(in.LeaseOwner),
			in.Now.UTC(),
		).Scan(&ok); qErr == nil && ok == 1 {
			out.Claimed = true
		}
	}
	return out, nil
}
