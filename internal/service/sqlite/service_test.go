package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestServiceEnsure_MigratesRawContent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	type testCase struct {
		name         string
		hasRawColumn bool
	}

	cases := []testCase{
		{name: "adds missing column", hasRawColumn: false},
		{name: "no-op when column already exists", hasRawColumn: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			dbDir := filepath.Join(root, "db")
			require.NoError(t, os.MkdirAll(dbDir, 0o755))
			dbPath := filepath.Join(dbDir, "agently.db")
			legacyDSN := "file:" + dbPath

			db, err := sql.Open("sqlite", legacyDSN)
			require.NoError(t, err)
			require.NoError(t, seedLegacySchema(db, tc.hasRawColumn))
			require.NoError(t, db.Close())

			svc := New(root)
			dsn, err := svc.Ensure(ctx)
			require.NoError(t, err)

			conn, err := sql.Open("sqlite", dsn)
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })

			hasRaw, err := sqliteColumnExists(ctx, conn, "message", "raw_content")
			require.NoError(t, err)
			assert.EqualValues(t, true, hasRaw)
			hasQueueSeq, err := sqliteColumnExists(ctx, conn, "turn", "queue_seq")
			require.NoError(t, err)
			assert.EqualValues(t, true, hasQueueSeq)
			version := fetchSchemaVersion(t, conn)
			assert.EqualValues(t, sqliteTargetSchemaVersion, version)
		})
	}
}

func seedLegacySchema(db *sql.DB, includeRaw bool) error {
	stmts := []string{
		"CREATE TABLE IF NOT EXISTS conversation (id TEXT PRIMARY KEY);",
		"CREATE TABLE IF NOT EXISTS turn (id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, status TEXT NOT NULL CHECK (status IN ('pending','running','waiting_for_user','succeeded','failed','canceled')));",
		"CREATE TABLE IF NOT EXISTS message (id TEXT PRIMARY KEY, content TEXT" + legacyRawColumnDDL(includeRaw) + ");",
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func legacyRawColumnDDL(includeRaw bool) string {
	if includeRaw {
		return ", raw_content TEXT"
	}
	return ""
}

func fetchSchemaVersion(t *testing.T, db *sql.DB) int {
	var version int
	query := "SELECT COALESCE(MAX(version), ?) FROM " + sqliteSchemaVersionTable
	require.NoError(t, db.QueryRow(query, sqliteBaseSchemaVersion).Scan(&version))
	return version
}
