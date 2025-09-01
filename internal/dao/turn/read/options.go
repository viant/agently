package read

import "time"

// InputOption mutates Input for query composition.
type InputOption func(*Input)

func WithConversationID(id string) InputOption {
	return func(in *Input) { in.ConversationID = id; ensureHas(&in.Has); in.Has.ConversationID = true }
}

func WithID(id string) InputOption {
	return func(in *Input) { in.Id = id; ensureHas(&in.Has); in.Has.Id = true }
}

func WithIDs(ids ...string) InputOption {
	return func(in *Input) { in.Ids = ids; ensureHas(&in.Has); in.Has.Ids = true }
}

func WithStatus(status string) InputOption {
	return func(in *Input) { in.Status = status; ensureHas(&in.Has); in.Has.Status = true }
}

func WithSince(ts time.Time) InputOption {
	return func(in *Input) { t := ts; in.Since = &t; ensureHas(&in.Has); in.Has.Since = true }
}

func WithInput(src Input) InputOption { return func(in *Input) { *in = src } }

func ensureHas(h **Has) {
	if *h == nil {
		*h = &Has{}
	}
}
