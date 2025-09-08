package memory

import (
	"context"
)

// History defines behaviour for conversation history storage.
type History interface {
	AddMessage(ctx context.Context, msg Message) error
	GetMessages(ctx context.Context, convID string) ([]Message, error)
	Retrieve(ctx context.Context, convID string, policy Policy) ([]Message, error)

	// UpdateMessage finds message by id within convID and applies mutator.
	UpdateMessage(ctx context.Context, messageId string, mutate func(*Message)) error

	// LookupMessage searches all conversations and returns the first message
	LookupMessage(ctx context.Context, messageID string) (*Message, error)

	LatestMessage(ctx context.Context) (msg *Message, err error)

	// --- Conversation meta management ------------------------------
	CreateMeta(ctx context.Context, id, parentID, title, visibility string)
	Meta(ctx context.Context, id string) (*ConversationMeta, bool)
	Children(ctx context.Context, parentID string) ([]ConversationMeta, bool)
}

// NoopHistory is a stub implementation that performs no persistence.
type NoopHistory struct{}

func (*NoopHistory) AddMessage(ctx context.Context, msg Message) error { return nil }
func (*NoopHistory) GetMessages(ctx context.Context, convID string) ([]Message, error) {
	return []Message{}, nil
}
func (*NoopHistory) Retrieve(ctx context.Context, convID string, policy Policy) ([]Message, error) {
	return []Message{}, nil
}
func (*NoopHistory) UpdateMessage(ctx context.Context, messageId string, mutate func(*Message)) error {
	return nil
}
func (*NoopHistory) LookupMessage(ctx context.Context, messageID string) (*Message, error) {
	return nil, nil
}
func (*NoopHistory) LatestMessage(ctx context.Context) (*Message, error)                    { return nil, nil }
func (*NoopHistory) CreateMeta(ctx context.Context, id, parentID, title, visibility string) {}
func (*NoopHistory) Meta(ctx context.Context, id string) (*ConversationMeta, bool)          { return nil, false }
func (*NoopHistory) Children(ctx context.Context, parentID string) ([]ConversationMeta, bool) {
	return nil, false
}
