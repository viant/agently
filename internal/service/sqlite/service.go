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
	path string
}

func New(root string) *Service { return &Service{root: root} }

// WithPath overrides the sqlite file path.
func (s *Service) WithPath(path string) *Service {
	if s == nil {
		return s
	}
	s.path = strings.TrimSpace(path)
	return s
}

const (
	sqliteSchemaVersionTable  = "schema_version"
	sqliteBaseSchemaVersion   = 1
	sqliteTargetSchemaVersion = 9
)

// Ensure sets up a SQLite database under $ROOT/db/agently.db when missing and
// ensures the schema is installed. It returns the DSN file path.
func (s *Service) Ensure(ctx context.Context) (string, error) { // ctx kept for future timeouts
	base := s.root
	if strings.TrimSpace(base) == "" {
		wd, _ := os.Getwd()
		base = wd
	}
	dbFile := strings.TrimSpace(s.path)
	if dbFile == "" {
		dbDir := filepath.Join(base, "db")
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create db dir: %w", err)
		}
		dbFile = filepath.Join(dbDir, "agently.db")
	} else {
		if err := os.MkdirAll(filepath.Dir(dbFile), 0755); err != nil {
			return "", fmt.Errorf("failed to create db dir: %w", err)
		}
	}
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
		ensureSQLiteMessageTypeConstraint,
		ensureSQLiteTurnQueueSchema,
		ensureSQLiteSchedulerLeaseSchema,
		ensureSQLiteScheduleUserCredURL,
		ensureSQLiteSessionTable,
		ensureSQLiteEmbediusTables,
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

