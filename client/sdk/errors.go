package sdk

import "fmt"

// HTTPError wraps non-2xx responses.
type HTTPError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPError) Error() string {
	if e == nil {
		return "http error"
	}
	if e.Body != "" {
		return fmt.Sprintf("request failed: %s (%d): %s", e.Status, e.StatusCode, e.Body)
	}
	return fmt.Sprintf("request failed: %s (%d)", e.Status, e.StatusCode)
}
