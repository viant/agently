package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/scy/auth/authorizer"
	"github.com/viant/scy/kms"
	"github.com/viant/scy/kms/blowfish"
	"golang.org/x/oauth2"

	oauthread "github.com/viant/agently/pkg/agently/user/oauth"
	oauthwrite "github.com/viant/agently/pkg/agently/user/oauth/write"
)

// NewTokenStoreDAO constructs a Datly-backed token store (package-level for external calls).
func NewTokenStoreDAO(dao *datly.Service, salt string) *TokenStoreDAO {
	return &TokenStoreDAO{dao: dao, salt: salt}
}

// TokenStoreDAO uses Datly operate with internal components to persist encrypted tokens.
type TokenStoreDAO struct {
	dao  *datly.Service
	salt string
}

var tokCipherDatly = blowfish.Cipher{}

func (s *TokenStoreDAO) encrypt(ctx context.Context, t *OAuthToken) (string, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	key := &kms.Key{Kind: "raw", Raw: string(blowfish.EnsureKey([]byte(s.salt)))}
	enc, err := tokCipherDatly.Encrypt(ctx, key, b)
	if err != nil {
		return "", err
	}
	return base64RawURL(enc), nil
}

func (s *TokenStoreDAO) decrypt(ctx context.Context, enc string) (*OAuthToken, error) {
	raw, err := base64RawURLDecode(enc)
	if err != nil {
		return nil, err
	}
	key := &kms.Key{Kind: "raw", Raw: string(blowfish.EnsureKey([]byte(s.salt)))}
	dec, err := tokCipherDatly.Decrypt(ctx, key, raw)
	if err != nil {
		return nil, err
	}
	var t OAuthToken
	if err := json.Unmarshal(dec, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// Get loads and decrypts token from DB.
func (s *TokenStoreDAO) Get(ctx context.Context, userID, provider string) (*OAuthToken, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	out := &oauthread.TokenOutput{}
	in := oauthread.TokenInput{}
	in.Has = &oauthread.TokenInputHas{Id: true}
	in.Id = userID
	if _, err := s.dao.Operate(ctx, datly.WithPath(contract.NewPath("GET", oauthread.TokenPathURI)), datly.WithInput(&in), datly.WithOutput(out)); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 || out.Data[0] == nil {
		return nil, nil
	}
	var row *oauthread.TokenView
	if strings.TrimSpace(provider) != "" {
		for _, r := range out.Data {
			if r != nil && strings.TrimSpace(r.Provider) == strings.TrimSpace(provider) {
				row = r
				break
			}
		}
	}
	if row == nil {
		row = out.Data[0]
	}
	if row == nil || strings.TrimSpace(row.EncToken) == "" {
		return nil, nil
	}
	return s.decrypt(ctx, row.EncToken)
}

// Upsert encrypts and saves token via internal write handler.
func (s *TokenStoreDAO) Upsert(ctx context.Context, userID, provider string, tok *OAuthToken) error {
	if s == nil || s.dao == nil {
		return nil
	}
	enc, err := s.encrypt(ctx, tok)
	if err != nil {
		return err
	}
	in := &oauthwrite.Input{Token: &oauthwrite.Token{}}
	in.Token.SetUserID(userID)
	in.Token.SetProvider(provider)
	in.Token.SetEncToken(enc)
	out := &oauthwrite.Output{}
	_, err = s.dao.Operate(ctx, datly.WithPath(contract.NewPath("PATCH", oauthwrite.PathURI)), datly.WithInput(in), datly.WithOutput(out))
	return err
}

// EnsureAccessToken refreshes if needed; updates DB on rotation.
func (s *TokenStoreDAO) EnsureAccessToken(ctx context.Context, userID, provider, configURL string) (string, time.Time, error) {
	tok, err := s.Get(ctx, userID, provider)
	if err != nil || tok == nil {
		return "", time.Time{}, err
	}
	if tok.AccessToken != "" && tok.ExpiresAt.After(time.Now().Add(60*time.Second)) {
		return tok.AccessToken, tok.ExpiresAt, nil
	}
	oa := authorizer.New()
	oc := &authorizer.OAuthConfig{ConfigURL: configURL}
	if err := oa.EnsureConfig(ctx, oc); err != nil {
		return "", time.Time{}, err
	}
	src := oc.Config.TokenSource(ctx, &oauth2.Token{RefreshToken: tok.RefreshToken, Expiry: time.Now().Add(-time.Hour)})
	nt, err := src.Token()
	if err != nil {
		return "", time.Time{}, err
	}
	tok.AccessToken = nt.AccessToken
	tok.ExpiresAt = nt.Expiry
	if r := nt.RefreshToken; r != "" { //
		tok.RefreshToken = r
	}
	if id := nt.Extra("id_token"); id != nil {
		if s, ok := id.(string); ok && s != "" {
			tok.IDToken = s
		}
	}
	if err := s.Upsert(ctx, userID, provider, tok); err != nil {
		return "", time.Time{}, err
	}
	return tok.AccessToken, tok.ExpiresAt, nil
}

// helpers
func base64RawURL(b []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=")
}
func base64RawURLDecode(s string) ([]byte, error) {
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}
