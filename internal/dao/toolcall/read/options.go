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
func WithOpID(op string) InputOption {
	return func(in *Input) { in.OpID = op; ensureHas(&in.Has); in.Has.OpID = true }
}
func WithToolName(name string) InputOption {
	return func(in *Input) { in.ToolName = name; ensureHas(&in.Has); in.Has.ToolName = true }
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
