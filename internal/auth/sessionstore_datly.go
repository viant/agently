package auth

import (
	"context"
	"strings"
	"time"

	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"

	sessionread "github.com/viant/agently/pkg/agently/user/session"
	sessiondelete "github.com/viant/agently/pkg/agently/user/session/delete"
	sessionwrite "github.com/viant/agently/pkg/agently/user/session/write"
)

// SessionRecord is the minimal persisted session data.
type SessionRecord struct {
	ID        string
	UserID    string
	Provider  string
	CreatedAt time.Time
	UpdatedAt *time.Time
	ExpiresAt time.Time
}

// NewSessionStoreDAO constructs a Datly-backed session store.
func NewSessionStoreDAO(dao *datly.Service) *SessionStoreDAO {
	return &SessionStoreDAO{dao: dao}
}

// SessionStoreDAO uses Datly operate with internal components to persist sessions.
type SessionStoreDAO struct {
	dao *datly.Service
}

// Get loads a session by id.
func (s *SessionStoreDAO) Get(ctx context.Context, id string) (*SessionRecord, error) {
	if s == nil || s.dao == nil || strings.TrimSpace(id) == "" {
		return nil, nil
	}
	out := &sessionread.SessionOutput{}
	in := sessionread.SessionInput{Id: id, Has: &sessionread.SessionInputHas{Id: true}}
	if _, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath("GET", sessionread.SessionPathURI)),
		datly.WithInput(&in),
		datly.WithOutput(out),
	); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 || out.Data[0] == nil {
		return nil, nil
	}
	row := out.Data[0]
	return &SessionRecord{
		ID:        row.Id,
		UserID:    row.UserId,
		Provider:  row.Provider,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
		ExpiresAt: row.ExpiresAt,
	}, nil
}

// Upsert inserts or updates a session record.
func (s *SessionStoreDAO) Upsert(ctx context.Context, rec *SessionRecord) error {
	if s == nil || s.dao == nil || rec == nil {
		return nil
	}
	if strings.TrimSpace(rec.ID) == "" || strings.TrimSpace(rec.UserID) == "" || strings.TrimSpace(rec.Provider) == "" {
		return nil
	}
	in := &sessionwrite.Input{Session: &sessionwrite.Session{}}
	in.Session.SetID(rec.ID)
	in.Session.SetUserID(rec.UserID)
	in.Session.SetProvider(rec.Provider)
	in.Session.SetExpiresAt(rec.ExpiresAt)
	out := &sessionwrite.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath("PATCH", sessionwrite.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	return err
}

// Delete removes a session by id.
func (s *SessionStoreDAO) Delete(ctx context.Context, id string) error {
	if s == nil || s.dao == nil || strings.TrimSpace(id) == "" {
		return nil
	}
	in := &sessiondelete.Input{Ids: []string{id}}
	out := &sessiondelete.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath("DELETE", sessiondelete.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	return err
}
