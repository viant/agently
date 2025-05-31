package llm

import "context"

type Finder interface {
	Find(ctx context.Context, id string) (Model, error)
}
