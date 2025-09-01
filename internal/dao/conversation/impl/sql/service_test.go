package sql

import (
	"context"
	"testing"

	"github.com/viant/datly/view"
)

// loadDDL reads the shared schema and executes each statement.
func newV2Service(t *testing.T, dbPath string) *Service {
	t.Helper()
	connector := view.NewConnector("agently", "sqlite", dbPath)
	srv, err := New(context.Background(), connector)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return srv
}
