package augmenter

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/viant/embedius/vectordb/sqlitevec"
)

func newSQLiteStore(baseURL string) (*sqlitevec.Store, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL is required")
	}
	dbPath := defaultSQLitePath(baseURL)
	return newSQLiteStoreWithDB(dbPath)
}

func defaultSQLitePath(baseURL string) string {
	return filepath.Join(baseURL, "embedius.sqlite")
}

func newSQLiteStoreWithDB(dbPath string) (*sqlitevec.Store, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("dbPath is required")
	}
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
