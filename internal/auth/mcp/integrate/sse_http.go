package integrate

import (
    "context"
    "io"
    "net/http"
)

// SSEOpenHTTP opens an SSE stream with the provided http.Client (whose Transport should be
// an auth RoundTripper). It sets the Authorization header and standard SSE headers.
func SSEOpenHTTP(ctx context.Context, hc *http.Client, url string, token string, headers map[string]string) (io.ReadCloser, *http.Response, error) {
    req, err := http.NewRequestWithContext(ContextWithAuthToken(ctx, token), http.MethodGet, url, nil)
    if err != nil {
        return nil, nil, err
    }
    // Ensure SSE headers
    req.Header.Set("Accept", "text/event-stream")
    req.Header.Set("Cache-Control", "no-cache")
    req.Header.Set("Connection", "keep-alive")
    if token != "" {
        req.Header.Set("Authorization", "Bearer "+token)
    }
    for k, v := range headers {
        req.Header.Set(k, v)
    }
    resp, err := hc.Do(req)
    if err != nil {
        return nil, resp, err
    }
    if resp.StatusCode >= 400 {
        // let caller handle 401/419 via OpenSSEWithAuth retry
        return resp.Body, resp, nil
    }
    return resp.Body, resp, nil
}

