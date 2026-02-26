package read

import "time"

type InputOption func(*Input)

func WithConversationID(id string) InputOption {
	return func(in *Input) { in.ConversationID = id; ensureHas(&in.Has); in.Has.ConversationID = true }
}
func WithTurnID(id string) InputOption {
	return func(in *Input) { in.TurnID = id; ensureHas(&in.Has); in.Has.TurnID = true }
}
func WithMessageID(id string) InputOption {
	return func(in *Input) { in.MessageID = id; ensureHas(&in.Has); in.Has.MessageID = true }
}
func WithID(id string) InputOption {
	return func(in *Input) { in.ID = id; ensureHas(&in.Has); in.Has.ID = true }
}
func WithProvider(provider string) InputOption {
	return func(in *Input) { in.Provider = provider; ensureHas(&in.Has); in.Has.Provider = true }
}
func WithStatus(status string) InputOption {
	return func(in *Input) { in.Status = status; ensureHas(&in.Has); in.Has.Status = true }
}
func WithSince(ts time.Time) InputOption {
	return func(in *Input) { t := ts; in.Since = &t; ensureHas(&in.Has); in.Has.Since = true }
}

func ensureHas(h **Has) {
	if *h == nil {
		*h = &Has{}
	}
}
