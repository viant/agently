package chat

import (
	"context"
	"strings"

	authctx "github.com/viant/agently/internal/auth"
)

func (s *Service) ensureBearerForTools(ctx context.Context, username string) context.Context {
	if ctx == nil {
		return ctx
	}
	if strings.TrimSpace(authctx.Bearer(ctx)) != "" {
		return ctx
	}
	if s == nil || s.dao == nil || s.authCfg == nil || s.authCfg.OAuth == nil {
		return ctx
	}
	uname := strings.TrimSpace(username)
	if uname == "" {
		return ctx
	}
	// Map username -> userID (UUID) and refresh access token from DB if available.
	userID := ""
	if s.users != nil {
		if u, err := s.users.FindByUsername(ctx, uname); err == nil && u != nil {
			userID = strings.TrimSpace(u.Id)
		}
	}
	if userID == "" {
		return ctx
	}
	configURL := strings.TrimSpace(s.authCfg.OAuth.Client.ConfigURL)
	if configURL == "" {
		return ctx
	}
	store := authctx.NewTokenStoreDAO(s.dao, configURL)
	prov := strings.TrimSpace(s.authCfg.OAuth.Name)
	if prov == "" {
		prov = "oauth"
	}
	if access, _, err := store.EnsureAccessToken(ctx, userID, prov, configURL); err == nil && strings.TrimSpace(access) != "" {
		return authctx.WithBearer(ctx, access)
	}
	return ctx
}
