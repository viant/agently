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
	return dsn, nil
}
