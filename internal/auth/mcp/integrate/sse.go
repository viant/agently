package integrate

import (
    "context"
    "io"
    "net/http"
    "time"
)

// SSEOpenFunc opens an SSE/streaming HTTP request and returns a ReadCloser for the event stream.
// The function implementation should honor the Authorization header present on the request context
// (via ContextWithAuthToken) and/or an explicit header set by the caller.
type SSEOpenFunc func(ctx context.Context, token string) (io.ReadCloser, *http.Response, error)

// OpenSSEWithAuth opens an SSE stream with bearer-first auth, and performs a single re-open on
// auth-related failures using a freshly obtained token. The provided open function should create
// a new HTTP request using the bearer token.
func OpenSSEWithAuth(ctx context.Context, open SSEOpenFunc, tokenFn func(context.Context) (string, time.Time, error)) (io.ReadCloser, *http.Response, error) {
    token, _, err := tokenFn(ctx)
    if err != nil {
        return nil, nil, err
    }
    // First attempt
    rc, resp, err := open(ContextWithAuthToken(ctx, token), token)
    if err == nil && resp != nil && resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != 419 {
        return rc, resp, nil
    }
    // If unauthorized at open, attempt once more with fresh token
    if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == 419) {
        // Acquire a fresh token
        token, _, err = tokenFn(ctx)
        if err != nil {
            return nil, resp, err
        }
        return open(ContextWithAuthToken(ctx, token), token)
    }
    // Non-401 errors just propagate
    return rc, resp, err
}

