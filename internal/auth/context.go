package auth

import (
	"context"
)

// ctx keys use unexported distinct types to avoid collisions.
type (
	bearerKey   struct{}
	userInfoKey struct{}
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
