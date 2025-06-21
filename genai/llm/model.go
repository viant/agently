package llm

import "context"

type Model interface {
	Generate(ctx context.Context, request *GenerateRequest) (*GenerateResponse, error)
	Implements(feature string) bool
}
