package integrate

import (
	"context"
	"net/http"
	"strings"
)

// oobRT wraps an inner RoundTripper and triggers an OAuthPrompt when a 401
// response includes an authorization_uri in WWW-Authenticate.
type oobRT struct {
	inner  http.RoundTripper
	prompt OAuthPrompt
}

// NewOOBRoundTripper wraps inner with an out-of-band prompt trigger.
func NewOOBRoundTripper(inner http.RoundTripper, prompt OAuthPrompt) http.RoundTripper {
	if inner == nil || prompt == nil {
		return inner
	}
	return &oobRT{inner: inner, prompt: prompt}
}

func (w *oobRT) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := w.inner.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		if url := parseAuthorizationURI(resp); url != "" {
			// Best-effort; do not block or fail the request based on prompt result
			_ = w.prompt.PromptOOB(context.Background(), url, OAuthMeta{Origin: req.URL.Host})
		}
	}
	return resp, nil
}

// parseAuthorizationURI extracts authorization_uri from WWW-Authenticate header
func parseAuthorizationURI(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	header := resp.Header.Get("WWW-Authenticate")
	if header == "" {
		return ""
	}
	// Split by comma, look for authorization_uri="..."
	parts := strings.Split(header, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "authorization_uri=") {
			v := strings.TrimPrefix(p, "authorization_uri=")
			v = strings.Trim(v, "\"")
			return v
		}
	}
	return ""
}
