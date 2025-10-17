package repository

import "context"

// CRUD is a simplified interface used by CLI/ws handlers.
type CRUD interface {
	List(ctx context.Context) ([]string, error)
	GetRaw(ctx context.Context, name string) ([]byte, error)
	Add(ctx context.Context, name string, data []byte) error
	Delete(ctx context.Context, name string) error
}
