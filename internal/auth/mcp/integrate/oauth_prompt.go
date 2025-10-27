package integrate

import (
	"context"
	"time"
)

// OAuthMeta carries context about the OAuth interaction that the prompt may show to the user.
type OAuthMeta struct {
	ProviderName   string
	Scopes         []string
	ConversationID string
	Audience       string
	Origin         string
	Timeout        time.Duration
}

// OAuthPrompt presents an out‑of‑band (OOB) authorization URL to the user.
// Implementations should return nil when the URL has been presented (e.g. MCP
// OOB elicitation recorded, or OS browser opened). The transport will still
// treat the auth as pending and the caller should retry after completion.
type OAuthPrompt interface {
	// PromptOOB presents the authorizationURL to the user (oob mode).
	// Do not block for completion; simply present the URL and return
	// nil, or return an error if the prompt could not be displayed.
	PromptOOB(ctx context.Context, authorizationURL string, meta OAuthMeta) error
}
