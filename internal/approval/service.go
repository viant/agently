package approval

import "context"

// Service defines the approval service interface (minimal subset used by agently).
type Service interface {
	RequestApproval(ctx context.Context, r *Request) error
	ListPending(ctx context.Context) ([]*Request, error)
	Decide(ctx context.Context, id string, approved bool, reason string) (*Decision, error)
	Queue() Queue[Event]
}
