package memory

import (
	"context"
	"strings"
)

// VisibilityPolicy filters or transforms a message slice according to the
// conversation visibility setting as defined by ConversationMeta.Visibility.
//
//	full    – include all messages unchanged.
//	none    – hide every message (empty slice).
//	summary – collapse the entire slice into a single synthetic summary
//	          message produced by the supplied Summarizer function.
//
// When Mode == "summary" the Summarizer callback must not be nil. All other
// modes ignore the field.
type VisibilityPolicy struct {
	Mode       string
	Summarizer SummarizerFunc
}

// NewVisibilityPolicy returns a VisibilityPolicy initialised with the supplied
// mode and summarizer. The mode string is compared case-insensitively. Callers
// may pass nil summarizer when mode != "summary".
func NewVisibilityPolicy(mode string, summarizer SummarizerFunc) *VisibilityPolicy {
	return &VisibilityPolicy{Mode: strings.ToLower(strings.TrimSpace(mode)), Summarizer: summarizer}
}

// Apply fulfils the Policy interface.
// It either returns the original slice, an empty slice, or a single-element
// slice containing the summary message.
func (p *VisibilityPolicy) Apply(ctx context.Context, messages []Message) ([]Message, error) {
	switch p.Mode {
	case "", "full":
		// Default: nothing filtered.
		return messages, nil

	case "none":
		return []Message{}, nil

	case "summary":
		if p.Summarizer == nil {
			return nil, nil // treat as no-op when summarizer is absent
		}
		msg, err := p.Summarizer(ctx, messages)
		if err != nil {
			return nil, err
		}
		return []Message{msg}, nil
	default:
		// Unknown mode – behave like full to avoid data loss.
		return messages, nil
	}
}
