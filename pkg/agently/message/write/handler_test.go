package write

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently/internal/testutil/dbtest"
	"github.com/viant/xdatly/handler"
	"github.com/viant/xdatly/handler/auth"
	"github.com/viant/xdatly/handler/differ"
	hhttp "github.com/viant/xdatly/handler/http"
	"github.com/viant/xdatly/handler/logger"
	"github.com/viant/xdatly/handler/mbus"
	"github.com/viant/xdatly/handler/sqlx"
	"github.com/viant/xdatly/handler/state"
	"github.com/viant/xdatly/handler/validator"
	_ "modernc.org/sqlite"
)

func TestHandler_Exec_AssignsSequence(t *testing.T) {
	db, _, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-message-write")
	t.Cleanup(cleanup)
	dbtest.LoadSQLiteSchema(t, db)

	seedConversationTurn(t, db, "c1", "t1")

	in := &Input{Messages: []*Message{
		{Id: "m1", ConversationID: "c1", TurnID: strPtr("t1"), Role: "assistant", Type: "text", Content: "hello"},
		{Id: "m2", ConversationID: "c1", TurnID: strPtr("t1"), Role: "assistant", Type: "text", Content: "world"},
	}}

	sqlxSvc := sqlx.New(&sqliteMessageSQLX{db: db})
	sess := newSQLiteSession(in, sqlxSvc, validator.New(&fakeValidator{}))

	outAny, err := (&Handler{}).Exec(context.Background(), sess)
	require.NoError(t, err)
	out, ok := outAny.(*Output)
	require.True(t, ok)
	require.Equal(t, "ok", out.Status.Status)

	seq1 := fetchMessageSequence(t, db, "m1")
	seq2 := fetchMessageSequence(t, db, "m2")
	require.Equal(t, int64(1), seq1)
	require.Equal(t, int64(2), seq2)
}

func TestHandler_Exec_AssignsSequenceAfterExisting(t *testing.T) {
	db, _, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-message-write")
	t.Cleanup(cleanup)
	dbtest.LoadSQLiteSchema(t, db)

	seedConversationTurn(t, db, "c2", "t2")
	seedMessage(t, db, "m0", "c2", "t2", 7)

	in := &Input{Messages: []*Message{
		{Id: "m3", ConversationID: "c2", TurnID: strPtr("t2"), Role: "assistant", Type: "text", Content: "next"},
	}}

	sqlxSvc := sqlx.New(&sqliteMessageSQLX{db: db})
	sess := newSQLiteSession(in, sqlxSvc, validator.New(&fakeValidator{}))

	_, err := (&Handler{}).Exec(context.Background(), sess)
	require.NoError(t, err)

	seq := fetchMessageSequence(t, db, "m3")
	require.Equal(t, int64(8), seq)
}

func TestHandler_Exec_PreservesSequence(t *testing.T) {
	db, _, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-message-write")
	t.Cleanup(cleanup)
	dbtest.LoadSQLiteSchema(t, db)

	seedConversationTurn(t, db, "c3", "t3")

	in := &Input{Messages: []*Message{
		{Id: "m4", ConversationID: "c3", TurnID: strPtr("t3"), Role: "assistant", Type: "text", Content: "fixed", Sequence: intPtr(42)},
	}}

	sqlxSvc := sqlx.New(&sqliteMessageSQLX{db: db})
	sess := newSQLiteSession(in, sqlxSvc, validator.New(&fakeValidator{}))

	_, err := (&Handler{}).Exec(context.Background(), sess)
	require.NoError(t, err)

	seq := fetchMessageSequence(t, db, "m4")
	require.Equal(t, int64(42), seq)
}

