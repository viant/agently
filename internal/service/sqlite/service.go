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
	sqliteTargetSchemaVersion = 5
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

	// Apply idempotent migrations unconditionally to heal cases where schema_version
	// was advanced but the DDL was only partially applied.
	ensures := []func(context.Context, *sql.DB) error{
		ensureSQLiteRawContentColumn,
		ensureSQLiteTurnQueueSchema,
		ensureSQLiteSchedulerLeaseSchema,
	}
	for _, ensure := range ensures {
		if err := ensure(ctx, db); err != nil {
			return err
		}
	}

	// Keep schema_version monotonic; only bump when behind the target.
	if current < sqliteTargetSchemaVersion {
		if err := setSQLiteSchemaVersion(ctx, db, sqliteTargetSchemaVersion); err != nil {
			return err
		}
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

func ensureSQLiteTurnQueueSchema(ctx context.Context, db *sql.DB) error {
	const table = "turn"
	tableExists, err := sqliteTableExists(ctx, db, table)
	if err != nil {
		return fmt.Errorf("check %s table: %w", table, err)
	}
	if !tableExists {
		return nil
	}

	hasQueueSeq, err := sqliteColumnExists(ctx, db, table, "queue_seq")
	if err != nil {
		return fmt.Errorf("check %s.queue_seq: %w", table, err)
	}
	allowsQueued, err := sqliteTurnAllowsQueuedStatus(ctx, db)
	if err != nil {
		return err
	}

	// If the column is missing we assume the table was created before queueing was introduced
	// and rebuild it to include:
	// - queue_seq
	// - updated status CHECK constraint that allows 'queued'
	if !hasQueueSeq || !allowsQueued {
		return rebuildSQLiteTurnTableWithQueueing(ctx, db)
	}

	// Ensure indexes exist (older schemas may miss them).
	if _, err := db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_turn_conversation ON turn (conversation_id)"); err != nil {
		return fmt.Errorf("create idx_turn_conversation: %w", err)
	}
	if _, err := db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_turn_conv_status_created ON turn (conversation_id, status, created_at)"); err != nil {
		return fmt.Errorf("create idx_turn_conv_status_created: %w", err)
	}
	if _, err := db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_turn_conv_queue_seq ON turn (conversation_id, queue_seq)"); err != nil {
		return fmt.Errorf("create idx_turn_conv_queue_seq: %w", err)
	}
	return nil
}

func sqliteTurnAllowsQueuedStatus(ctx context.Context, db *sql.DB) (bool, error) {
	var sqlText sql.NullString
	row := db.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE type='table' AND name='turn'")
	switch err := row.Scan(&sqlText); err {
	case nil:
		// continue
	case sql.ErrNoRows:
		return true, nil
	default:
		return false, fmt.Errorf("read turn DDL: %w", err)
	}
	if !sqlText.Valid || strings.TrimSpace(sqlText.String) == "" {
		return true, nil
	}
	ddl := strings.ToLower(sqlText.String)
	return strings.Contains(ddl, "'queued'"), nil
}

