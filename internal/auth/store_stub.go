package auth

import (
	"context"
	"time"
)

// Store is a minimal placeholder token store (no-op persistence).
// A full implementation can be added to persist encrypted tokens to DB.
type Store struct{ salt string }

func NewTokenStore(salt string) *Store { return &Store{salt: salt} }

func (s *Store) Upsert(ctx context.Context, userID, provider string, tok *OAuthToken) error {
	return nil
}

func (s *Store) EnsureAccessToken(ctx context.Context, userID, provider, configURL string) (string, time.Time, error) {
	return "", time.Time{}, nil
}
