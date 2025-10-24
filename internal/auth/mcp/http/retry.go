package http

import (
    "net/http"
    "time"
)

// AttemptFunc executes a single HTTP attempt with the provided auth header key/value.
// It should return the HTTP status code and any error encountered making the request.
type AttemptFunc func(authKey, authValue string) (statusCode int, err error)

// WithBearerFirstRetry performs a bearer-first attempt using BuildAuthHeader, and on 401/419
// invalidates the access token, obtains a fresh one via the resolver, and retries once.
func WithBearerFirstRetry(p AuthParams, attempt AttemptFunc) (statusCode int, err error) {
    // initial header
    key, val, ok, err := BuildAuthHeader(p)
    if err != nil {
        return 0, err
    }
    if !ok {
        // No reuse; let caller decide how to proceed without auth header
        return attempt("", "")
    }
    code, err := attempt(key, val)
    if err != nil {
        return code, err
    }
    if code != http.StatusUnauthorized && code != 419 { // 419: session/CSRF expired in some stacks
        return code, nil
    }
    // Invalidate and refresh once
    if p.Resolver == nil {
        return code, nil
    }
    if p.Tracer != nil { p.Tracer.Debug("auth_retry_401", map[string]any{"origin": p.TargetOrigin, "code": code}) }
    if p.Metrics != nil { p.Metrics.Inc("auth_retry_401", map[string]string{"origin": p.TargetOrigin}, 1) }
    p.Resolver.InvalidateAccess(p.TokenKey)
    // conservative sleep to avoid immediate replay
    time.Sleep(10 * time.Millisecond)
    // Rebuild header (gets fresh token)
    key, val, ok, err = BuildAuthHeader(p)
    if err != nil || !ok {
        return code, err
    }
    code, err = attempt(key, val)
    if err == nil && code >= 200 && code < 300 {
        if p.Tracer != nil { p.Tracer.Debug("auth_retry_success", map[string]any{"origin": p.TargetOrigin}) }
        if p.Metrics != nil { p.Metrics.Inc("auth_retry_success", map[string]string{"origin": p.TargetOrigin}, 1) }
    }
    return code, err
}
