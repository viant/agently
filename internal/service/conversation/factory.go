package conversation

import (
	"context"
	"os"
	"strings"

	sqlitesvc "github.com/viant/agently/internal/service/sqlite"
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
	if dsn == "" {
		// Fallback to local SQLite under $AGENTLY_ROOT/db/agently.db
		root := strings.TrimSpace(os.Getenv("AGENTLY_ROOT"))
		sqlite := sqlitesvc.New(root)
		var err error
		if dsn, err = sqlite.Ensure(ctx); err != nil {
			return nil, err
		}
		driver = "sqlite"
	}

	if err := dao.AddConnectors(ctx, view.NewConnector("agently", driver, dsn)); err != nil {
		return nil, err
	}
	return dao, nil
}

// Backward-compatible helper name used elsewhere in the repo.
func NewDatlyServiceFromEnv(ctx context.Context) (*datly.Service, error) { return NewDatly(ctx) }