func TestHandler_Exec_AssignsSequence_ConcurrentInserts(t *testing.T) {
	db, _, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-message-write")
	t.Cleanup(cleanup)
	dbtest.LoadSQLiteSchema(t, db)

	seedConversationTurn(t, db, "c4", "t4")

	barrier := newInsertBarrier(2)
	sqlxSvc := sqlx.New(&sqliteMessageSQLX{db: db, insertBarrier: barrier})

	sess1 := newSQLiteSession(&Input{Messages: []*Message{
		{Id: "m5", ConversationID: "c4", TurnID: strPtr("t4"), Role: "assistant", Type: "text", Content: "a"},
	}}, sqlxSvc, validator.New(&fakeValidator{}))

	sess2 := newSQLiteSession(&Input{Messages: []*Message{
		{Id: "m6", ConversationID: "c4", TurnID: strPtr("t4"), Role: "assistant", Type: "text", Content: "b"},
	}}, sqlxSvc, validator.New(&fakeValidator{}))

	h := &Handler{}
	ctx := context.Background()
	var wg sync.WaitGroup
	type execResult struct {
		out *Output
		err error
	}
	resCh := make(chan execResult, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		outAny, err := h.Exec(ctx, sess1)
		out, _ := outAny.(*Output)
		resCh <- execResult{out: out, err: err}
	}()
	go func() {
		defer wg.Done()
		outAny, err := h.Exec(ctx, sess2)
		out, _ := outAny.(*Output)
		resCh <- execResult{out: out, err: err}
	}()
	wg.Wait()
	close(resCh)
	for res := range resCh {
		require.NoError(t, res.err)
		require.NotNil(t, res.out)
		require.Equal(t, "ok", res.out.Status.Status, res.out.Status.Message)
	}

	seq1 := fetchMessageSequence(t, db, "m5")
	seq2 := fetchMessageSequence(t, db, "m6")
	require.NotZero(t, seq1)
	require.NotZero(t, seq2)
	require.NotEqual(t, seq1, seq2)

	// With two inserts into an empty turn, the only valid sequences are 1 and 2.
	require.ElementsMatch(t, []int64{1, 2}, []int64{seq1, seq2})
}

type sqliteMessageSQLX struct {
	db            *sql.DB
	insertBarrier *insertBarrier
}

