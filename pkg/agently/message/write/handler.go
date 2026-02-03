package write

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/viant/xdatly/handler"
	"github.com/viant/xdatly/handler/response"
)

type Handler struct{}

func (h *Handler) Exec(ctx context.Context, sess handler.Session) (interface{}, error) {
	out := &Output{}
	out.Status.Status = "ok"
	if err := h.exec(ctx, sess, out); err != nil {
		var rErr *response.Error
		if errors.As(err, &rErr) {
			return out, err
		}
		out.setError(err)
	}
	if len(out.Violations) > 0 { //TODO better error hanlding
		out.setError(fmt.Errorf("failed validation"))
		return out, response.NewError(http.StatusBadRequest, "bad request"+" - failed validation: "+out.Violations[0].Message)
	}
	return out, nil
}

func (h *Handler) exec(ctx context.Context, sess handler.Session, out *Output) error {
	in := &Input{}
	if err := in.Init(ctx, sess, out); err != nil {
		return err
	}
	out.Data = in.Messages
	if err := in.Validate(ctx, sess, out); err != nil || len(out.Violations) > 0 {
		return err
	}
	sql, err := sess.Db()
	if err != nil {
		return err
	}
	db, err := sql.Db(ctx)
	if err != nil {
		return err
	}
	const maxContentBytes = 16777215 //16MB - MEDIUMTEXT in MySQL
	for _, rec := range in.Messages {
		// Truncate content to maxContentBytes preserving valid UTF-8
		if rec != nil && maxContentBytes > 0 {
			if len(rec.Content) > maxContentBytes {
				// Work on at most maxContentBytes bytes
				s := rec.Content[:maxContentBytes]

				// Ensure we don't cut through a multi-byte UTF-8 rune
				for len(s) > 0 && !utf8.ValidString(s) {
					s = s[:len(s)-1]
				}

				rec.Content = s

				// If this flag means "was truncated"
				if rec.Has != nil {
					rec.Has.Content = true
				}
			}
		}

		if rec != nil && maxContentBytes > 0 && rec.RawContent != nil {
			if len(*rec.RawContent) > maxContentBytes {
				// Work on at most maxContentBytes bytes
				s := (*rec.RawContent)[:maxContentBytes]

				// Ensure we don't cut through a multi-byte UTF-8 rune
				for len(s) > 0 && !utf8.ValidString(s) {
					s = s[:len(s)-1]
				}

				rec.RawContent = &s

				// If this flag means "was truncated"
				if rec.Has != nil {
					rec.Has.RawContent = true
				}
			}
		}

		if rec == nil {
			continue
		}
		_, exists := in.CurMessageById[rec.Id]
		if err = upsertMessageWithSequenceRetry(ctx, sql, db, rec, exists); err != nil {
			return err
		}
	}
	return nil
}

func upsertMessageWithSequenceRetry(
	ctx context.Context,
	sqlxSvc interface {
		Insert(string, interface{}) error
		Update(string, interface{}) error
	},
	db interface {
		QueryRowContext(context.Context, string, ...interface{}) *sql.Row
	},
	rec *Message,
	exists bool,
) error {
	const maxRetries = 10
	var err error
	callerProvidedSequence := rec.Sequence != nil
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Assign sequence only for inserts and only when caller didn't set it.
		// This avoids resequencing existing messages on partial updates.
		if !exists && !callerProvidedSequence && rec.Sequence == nil && rec.TurnID != nil && *rec.TurnID != "" {
			seq, seqErr := nextSequenceForTurn(ctx, db, *rec.TurnID)
			if seqErr != nil {
				return seqErr
			}
			rec.SetSequence(seq)
		}

		if !exists {
			err = sqlxSvc.Insert("message", rec)
		} else {
			rec.SetUpdatedAt(time.Now().UTC())
			err = sqlxSvc.Update("message", rec)
		}
		if err == nil {
			return nil
		}
		if !isMessageTurnSequenceConflict(err) || rec.TurnID == nil || *rec.TurnID == "" {
			return err
		}
		// Do not auto-repair updates or explicit sequences.
		if exists || callerProvidedSequence {
			return err
		}

		// Another writer won the same (turn_id, sequence). Retry by allocating a
		// new sequence from the process-wide per-turn sequencer.
		rec.Sequence = nil
		if rec.Has != nil {
			rec.Has.Sequence = false
		}

		// Lightweight backoff; avoid sleeping when ctx is canceled.
		delay := time.Duration(attempt+1) * 5 * time.Millisecond
		if delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return err
}

var globalTurnSeq = &turnSequencer{}

type turnSequencer struct {
	turns sync.Map // map[turnID]*turnSeqState
}

type turnSeqState struct {
	mu          sync.Mutex
	initialized bool
	next        int
}

func (s *turnSequencer) next(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}, turnID string) (int, error) {
	if strings.TrimSpace(turnID) == "" {
		return 0, fmt.Errorf("turnID is required")
	}
	v, _ := s.turns.LoadOrStore(turnID, &turnSeqState{})
	st := v.(*turnSeqState)
	st.mu.Lock()
	defer st.mu.Unlock()

	if !st.initialized {
		max, err := maxSequenceForTurn(ctx, db, turnID)
		if err != nil {
			return 0, err
		}
		st.next = max + 1
		if st.next < 1 {
			st.next = 1
		}
		st.initialized = true
	}

	seq := st.next
	st.next++
	return seq, nil
}

func maxSequenceForTurn(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}, turnID string) (int, error) {
	var max sql.NullInt64
	row := db.QueryRowContext(ctx, "SELECT MAX(sequence) FROM message WHERE turn_id = ?", turnID)
	if err := row.Scan(&max); err != nil {
		return 0, err
	}
	if !max.Valid {
		return 0, nil
	}
	maxInt := int64(^uint(0) >> 1)
	if max.Int64 > maxInt {
		return 0, fmt.Errorf("sequence overflow: %d", max.Int64)
	}
	return int(max.Int64), nil
}

func isMessageTurnSequenceConflict(err error) bool {
	if err == nil {
		return false
	}
	// Note: xdatly/sqlx and drivers can wrap errors; message matching keeps this portable.
	msg := strings.ToLower(err.Error())
	// SQLite: "UNIQUE constraint failed: message.turn_id, message.sequence"
	if strings.Contains(msg, "unique constraint failed") &&
		strings.Contains(msg, "message.turn_id") &&
		strings.Contains(msg, "message.sequence") {
		return true
	}
	// MySQL: "Duplicate entry ... for key 'idx_message_turn_seq'"
	if strings.Contains(msg, "duplicate entry") && strings.Contains(msg, "idx_message_turn_seq") {
		return true
	}
	// Postgres (if used): "duplicate key value violates unique constraint \"idx_message_turn_seq\""
	if strings.Contains(msg, "duplicate key value violates unique constraint") && strings.Contains(msg, "idx_message_turn_seq") {
		return true
	}
	// Generic catch-all when index name is included.
	if strings.Contains(msg, "idx_message_turn_seq") && (strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique")) {
		return true
	}
	return false
}

func nextSequenceForTurn(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}, turnID string) (int, error) {
	return globalTurnSeq.next(ctx, db, turnID)
}
