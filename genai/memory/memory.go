package memory

import (
	"context"
	"time"
)

type messageID string

var MessageIDKey = messageID("messageID")

// Message represents a conversation message for memory storage.
type Message struct {
	ID       string  `json:"id"`
	Role     string  `json:"role"`
	Content  string  `json:"content"`
	ToolName *string `yaml:"toolName"` // Optional tool name, can be nil
	// When messages include file uploads the Attachments slice describes each
	// uploaded asset (or generated/downloadable asset on assistant side).
	Attachments Attachments `json:"attachments,omitempty" yaml:"attachments,omitempty" sqlx:"-"`
	CreatedAt   time.Time   `json:"createdAt" yaml:"createdAt"`
}

type Attachments []Attachment

// Attachment describes a file linked to the message.
type Attachment struct {
	Name string `json:"name" yaml:"name"`
	URL  string `json:"url"  yaml:"url"`
	Size int64  `json:"size" yaml:"size"` // bytes
	// MediaType allows UI to decide how to display or download.
	MediaType string `json:"mediaType,omitempty" yaml:"mediaType,omitempty"`
}

// EmbedFunc defines a function that creates embeddings for given texts.
// It should return one embedding per input text.
type EmbedFunc func(ctx context.Context, texts []string) ([][]float32, error)
