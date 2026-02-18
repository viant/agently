package chat

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	authctx "github.com/viant/agently/internal/auth"
	scyauth "github.com/viant/scy/auth"
	"golang.org/x/oauth2"
)

func (s *Service) ensureBearerForTools(ctx context.Context, username string) context.Context {
	if ctx == nil {
		return ctx
	}
	// If we already have usable auth in context (access or id token), do not
	// attempt a refresh. BFF flows often do not provide refresh tokens.
	if tok := authctx.TokensFromContext(ctx); tok != nil {
		if strings.TrimSpace(tok.AccessToken) != "" || strings.TrimSpace(tok.IDToken) != "" {
			return ctx
		}
	}
	if strings.TrimSpace(authctx.IDToken(ctx)) != "" || strings.TrimSpace(authctx.Bearer(ctx)) != "" {
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
	token, tokErr := store.EnsureToken(ctx, userID, prov, configURL)
	if tokErr == nil && token != nil {
		if strings.TrimSpace(os.Getenv("AGENTLY_DEBUG_MCP_AUTH")) != "" {
			uid := userID
			if len(uid) > 8 {
				uid = uid[:8]
			}
			fmt.Fprintf(os.Stderr, "[mcp-auth] ensureBearerForTools user=%s userID=%s accessLen=%d idLen=%d refreshLen=%d exp=%s\n",
				uname, uid, len(strings.TrimSpace(token.AccessToken)), len(strings.TrimSpace(token.IDToken)), len(strings.TrimSpace(token.RefreshToken)), token.ExpiresAt.Format(time.RFC3339))
		}
		ctx = authctx.WithTokens(ctx, &scyauth.Token{Token: oauth2.Token{
			AccessToken:  token.AccessToken,
			TokenType:    "Bearer",
			RefreshToken: token.RefreshToken,
			Expiry:       token.ExpiresAt,
		}, IDToken: token.IDToken})
		if idt := strings.TrimSpace(token.IDToken); idt != "" {
			ctx = authctx.WithIDToken(ctx, idt)
		}
		if at := strings.TrimSpace(token.AccessToken); at != "" {
			ctx = authctx.WithBearer(ctx, at)
		}
		return ctx
	}
	if tokErr != nil {
		uid := userID
		if len(uid) > 8 {
			uid = uid[:8]
		}
		errorf("ensureBearerForTools auth error user=%q user_id=%q provider=%q err=%v", strings.TrimSpace(uname), uid, strings.TrimSpace(prov), tokErr)
	}
	if strings.TrimSpace(os.Getenv("AGENTLY_DEBUG_MCP_AUTH")) != "" {
		uid := userID
		if len(uid) > 8 {
			uid = uid[:8]
		}
		e := ""
		if tokErr != nil {
			e = tokErr.Error()
		}
		fmt.Fprintf(os.Stderr, "[mcp-auth] ensureBearerForTools user=%s userID=%s token=missing err=%s\n", uname, uid, e)
	}
	return ctx
}
