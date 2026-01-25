package augmenter

import (
	"fmt"
	"path/filepath"

	"github.com/viant/embedius/vectordb/sqlitevec"
)

func newSQLiteStore(baseURL string) (*sqlitevec.Store, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL is required")
	}
	dbPath := filepath.Join(baseURL, "embedius.sqlite")
	store, err := sqlitevec.NewStore(
		sqlitevec.WithDSN(dbPath),
		sqlitevec.WithEnsureSchema(true),
		sqlitevec.WithWAL(true),
		sqlitevec.WithBusyTimeout(5000),
	)
	if err != nil {
		return nil, err
	}
	store.SetSCNAllocator(sqlitevec.DefaultSCNAllocator(store.DB()))
	return store, nil
}
