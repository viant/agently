package read

import "time"

type InputOption func(*Input)

func WithConversationID(id string) InputOption {
	return func(in *Input) { in.ConversationID = id; ensureHas(&in.Has); in.Has.ConversationID = true }
}
func WithMessageID(id string) InputOption {
	return func(in *Input) { in.MessageID = id; ensureHas(&in.Has); in.Has.MessageID = true }
}
func WithMessageIDs(ids ...string) InputOption {
	return func(in *Input) { in.MessageIDs = ids; ensureHas(&in.Has); in.Has.MessageIDs = true }
}
func WithTurnID(id string) InputOption {
	return func(in *Input) { in.TurnID = id; ensureHas(&in.Has); in.Has.TurnID = true }
}
func WithProvider(p string) InputOption {
	return func(in *Input) { in.Provider = p; ensureHas(&in.Has); in.Has.Provider = true }
}
func WithModel(m string) InputOption {
	return func(in *Input) { in.Model = m; ensureHas(&in.Has); in.Has.Model = true }
}
func WithModelKind(k string) InputOption {
	return func(in *Input) { in.ModelKind = k; ensureHas(&in.Has); in.Has.ModelKind = true }
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