func (s *sqliteMessageSQLX) Allocate(ctx context.Context, tableName string, dest interface{}, selector string) error {
	return nil
}
func (s *sqliteMessageSQLX) Load(ctx context.Context, tableName string, data interface{}) error {
	return nil
}
func (s *sqliteMessageSQLX) Flush(ctx context.Context, tableName string) error { return nil }
func (s *sqliteMessageSQLX) Insert(tableName string, data interface{}) error {
	m, ok := data.(*Message)
	if !ok || m == nil {
		return nil
	}
	if s.insertBarrier != nil {
		after := s.insertBarrier.Wait()
		defer after()
	}
	createdAt := time.Now().UTC()
	if m.CreatedAt != nil {
		createdAt = *m.CreatedAt
	}
	interim := 0
	if m.Interim != nil {
		interim = *m.Interim
	}
	_, err := s.db.Exec(
		`INSERT INTO message (id, conversation_id, turn_id, role, type, content, sequence, created_at, interim) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.Id, m.ConversationID, m.TurnID, m.Role, m.Type, m.Content, m.Sequence, createdAt, interim,
	)
	return err
}
func (s *sqliteMessageSQLX) Update(tableName string, data interface{}) error {
	m, ok := data.(*Message)
	if !ok || m == nil {
		return nil
	}
	updatedAt := time.Now().UTC()
	if m.UpdatedAt != nil {
		updatedAt = *m.UpdatedAt
	}
	_, err := s.db.Exec(`UPDATE message SET sequence = ?, updated_at = ? WHERE id = ?`, m.Sequence, updatedAt, m.Id)
	return err
}
func (s *sqliteMessageSQLX) Delete(tableName string, data interface{}) error { return nil }
func (s *sqliteMessageSQLX) Execute(DML string, options ...sqlx.ExecutorOption) error {
	return nil
}
func (s *sqliteMessageSQLX) Read(ctx context.Context, dest interface{}, SQL string, params ...interface{}) error {
	return nil
}
func (s *sqliteMessageSQLX) Db(ctx context.Context) (*sql.DB, error) { return s.db, nil }
func (s *sqliteMessageSQLX) Tx(ctx context.Context) (*sql.Tx, error) { return s.db.BeginTx(ctx, nil) }
func (s *sqliteMessageSQLX) Validator() *validator.Service           { return validator.New(&fakeValidator{}) }

type sqliteSession struct {
	input     *Input
	sqlx      *sqlx.Service
	validator *validator.Service
}

func newSQLiteSession(input *Input, sqlxSvc *sqlx.Service, v *validator.Service) *sqliteSession {
	return &sqliteSession{input: input, sqlx: sqlxSvc, validator: v}
}

func (s *sqliteSession) Validator() *validator.Service { return s.validator }
func (s *sqliteSession) Differ() *differ.Service       { return nil }
func (s *sqliteSession) MessageBus() *mbus.Service     { return nil }
func (s *sqliteSession) Db(opts ...sqlx.Option) (*sqlx.Service, error) {
	return s.sqlx, nil
}
func (s *sqliteSession) Stater() *state.Service { return state.New(&stubInjector{input: s.input}) }
func (s *sqliteSession) FlushTemplate(context.Context) error {
	return nil
}
func (s *sqliteSession) Session(ctx context.Context, route *hhttp.Route, opts ...state.Option) (handler.Session, error) {
	return s, nil
}
func (s *sqliteSession) Http() hhttp.Http { return nil }
func (s *sqliteSession) Auth() auth.Auth  { return nil }
func (s *sqliteSession) Logger() logger.Logger {
	return nil
}

type stubInjector struct {
	input *Input
}

func (s *stubInjector) Into(ctx context.Context, any interface{}, opt ...state.Option) error {
	return s.Bind(ctx, any, opt...)
}

func (s *stubInjector) Bind(ctx context.Context, any interface{}, _ ...state.Option) error {
	switch dst := any.(type) {
	case *Input:
		*dst = *s.input
	}
	return nil
}

func (s *stubInjector) Value(context.Context, string) (interface{}, bool, error) {
	return nil, false, nil
}

func (s *stubInjector) ValuesOf(context.Context, interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

type fakeValidator struct{}

func (v *fakeValidator) Validate(ctx context.Context, any interface{}, opts ...validator.Option) (*validator.Validation, error) {
	options := &validator.Options{}
	options.Apply(opts)
	if options.WithValidation == nil {
		options.WithValidation = validator.NewValidation()
	}
	return options.WithValidation, nil
}

func seedConversationTurn(t *testing.T, db *sql.DB, convID, turnID string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO conversation (id, visibility) VALUES (?, ?)`, convID, "private")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO turn (id, conversation_id, created_at, status) VALUES (?, ?, ?, ?)`, turnID, convID, time.Now().UTC(), "running")
	require.NoError(t, err)
}

func seedMessage(t *testing.T, db *sql.DB, msgID, convID, turnID string, seq int64) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO message (id, conversation_id, turn_id, role, type, content, sequence, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		msgID, convID, turnID, "assistant", "text", "seed", seq, time.Now().UTC())
	require.NoError(t, err)
}

func fetchMessageSequence(t *testing.T, db *sql.DB, msgID string) int64 {
	t.Helper()
	var seq sql.NullInt64
	err := db.QueryRow(`SELECT sequence FROM message WHERE id = ?`, msgID).Scan(&seq)
	require.NoError(t, err)
	if !seq.Valid {
		return 0
	}
	return seq.Int64
}

func strPtr(v string) *string { return &v }
func intPtr(v int) *int       { return &v }

type insertBarrier struct {
	n       int32
	counter atomic.Int32
	ready   chan struct{}
	done    chan struct{}
}

func newInsertBarrier(n int) *insertBarrier {
	return &insertBarrier{
		n:     int32(n),
		ready: make(chan struct{}),
		done:  make(chan struct{}),
	}
}

// Wait coordinates the first N calls so that:
// - the first caller waits until Nth caller arrives, then proceeds
// - the Nth caller waits until the first caller signals completion, then proceeds
// - subsequent callers proceed immediately
//
// This keeps inserts serialized (avoids SQLITE_BUSY in tests) while still ensuring
// all callers computed their sequence before any insert is executed.
func (b *insertBarrier) Wait() func() {
	if b == nil || b.n <= 0 {
		return func() {}
	}
	callN := b.counter.Add(1)
	if callN > b.n {
		return func() {}
	}

	// Only supports n=2 for now (used by the concurrent insert regression test).
	if b.n != 2 {
		return func() {}
	}

	if callN == 2 {
		close(b.ready)
		<-b.done
		return func() {}
	}

	<-b.ready
	return func() { close(b.done) }
}
