package impl

import (
	"context"
	"os"
	"strings"

	"github.com/viant/datly"
	"github.com/viant/datly/view"
)

// NewDatly constructs a datly.Service and wires the optional SQL connector
// from AGENTLY_DB_* environment variables. It returns the service with or without
// connector depending on configuration and bubbles up any connector wiring errors.
func NewDatly(ctx context.Context) (*datly.Service, error) {
	dao, err := datly.New(ctx)
	if err != nil {
		return nil, err
	}

	driver := strings.TrimSpace(os.Getenv("AGENTLY_DB_DRIVER"))
	dsn := strings.TrimSpace(os.Getenv("AGENTLY_DB_DSN"))
	if driver == "" || dsn == "" {
		// No SQL configured; return service as-is.
		return dao, nil
	}

	if err := dao.AddConnectors(ctx, view.NewConnector("agently", driver, dsn)); err != nil {
		return nil, err
	}
	return dao, nil
}
