package v1

import (
	"context"
	"strings"
	"time"

	svcauth "github.com/viant/agently-core/service/auth"
	iauth "github.com/viant/agently/internal/auth"
	"github.com/viant/datly"
)

// sessionStoreAdapter bridges the internal/auth Datly-backed session store
// (used by the original agently router) to the service/auth.SessionStore
// interface used by the v1 agently-core session manager.
type sessionStoreAdapter struct {
	inner *iauth.SessionStoreDAO
}

func newSessionStoreAdapter(dao *datly.Service) svcauth.SessionStore {
	if dao == nil {
		return nil
	}
	return &sessionStoreAdapter{inner: iauth.NewSessionStoreDAO(dao)}
}

func (a *sessionStoreAdapter) Get(ctx context.Context, id string) (*svcauth.SessionRecord, error) {
	rec, err := a.inner.Get(ctx, id)
	if err != nil || rec == nil {
		return nil, err
	}
	return &svcauth.SessionRecord{
		ID:        rec.ID,
		Username:  rec.UserID,
		Subject:   rec.UserID,
		CreatedAt: rec.CreatedAt,
		ExpiresAt: rec.ExpiresAt,
	}, nil
}

func (a *sessionStoreAdapter) Upsert(ctx context.Context, rec *svcauth.SessionRecord) error {
	if rec == nil {
		return nil
	}
	userID := strings.TrimSpace(rec.Username)
	if userID == "" {
		userID = strings.TrimSpace(rec.Subject)
	}
	if userID == "" {
		userID = strings.TrimSpace(rec.Email)
	}
	if userID == "" {
		return nil
	}
	now := time.Now().UTC()
	return a.inner.Upsert(ctx, &iauth.SessionRecord{
		ID:        rec.ID,
		UserID:    userID,
		Provider:  "oauth",
		CreatedAt: rec.CreatedAt,
		UpdatedAt: &now,
		ExpiresAt: rec.ExpiresAt,
	})
}

func (a *sessionStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.inner.Delete(ctx, id)
}
