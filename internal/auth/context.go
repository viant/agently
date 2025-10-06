package auth

import (
	"context"
	"strings"
)

// ctx keys use unexported distinct types to avoid collisions.
type (
	bearerKey   struct{}
	userInfoKey struct{}
	idTokenKey  struct{}
)

// UserInfo carries minimal identity extracted from a bearer token.
type UserInfo struct {
	Subject string
	Email   string
}

// WithBearer stores a raw bearer token in context.
func WithBearer(ctx context.Context, token string) context.Context {
	if ctx == nil || token == "" {
		return ctx
	}
	return context.WithValue(ctx, bearerKey{}, token)
}

// Bearer returns a raw bearer token from context, if present.
func Bearer(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(bearerKey{}).(string); ok {
		return v
	}
	return ""
}

// WithIDToken stores a raw ID token in context.
func WithIDToken(ctx context.Context, token string) context.Context {
	if ctx == nil || strings.TrimSpace(token) == "" {
		return ctx
	}
	return context.WithValue(ctx, idTokenKey{}, token)
}

// IDToken returns a raw ID token from context, if present.
func IDToken(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(idTokenKey{}).(string); ok {
		return v
	}
	return ""
}

// WithUserInfo stores identity data in context.
func WithUserInfo(ctx context.Context, info *UserInfo) context.Context {
	if ctx == nil || info == nil {
		return ctx
	}
	return context.WithValue(ctx, userInfoKey{}, *info)
}

// User returns identity data from context when available.
func User(ctx context.Context) *UserInfo {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Value(userInfoKey{}).(UserInfo); ok {
		return &v
	}
	return nil
}

// EffectiveUserID returns a stable user identifier from context (subject or email).
// Returns empty string when no identity is present.
func EffectiveUserID(ctx context.Context) string {
	if ui := User(ctx); ui != nil {
		if s := strings.TrimSpace(ui.Subject); s != "" {
			return s
		}
		if e := strings.TrimSpace(ui.Email); e != "" {
			return e
		}
	}
	return ""
}

// EnsureUser populates a user identity in context when missing using config
// fallbacks (e.g., local mode default username). Returns the original context
// when no action is needed.
func EnsureUser(ctx context.Context, cfg *Config) context.Context {
	if ctx == nil {
		return ctx
	}
	if ui := User(ctx); ui != nil {
		if strings.TrimSpace(ui.Subject) != "" || strings.TrimSpace(ui.Email) != "" {
			return ctx
		}
	}
	if cfg != nil && cfg.IsLocalAuth() {
		if u := strings.TrimSpace(cfg.DefaultUsername); u != "" {
			return WithUserInfo(ctx, &UserInfo{Subject: u})
		}
	}
	return ctx
}
