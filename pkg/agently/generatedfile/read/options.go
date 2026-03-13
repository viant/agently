package read

import "time"

type InputOption func(*GeneratedFileInput)

func WithConversationID(id string) InputOption {
	return func(in *GeneratedFileInput) { in.ConversationID = id; ensureHas(&in.Has); in.Has.ConversationID = true }
}
func WithTurnID(id string) InputOption {
	return func(in *GeneratedFileInput) { in.TurnID = id; ensureHas(&in.Has); in.Has.TurnID = true }
}
func WithMessageID(id string) InputOption {
	return func(in *GeneratedFileInput) { in.MessageID = id; ensureHas(&in.Has); in.Has.MessageID = true }
}
func WithID(id string) InputOption {
	return func(in *GeneratedFileInput) { in.ID = id; ensureHas(&in.Has); in.Has.ID = true }
}
func WithProvider(provider string) InputOption {
	return func(in *GeneratedFileInput) { in.Provider = provider; ensureHas(&in.Has); in.Has.Provider = true }
}
func WithStatus(status string) InputOption {
	return func(in *GeneratedFileInput) { in.Status = status; ensureHas(&in.Has); in.Has.Status = true }
}
func WithSince(ts time.Time) InputOption {
	return func(in *GeneratedFileInput) { in.Since = ts; ensureHas(&in.Has); in.Has.Since = true }
}

func ensureHas(h **GeneratedFileInputHas) {
	if *h == nil {
		*h = &GeneratedFileInputHas{}
	}
}
