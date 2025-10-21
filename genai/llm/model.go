package llm

import (
	"context"
	"time"
)

// Model represents a Large Language Model client capable of generating content.
type Model interface {
	Generate(ctx context.Context, request *GenerateRequest) (*GenerateResponse, error)
	Implements(feature string) bool
}

// BackoffAdvisor is an optional interface that a Model may implement to advise
// the caller about retry/backoff policy for specific provider/model errors.
//
// When implemented, the core retry loop may consult AdviseBackoff to decide
// whether to retry on a given error and how long to wait before the next
// attempt. The returned duration is a suggested delay; if retry is false the
// advice is ignored.
type BackoffAdvisor interface {
	// AdviseBackoff returns a suggested backoff duration and whether the error is
	// retryable for the given attempt index (0-based). Returning retry=false means
	// the error should not be retried by this advisor.
	AdviseBackoff(err error, attempt int) (delay time.Duration, retry bool)
}
