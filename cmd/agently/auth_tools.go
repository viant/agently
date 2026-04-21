package agently

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/viant/agently-core/sdk"
)

const defaultSessionCookieName = "agently_session"

func ensureToolAuth(ctx context.Context, client *sdk.HTTPClient, providers []authProviderInfo, rawToken, rawSession, rawOOB, rawOAuthCfg, rawOAuthScopes string) error {
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

	if secretRef := strings.TrimSpace(rawOOB); secretRef != "" {
		return authenticateWithOOB(ctx, client, secretRef, strings.TrimSpace(rawOAuthCfg), parseScopes(rawOAuthScopes))
	}
	if envSec := strings.TrimSpace(os.Getenv("AGENTLY_OOB_SECRETS")); envSec != "" {
		if err := authenticateWithOOB(ctx, client, envSec, strings.TrimSpace(rawOAuthCfg), parseScopes(os.Getenv("AGENTLY_OOB_SCOPES"))); err == nil {
			if _, err := client.AuthMe(ctx); err == nil {
				return nil
			}
		}
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

func authenticateWithOOB(ctx context.Context, client *sdk.HTTPClient, secretRef, configURL string, scopes []string) error {
	secretRef = strings.TrimSpace(secretRef)
	if secretRef == "" {
		return fmt.Errorf("--oob requires a secrets URL value")
	}
	return client.AuthLocalOOBSession(ctx, &sdk.LocalOOBSessionOptions{
		ConfigURL:  strings.TrimSpace(configURL),
		SecretsURL: secretRef,
		Scopes:     scopes,
	})
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
