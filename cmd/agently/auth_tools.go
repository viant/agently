package agently

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/viant/agently-core/sdk"
)

const defaultSessionCookieName = "agently_session"

func ensureToolAuth(ctx context.Context, client *sdk.HTTPClient, providers []authProviderInfo, rawToken, rawSession string) error {
	if client == nil {
		return fmt.Errorf("client is required")
	}
	if sessionID := strings.TrimSpace(rawSession); sessionID != "" {
		if err := applySessionCookie(client, sessionID); err != nil {
			return err
		}
		if _, err := client.AuthMe(ctx); err == nil {
			return nil
		}
	}
	if _, err := client.AuthMe(ctx); err == nil {
		return nil
	}

	if token := resolvedToken(rawToken); token != "" {
		// Prefer bearer-based session bootstrap first so the raw token is sent
		// in the Authorization header when the server supports that flow.
		if err := client.AuthSessionExchange(ctx, token); err == nil {
			if _, err := client.AuthMe(ctx); err == nil {
				return nil
			}
		}
		for _, req := range []*sdk.CreateSessionRequest{
			{AccessToken: token},
			{IDToken: token},
		} {
			if err := client.AuthCreateSession(ctx, req); err != nil {
				continue
			}
			if _, err := client.AuthMe(ctx); err == nil {
				return nil
			}
		}
	}

	chat := &ChatCmd{Token: strings.TrimSpace(rawToken)}
	return chat.ensureAuth(ctx, client, providers)
}

func applySessionCookie(client *sdk.HTTPClient, sessionID string) error {
	if client == nil {
		return fmt.Errorf("client is required")
	}
	baseURL := strings.TrimSpace(client.BaseURL())
	if baseURL == "" {
		return fmt.Errorf("base URL is required")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("parse base URL: %w", err)
	}
	jar := client.HTTPClient().Jar
	if jar == nil {
		return fmt.Errorf("http cookie jar is not configured")
	}
	jar.SetCookies(u, []*http.Cookie{{
		Name:     defaultSessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
	}})
	return nil
}
