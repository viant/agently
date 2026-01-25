package dbtest

import (
	"bufio"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/viant/agently/internal/script"
)

// ParameterizedSQL represents a statement with optional parameters.
type ParameterizedSQL struct {
	SQL    string
	Params []interface{}
}

// ExecAll executes statements sequentially, failing the test on first error.
func ExecAll(t *testing.T, db *sql.DB, items []ParameterizedSQL) {
	t.Helper()
	for _, it := range items {
		if strings.TrimSpace(it.SQL) == "" {
			continue
		}
		if _, err := db.Exec(it.SQL, it.Params...); err != nil {
			t.Fatalf("exec SQL failed: %v\nSQL: %s", err, it.SQL)
		}
	}
}

// LoadDDLFromFile reads a DDL file and executes contained statements separated by ';'.
func LoadDDLFromFile(t *testing.T, db *sql.DB, path string) {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ddl: %v", err)
	}
	var ddls []ParameterizedSQL
	for _, stmt := range strings.Split(string(bytes), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		ddls = append(ddls, ParameterizedSQL{SQL: stmt})
	}
	ExecAll(t, db, ddls)
}

// LoadSQLiteSchema loads the embedded SQLite schema DDL and executes it.
func LoadSQLiteSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	ddl := script.SqlListScript
	if strings.TrimSpace(ddl) == "" {
		t.Fatalf("embedded sqlite schema is empty")
	}
	scanner := bufio.NewScanner(strings.NewReader(ddl))
	var buf strings.Builder
	flush := func() {
		stmt := strings.TrimSpace(buf.String())
		if stmt == "" {
			return
		}
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("schema exec failed: %v\nSQL: %s", err, stmt)
		}
		buf.Reset()
	}
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		buf.WriteString(line)
		buf.WriteString("\n")
		if strings.HasSuffix(trimmed, ";") {
			flush()
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("schema scan failed: %v", err)
	}
	flush()
}

// CreateTempSQLiteDB creates a temporary SQLite database file and opens a connection.
// It returns the *sql.DB, its path, and a cleanup function that closes the DB and
// removes the temp directory.
func CreateTempSQLiteDB(t *testing.T, prefix string) (*sql.DB, string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	dbPath := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("open db: %v", err)
	}
	cleanup := func() {
		_ = db.Close()
		_ = os.RemoveAll(dir)
	}
	return db, dbPath, cleanup
}
