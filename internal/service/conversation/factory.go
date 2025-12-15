package conversation

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	sqlitesvc "github.com/viant/agently/internal/service/sqlite"
	"github.com/viant/datly"
	"github.com/viant/datly/view"
	"github.com/viant/scy"
	"github.com/viant/scy/cred"
)

const (
	defaultConnMaxLifetime = 55 * time.Minute
	defaultConnMaxIdle     = 5 * time.Minute
	defaultMaxIdleConns    = 4
)

// NewDatly constructs a datly.Service and wires the optional SQL connector
// from AGENTLY_DB_* environment variables. It returns the service with or without
// connector depending on configuration and bubbles up any connector wiring errors.
func NewDatly(ctx context.Context) (*datly.Service, error) {
	// Singleton provider to ensure a single datly.Service across the process
	var initErr error
	daoOnce.Do(func() {
		var svc *datly.Service
		svc, initErr = datly.New(ctx)
		if initErr != nil {
			return
		}

		driver := strings.TrimSpace(os.Getenv("AGENTLY_DB_DRIVER"))
		dsn := strings.TrimSpace(os.Getenv("AGENTLY_DB_DSN"))
		secrets := strings.TrimSpace(os.Getenv("AGENTLY_DB_SECRETS"))

		if dsn == "" {
			// Fallback to local SQLite under $AGENTLY_WORKSPACE/db/agently.db
			root := strings.TrimSpace(os.Getenv("AGENTLY_WORKSPACE"))
			sqlite := sqlitesvc.New(root)
			var err error
			if dsn, err = sqlite.Ensure(ctx); err != nil {
				initErr = err
				return
			}
			driver = "sqlite"
		}
		conn := view.NewConnector("agently", driver, dsn)
		if secrets != "" {
			conn.Secret = scy.EncodedResource(secrets).Decode(ctx, &cred.Basic{})
		}
		if strings.EqualFold(driver, "mysql") {
			if conn.ConnMaxLifetimeMs == 0 {
				conn.ConnMaxLifetimeMs = int(defaultConnMaxLifetime / time.Millisecond)
			}
			if conn.ConnMaxIdleTimeMs == 0 {
				conn.ConnMaxIdleTimeMs = int(defaultConnMaxIdle / time.Millisecond)
			}
			if conn.MaxIdleConns == 0 {
				conn.MaxIdleConns = defaultMaxIdleConns
			}
		}
		if err := svc.AddConnectors(ctx, conn); err != nil {
			initErr = err
			return
		}
		sharedDAO = svc
	})
	if initErr != nil {
		return nil, initErr
	}
	return sharedDAO, nil
}

// Backward-compatible helper name used elsewhere in the repo.
func NewDatlyServiceFromEnv(ctx context.Context) (*datly.Service, error) { return NewDatly(ctx) }

var (
	sharedDAO *datly.Service
	daoOnce   sync.Once
)
