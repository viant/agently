package read

import "time"

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

func WithRoles(roles ...string) InputOption {
	return func(in *Input) { in.Roles = roles; ensureHas(&in.Has); in.Has.Roles = true }
}

func WithType(typ string) InputOption {
	return func(in *Input) { in.Type = typ; ensureHas(&in.Has); in.Has.Type = true }
}

func WithInterim(values ...int) InputOption {
	return func(in *Input) { in.Interim = values; ensureHas(&in.Has); in.Has.Interim = true }
}

func WithElicitationID(id string) InputOption {
	return func(in *Input) { in.ElicitationID = id; ensureHas(&in.Has); in.Has.ElicitationID = true }
}

func WithTurnID(id string) InputOption {
	return func(in *Input) { in.TurnID = id; ensureHas(&in.Has); in.Has.TurnID = true }
}

func WithInput(src Input) InputOption { return func(in *Input) { *in = src } }

func ensureHas(h **Has) {
	if *h == nil {
		*h = &Has{}
	}
}

// WithSince filters messages created at or after the provided timestamp.
func WithSince(ts time.Time) InputOption {
	return func(in *Input) { t := ts; in.Since = &t; ensureHas(&in.Has); in.Has.Since = true }
}

// WithSinceID requests client-side slicing of the transcript starting from the
// given message ID (inclusive). This is a non-DB filter applied after rows are
// fetched. It preserves wire-compatibility and default behavior because it is
// only effective when provided.
func WithSinceID(id string) InputOption {
	return func(in *Input) { in.SinceID = id; ensureHas(&in.Has); in.Has.SinceID = true }
}

// WithElicitationInline asks the service implementation to backfill
// MessageView.ElicitationJSON (when ElicitationID is present) with the inline
// payload JSON when available, or a preview if configured by persistence.
// This flag has no DB effect; it enables a secondary lookup step.
func WithElicitationInline() InputOption {
	return func(in *Input) { in.ElicitInline = true }
}

// WithIncludeOutcomes instructs the service layer to include aggregated
// execution outcomes (tool/model calls grouped by root message) on the
// transcript. This is a non-DB post-processing step.
func WithIncludeOutcomes() InputOption {
	return func(in *Input) { in.IncludeOutcomes = true }
}

// WithFlattenExecutions asks the service to convert aggregated executions into
// synthetic tool messages appended to the transcript, instead of attaching
// Executions to the root message. Useful to keep v1 response shape.
func WithFlattenExecutions() InputOption {
	return func(in *Input) { in.FlattenExecutions = true }
}

// WithParentRoot filters the transcript to a specific root message and its
// children (i.e. tool messages). Non-DB filter.
func WithParentRoot(id string) InputOption {
	return func(in *Input) { in.ParentRoot = id }
}