func rebuildSQLiteTurnTableWithQueueing(ctx context.Context, db *sql.DB) error {
	cols, err := sqliteTableColumns(ctx, db, "turn")
	if err != nil {
		return fmt.Errorf("read turn columns: %w", err)
	}
	if len(cols) == 0 {
		return fmt.Errorf("turn table has no columns")
	}
	has := map[string]bool{}
	for _, c := range cols {
		has[strings.ToLower(c)] = true
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("disable foreign_keys: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "ALTER TABLE turn RENAME TO turn_old"); err != nil {
		return fmt.Errorf("rename turn: %w", err)
	}

	create := `
CREATE TABLE turn
(
    id                      TEXT PRIMARY KEY,
    conversation_id         TEXT      NOT NULL REFERENCES conversation (id) ON DELETE CASCADE,
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    queue_seq               INTEGER,
    status                  TEXT      NOT NULL CHECK (status IN
                                                      ('queued', 'pending', 'running', 'waiting_for_user', 'succeeded',
                                                       'failed', 'canceled')),
    error_message           TEXT,
    started_by_message_id   TEXT,
    retry_of                TEXT,
    agent_id_used           TEXT,
    agent_config_used_id    TEXT,
    model_override_provider TEXT,
    model_override          TEXT,
    model_params_override   TEXT
);
`
	if _, err := tx.ExecContext(ctx, create); err != nil {
		return fmt.Errorf("create turn: %w", err)
	}

	// Copy over any columns that existed on the legacy table.
	dstCols := []string{"id", "conversation_id", "created_at", "status"}
	srcCols := []string{"id", "conversation_id", "created_at", "status"}
	optional := []string{
		"error_message",
		"started_by_message_id",
		"retry_of",
		"agent_id_used",
		"agent_config_used_id",
		"model_override_provider",
		"model_override",
		"model_params_override",
	}
	for _, c := range optional {
		if has[c] {
			dstCols = append(dstCols, c)
			srcCols = append(srcCols, c)
		}
	}
	insert := fmt.Sprintf("INSERT INTO turn (%s) SELECT %s FROM turn_old",
		strings.Join(dstCols, ", "),
		strings.Join(srcCols, ", "),
	)
	if _, err := tx.ExecContext(ctx, insert); err != nil {
		return fmt.Errorf("copy turn data: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "DROP TABLE turn_old"); err != nil {
		return fmt.Errorf("drop turn_old: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_turn_conversation ON turn (conversation_id)"); err != nil {
		return fmt.Errorf("create idx_turn_conversation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_turn_conv_status_created ON turn (conversation_id, status, created_at)"); err != nil {
		return fmt.Errorf("create idx_turn_conv_status_created: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_turn_conv_queue_seq ON turn (conversation_id, queue_seq)"); err != nil {
		return fmt.Errorf("create idx_turn_conv_queue_seq: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return fmt.Errorf("enable foreign_keys: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func ensureSQLiteSchedulerLeaseSchema(ctx context.Context, db *sql.DB) error {
	// schedule table lease fields
	if ok, err := sqliteTableExists(ctx, db, "schedule"); err != nil {
		return fmt.Errorf("check schedule table: %w", err)
	} else if ok {
		if err := ensureSQLiteColumn(ctx, db, "schedule", "lease_owner", "TEXT"); err != nil {
			return err
		}
		if err := ensureSQLiteColumn(ctx, db, "schedule", "lease_until", "TIMESTAMP"); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_schedule_enabled_next_lease ON schedule(enabled, next_run_at, lease_until)"); err != nil {
			return fmt.Errorf("create idx_schedule_enabled_next_lease: %w", err)
		}
	}

	// schedule_run scheduled_for
	if ok, err := sqliteTableExists(ctx, db, "schedule_run"); err != nil {
		return fmt.Errorf("check schedule_run table: %w", err)
	} else if ok {
		if err := ensureSQLiteColumn(ctx, db, "schedule_run", "scheduled_for", "TIMESTAMP"); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, "CREATE UNIQUE INDEX IF NOT EXISTS ux_run_schedule_scheduled_for ON schedule_run(schedule_id, scheduled_for)"); err != nil {
			return fmt.Errorf("create ux_run_schedule_scheduled_for: %w", err)
		}
	}

	return nil
}

func ensureSQLiteColumn(ctx context.Context, db *sql.DB, table, column, decl string) error {
	exists, err := sqliteColumnExists(ctx, db, table, column)
	if err != nil {
		return fmt.Errorf("check %s.%s: %w", table, column, err)
	}
	if exists {
		return nil
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, decl)); err != nil {
		return fmt.Errorf("add %s.%s column: %w", table, column, err)
	}
	return nil
}

func sqliteTableColumns(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
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
			return nil, err
		}
		cols = append(cols, strings.TrimSpace(name))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cols, nil
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