func ensureSQLiteScheduleUserCredURL(ctx context.Context, db *sql.DB) error {
	const (
		table  = "schedule"
		column = "user_cred_url"
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
	if _, err := db.ExecContext(ctx, "ALTER TABLE schedule ADD COLUMN user_cred_url TEXT"); err != nil {
		return fmt.Errorf("add %s.%s column: %w", table, column, err)
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

func ensureSQLiteMessageTypeConstraint(ctx context.Context, db *sql.DB) error {
	const table = "message"
	tableExists, err := sqliteTableExists(ctx, db, table)
	if err != nil {
		return fmt.Errorf("check %s table: %w", table, err)
	}
	if !tableExists {
		return nil
	}
	ok, err := sqliteMessageTypeConstraintOK(ctx, db)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	if err := rebuildSQLiteMessageTable(ctx, db); err != nil {
		return fmt.Errorf("rebuild %s: %w", table, err)
	}
	return nil
}

func sqliteMessageTypeConstraintOK(ctx context.Context, db *sql.DB) (bool, error) {
	const query = "SELECT sql FROM sqlite_master WHERE type='table' AND name='message'"
	var ddl sql.NullString
	if err := db.QueryRowContext(ctx, query).Scan(&ddl); err != nil {
		return false, fmt.Errorf("read message table ddl: %w", err)
	}
	if !ddl.Valid {
		return false, nil
	}
	sqlText := strings.ToLower(ddl.String)
	return strings.Contains(sqlText, "elicitation_request") && strings.Contains(sqlText, "elicitation_response"), nil
}

func rebuildSQLiteMessageTable(ctx context.Context, db *sql.DB) error {
	const createMessageNew = `
CREATE TABLE message_new
(
    id                 TEXT PRIMARY KEY,
    archived           INTEGER   CHECK (archived IN (0, 1)),
    conversation_id    TEXT      NOT NULL REFERENCES conversation (id) ON DELETE CASCADE,
    turn_id            TEXT      REFERENCES turn (id) ON DELETE SET NULL,
    sequence           INTEGER,
    created_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMP,
    created_by_user_id TEXT,
    status             TEXT CHECK (status IS NULL OR status IN ('', 'pending','accepted','rejected','cancel','open','summary','summarized', 'completed','error')),
    mode               TEXT,
    role               TEXT      NOT NULL CHECK (role IN ('system', 'user', 'assistant', 'tool', 'chain')),
    type               TEXT      NOT NULL DEFAULT 'text' CHECK (type IN ('text', 'tool_op',  'control', 'elicitation_request', 'elicitation_response')),
    content            TEXT,
    raw_content        TEXT,
    summary            TEXT,
    context_summary    TEXT,
    tags               TEXT,
    interim            INTEGER   NOT NULL DEFAULT 0 CHECK (interim IN (0, 1)),
    elicitation_id     TEXT,
    parent_message_id  TEXT,
    superseded_by      TEXT,
    linked_conversation_id  TEXT,
    attachment_payload_id  TEXT REFERENCES call_payload (id) ON DELETE SET NULL,
    elicitation_payload_id TEXT REFERENCES call_payload (id) ON DELETE SET NULL,
    -- legacy column to remain compatible with older readers
    tool_name          TEXT,
    embedding_index    BLOB
);`
	allowedCols := map[string]struct{}{
		"id":                     {},
		"archived":               {},
		"conversation_id":        {},
		"turn_id":                {},
		"sequence":               {},
		"created_at":             {},
		"updated_at":             {},
		"created_by_user_id":     {},
		"status":                 {},
		"mode":                   {},
		"role":                   {},
		"type":                   {},
		"content":                {},
		"raw_content":            {},
		"summary":                {},
		"context_summary":        {},
		"tags":                   {},
		"interim":                {},
		"elicitation_id":         {},
		"parent_message_id":      {},
		"superseded_by":          {},
		"linked_conversation_id": {},
		"attachment_payload_id":  {},
		"elicitation_payload_id": {},
		"tool_name":              {},
		"embedding_index":        {},
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("disable foreign_keys: %w", err)
	}
	if _, err := tx.ExecContext(ctx, createMessageNew); err != nil {
		return fmt.Errorf("create message_new: %w", err)
	}

	existingCols, err := sqliteTableColumns(ctx, tx, "message")
	if err != nil {
		return err
	}
	var cols []string
	for _, col := range existingCols {
		if _, ok := allowedCols[col]; ok {
			cols = append(cols, col)
		}
	}
	if len(cols) == 0 {
		return fmt.Errorf("no columns to copy for message table")
	}
	colList := strings.Join(cols, ", ")
	insertSQL := fmt.Sprintf("INSERT INTO message_new (%s) SELECT %s FROM message", colList, colList)
	if _, err := tx.ExecContext(ctx, insertSQL); err != nil {
		return fmt.Errorf("copy message rows: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE message"); err != nil {
		return fmt.Errorf("drop message: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "ALTER TABLE message_new RENAME TO message"); err != nil {
		return fmt.Errorf("rename message_new: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "CREATE UNIQUE INDEX idx_message_turn_seq ON message (turn_id, sequence)"); err != nil {
		return fmt.Errorf("create idx_message_turn_seq: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "CREATE INDEX idx_msg_conv_created ON message (conversation_id, created_at DESC)"); err != nil {
		return fmt.Errorf("create idx_msg_conv_created: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return fmt.Errorf("enable foreign_keys: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
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

func ensureSQLiteSessionTable(ctx context.Context, db *sql.DB) error {
	const table = "session"
	exists, err := sqliteTableExists(ctx, db, table)
	if err != nil {
		return fmt.Errorf("check %s table: %w", table, err)
	}
	if exists {
		return nil
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE session (
  id          TEXT      PRIMARY KEY,
  user_id     TEXT      NOT NULL,
  provider    TEXT      NOT NULL,
  created_at  DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  DATETIME,
  expires_at  DATETIME  NOT NULL
);`); err != nil {
		return fmt.Errorf("create %s table: %w", table, err)
	}
	if _, err := db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_session_user_id ON session(user_id)"); err != nil {
		return fmt.Errorf("create idx_session_user_id: %w", err)
	}
	if _, err := db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_session_provider ON session(provider)"); err != nil {
		return fmt.Errorf("create idx_session_provider: %w", err)
	}
	if _, err := db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_session_expires_at ON session(expires_at)"); err != nil {
		return fmt.Errorf("create idx_session_expires_at: %w", err)
	}
	return nil
}

func ensureSQLiteEmbediusTables(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS vec_dataset (
  dataset_id   TEXT PRIMARY KEY,
  description  TEXT,
  source_uri   TEXT,
  last_scn     INTEGER NOT NULL DEFAULT 0
)`,
		`CREATE TABLE IF NOT EXISTS vec_dataset_scn (
  dataset_id TEXT PRIMARY KEY,
  next_scn   INTEGER NOT NULL DEFAULT 0
)`,
		`CREATE TABLE IF NOT EXISTS vec_shadow_log (
  dataset_id   TEXT NOT NULL,
  shadow_table TEXT NOT NULL,
  scn          INTEGER NOT NULL,
  op           TEXT NOT NULL,
  document_id  TEXT NOT NULL,
  payload      BLOB NOT NULL,
  created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(dataset_id, shadow_table, scn)
)`,
		`CREATE TABLE IF NOT EXISTS shadow_vec_docs (
  dataset_id       TEXT NOT NULL,
  id               TEXT NOT NULL,
  content          TEXT,
  meta             TEXT,
  embedding        BLOB,
  embedding_model  TEXT,
  scn              INTEGER NOT NULL DEFAULT 0,
  archived         INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(dataset_id, id)
)`,
		`CREATE TABLE IF NOT EXISTS emb_root (
  dataset_id      TEXT PRIMARY KEY,
  source_uri      TEXT,
  description     TEXT,
  last_indexed_at DATETIME NULL,
  last_scn        INTEGER NOT NULL DEFAULT 0
)`,
		`CREATE TABLE IF NOT EXISTS emb_root_config (
  dataset_id     TEXT PRIMARY KEY,
  include_globs  TEXT,
  exclude_globs  TEXT,
  max_size_bytes INTEGER NOT NULL DEFAULT 0,
  updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)`,
		`CREATE TABLE IF NOT EXISTS emb_asset (
  dataset_id TEXT NOT NULL,
  asset_id   TEXT NOT NULL,
  path       TEXT NOT NULL,
  md5        TEXT NOT NULL,
  size       INTEGER NOT NULL,
  mod_time   DATETIME NOT NULL,
  scn        INTEGER NOT NULL,
  archived   INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (dataset_id, asset_id)
)`,
		`CREATE INDEX IF NOT EXISTS idx_emb_asset_path ON emb_asset(dataset_id, path)`,
		`CREATE INDEX IF NOT EXISTS idx_emb_asset_mod ON emb_asset(dataset_id, mod_time)`,
		`CREATE INDEX IF NOT EXISTS idx_shadow_vec_docs_scn ON shadow_vec_docs(dataset_id, scn)`,
		`CREATE INDEX IF NOT EXISTS idx_shadow_vec_docs_archived ON shadow_vec_docs(dataset_id, archived)`,
	}

	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("embedius schema: %w", err)
		}
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

type sqliteQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func sqliteTableColumns(ctx context.Context, db sqliteQueryer, table string) ([]string, error) {
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
