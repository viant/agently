package sqlite

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	iscript "github.com/viant/agently/internal/script"
	_ "modernc.org/sqlite"
)

type Service struct {
	root string
}

func New(root string) *Service { return &Service{root: root} }

const (
	sqliteSchemaVersionTable  = "schema_version"
	sqliteBaseSchemaVersion   = 1
	sqliteTargetSchemaVersion = 3
)

// Ensure sets up a SQLite database under $ROOT/db/agently.db when missing and
// ensures the schema is installed. It returns the DSN file path.
func (s *Service) Ensure(ctx context.Context) (string, error) { // ctx kept for future timeouts
	base := s.root
	if strings.TrimSpace(base) == "" {
		wd, _ := os.Getwd()
		base = wd
	}
	dbDir := filepath.Join(base, "db")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create db dir: %w", err)
	}
	dbFile := filepath.Join(dbDir, "agently.db")
	// Use SQLite URI with pragmas to improve concurrency and avoid SQLITE_BUSY
	dsn := "file:" + dbFile + "?cache=shared&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return "", fmt.Errorf("failed to open sqlite db: %w", err)
	}
	defer db.Close()

	// Best-effort apply pragmas (journal_mode persisted at DB level; busy_timeout per-conn)
	_, _ = db.ExecContext(ctx, "PRAGMA journal_mode=WAL")
	_, _ = db.ExecContext(ctx, "PRAGMA busy_timeout=5000")
	_, _ = db.ExecContext(ctx, "PRAGMA synchronous=NORMAL")

	// Check if schema is already present
	var name string
	err = db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='conversation'").Scan(&name)
	if err == nil && name == "conversation" {
		if err := applySQLiteMigrations(ctx, db); err != nil {
			return "", err
		}
		return dsn, nil
	}
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "no rows") {
		// Unexpected error; still attempt to bootstrap schema
	}

	// Load schema from embedded SQLite script (kept in sync with MySQL schema).
	ddlBytes := []byte(iscript.SqlListScript)
	if len(ddlBytes) == 0 {
		return "", fmt.Errorf("embedded sqlite schema is empty")
	}
	// Execute statements line-by-line, accumulating until a terminating ';'.
	scanner := bufio.NewScanner(strings.NewReader(string(ddlBytes)))
	var buf strings.Builder
	flush := func() error {
		stmt := strings.TrimSpace(buf.String())
		if stmt == "" {
			return nil
		}
		if _, execErr := db.ExecContext(ctx, stmt); execErr != nil {
			return fmt.Errorf("schema exec failed: %w (sql: %s)", execErr, stmt)
		}
		buf.Reset()
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		// Skip comments / empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		buf.WriteString(line)
		buf.WriteString("\n")
		if strings.HasSuffix(trimmed, ";") {
			if err := flush(); err != nil {
				return "", err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("schema scan failed: %w", err)
	}
	// Flush trailing statement without semicolon
	if err := flush(); err != nil {
		return "", err
	}
	if err := applySQLiteMigrations(ctx, db); err != nil {
		return "", err
	}
	return dsn, nil
}

func applySQLiteMigrations(ctx context.Context, db *sql.DB) error {
	if err := ensureSQLiteSchemaVersionTable(ctx, db); err != nil {
		return err
	}
	current, err := getSQLiteSchemaVersion(ctx, db)
	if err != nil {
		return err
	}
	migrations := []struct {
		target int
		apply  func(context.Context, *sql.DB) error
	}{
		{target: 3, apply: ensureSQLiteRawContentColumn},
	}
	for _, m := range migrations {
		if current >= m.target {
			continue
		}
		if err := m.apply(ctx, db); err != nil {
			return err
		}
		if err := setSQLiteSchemaVersion(ctx, db, m.target); err != nil {
			return err
		}
		current = m.target
	}
	return nil
}

func ensureSQLiteRawContentColumn(ctx context.Context, db *sql.DB) error {
	const (
		table  = "message"
		column = "raw_content"
	)
	tableExists, err := sqliteTableExists(ctx, db, table)
	if err != nil {
		return fmt.Errorf("check %s table: %w", table, err)
	}
	if !tableExists {
		return nil
	}
	exists, err := sqliteColumnExists(ctx, db, table, column)
	if err != nil {
		return fmt.Errorf("check %s.%s column: %w", table, column, err)
	}
	if exists {
		return nil
	}
	if _, err := db.ExecContext(ctx, "ALTER TABLE message ADD COLUMN raw_content TEXT"); err != nil {
		return fmt.Errorf("add %s.%s column: %w", table, column, err)
	}
	return nil
}

func sqliteColumnExists(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func sqliteTableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var name string
	row := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name=?", table)
	switch err := row.Scan(&name); err {
	case nil:
		return true, nil
	case sql.ErrNoRows:
		return false, nil
	default:
		return false, err
	}
}

func ensureSQLiteSchemaVersionTable(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (version INTEGER NOT NULL)", sqliteSchemaVersionTable)); err != nil {
		return fmt.Errorf("create %s table: %w", sqliteSchemaVersionTable, err)
	}
	var count int
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(1) FROM %s", sqliteSchemaVersionTable)).Scan(&count); err != nil {
		return fmt.Errorf("count %s rows: %w", sqliteSchemaVersionTable, err)
	}
	if count == 0 {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s(version) VALUES (?)", sqliteSchemaVersionTable), sqliteBaseSchemaVersion); err != nil {
			return fmt.Errorf("init %s row: %w", sqliteSchemaVersionTable, err)
		}
	}
	return nil
}

func getSQLiteSchemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var version int
	query := fmt.Sprintf("SELECT COALESCE(MAX(version), ?) FROM %s", sqliteSchemaVersionTable)
	if err := db.QueryRowContext(ctx, query, sqliteBaseSchemaVersion).Scan(&version); err != nil {
		return 0, fmt.Errorf("read %s: %w", sqliteSchemaVersionTable, err)
	}
	return version, nil
}

func setSQLiteSchemaVersion(ctx context.Context, db *sql.DB, version int) error {
	if _, err := db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", sqliteSchemaVersionTable)); err != nil {
		return fmt.Errorf("clear %s: %w", sqliteSchemaVersionTable, err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s(version) VALUES (?)", sqliteSchemaVersionTable), version); err != nil {
		return fmt.Errorf("update %s: %w", sqliteSchemaVersionTable, err)
	}
	return nil
}
