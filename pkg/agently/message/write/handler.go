package write

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
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
	nextSeq := map[string]int{}
	for _, rec := range in.Messages {
		if rec != nil && rec.Sequence == nil && rec.TurnID != nil && *rec.TurnID != "" {
			seq, err := nextSequenceForTurn(ctx, db, nextSeq, *rec.TurnID)
			if err != nil {
				return err
			}
			rec.SetSequence(seq)
		}
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

		if _, ok := in.CurMessageById[rec.Id]; !ok {
			if err = sql.Insert("message", rec); err != nil {
				return err
			}
		} else {
			rec.SetUpdatedAt(time.Now().UTC())
			if err = sql.Update("message", rec); err != nil {
				return err
			}
		}
	}
	return nil
}

func nextSequenceForTurn(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}, cache map[string]int, turnID string) (int, error) {
	if v, ok := cache[turnID]; ok {
		v++
		cache[turnID] = v
		return v, nil
	}
	var max sql.NullInt64
	row := db.QueryRowContext(ctx, "SELECT MAX(sequence) FROM message WHERE turn_id = ?", turnID)
	if err := row.Scan(&max); err != nil {
		return 0, err
	}
	next := 1
	if max.Valid {
		next = int(max.Int64) + 1
	}
	cache[turnID] = next
	return next, nil
}
