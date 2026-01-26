package sdk

import (
	"context"
	"net/http"
	"time"
)

// TokenProvider returns a bearer token for the request.
type TokenProvider func(ctx context.Context) (string, error)

// Option customizes the SDK client.
type Option func(c *Client)

// RetryPolicy controls request retry behavior.
type RetryPolicy struct {
	MaxAttempts   int
	Delay         time.Duration
	RetryStatuses map[int]struct{}
	RetryMethods  map[string]struct{}
	RetryOnError  bool
}

// WithHTTPClient supplies a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.http = hc
		}
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if c.http == nil {
			c.http = &http.Client{}
		}
		c.http.Timeout = d
	}
}

// WithTokenProvider supplies a bearer token provider.
func WithTokenProvider(tp TokenProvider) Option {
	return func(c *Client) {
		c.tokenProvider = tp
	}
}

// WithCookieJar sets the cookie jar on the HTTP client.
func WithCookieJar(jar http.CookieJar) Option {
	return func(c *Client) {
		if c.http == nil {
			c.http = &http.Client{}
		}
		c.http.Jar = jar
	}
}

// WithRetryPolicy sets a retry policy for requests.
func WithRetryPolicy(p RetryPolicy) Option {
	return func(c *Client) {
		c.retry = p
	}
}

// WithRequestHook adds a hook executed before every request is sent.
func WithRequestHook(hook func(*http.Request) error) Option {
	return func(c *Client) {
		c.requestHook = hook
	}
}

// WithResponseHook adds a hook executed after every response is received.
func WithResponseHook(hook func(*http.Response) error) Option {
	return func(c *Client) {
		c.responseHook = hook
	}
}

// WithHeader sets a static header on all requests.
func WithHeader(key, value string) Option {
	return func(c *Client) {
		if c.headers == nil {
			c.headers = map[string]string{}
		}
		c.headers[key] = value
	}
}
