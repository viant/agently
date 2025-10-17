package approval

import "context"

type Message[T any] interface {
	T() *T
	Ack() error
	Nack(error) error
}

type Queue[T any] interface {
	Publish(ctx context.Context, t *T) error
	Consume(ctx context.Context) (Message[T], error)
}
