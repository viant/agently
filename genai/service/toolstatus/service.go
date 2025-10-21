package toolstatus

import (
	"context"
	"fmt"
	"strings"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
)

// Service publishes tool-run status messages into the parent conversation turn.
// It supports start/update/finalize lifecycle with minimal, consistent fields.
type Service struct {
	conv apiconv.Client
}

func New(c apiconv.Client) *Service { return &Service{conv: c} }

// Start creates an interim status message under the parent turn and returns its id.
// role defaults to "assistant"; mode defaults to "exec"; actor defaults to "tool".
func (s *Service) Start(ctx context.Context, parent memory.TurnMeta, toolName, role, actor, mode string) (string, error) {
	if s == nil || s.conv == nil {
		return "", fmt.Errorf("status: conversation client not configured")
	}
	if strings.TrimSpace(role) == "" {
		role = "assistant"
	}
	if strings.TrimSpace(actor) == "" {
		actor = "tool"
	}
	if strings.TrimSpace(mode) == "" {
		mode = "exec"
	}
	m, err := apiconv.AddMessage(ctx, s.conv, &parent,
		apiconv.WithRole(role),
		apiconv.WithInterim(1),
		apiconv.WithContent(""),
		apiconv.WithCreatedByUserID(actor),
		apiconv.WithMode(mode),
		apiconv.WithToolName(toolName),
	)
	if err != nil {
		return "", fmt.Errorf("status: start failed: %w", err)
	}
	return m.Id, nil
}

// Update sets interim content (e.g., progress text) on the status message.
func (s *Service) Update(ctx context.Context, parent memory.TurnMeta, messageID, content string) error {
	if s == nil || s.conv == nil {
		return fmt.Errorf("status: conversation client not configured")
	}
	if strings.TrimSpace(messageID) == "" {
		return fmt.Errorf("status: empty messageID")
	}
	mu := apiconv.NewMessage()
	mu.SetId(messageID)
	mu.SetConversationID(parent.ConversationID)
	mu.SetTurnID(parent.TurnID)
	mu.SetContent(content)
	mu.SetInterim(1)
	if err := s.conv.PatchMessage(ctx, mu); err != nil {
		return fmt.Errorf("status: update failed: %w", err)
	}
	return nil
}

// Finalize clears interim, sets final status, and writes an optional preview content.
// status should be one of running|succeeded|failed|canceled|auth-required.
func (s *Service) Finalize(ctx context.Context, parent memory.TurnMeta, messageID, status, preview string) error {
	if s == nil || s.conv == nil {
		return fmt.Errorf("status: conversation client not configured")
	}
	if strings.TrimSpace(messageID) == "" {
		return fmt.Errorf("status: empty messageID")
	}
	mu := apiconv.NewMessage()
	mu.SetId(messageID)
	mu.SetConversationID(parent.ConversationID)
	mu.SetTurnID(parent.TurnID)
	if strings.TrimSpace(preview) != "" {
		mu.SetContent(preview)
	}
	mu.SetInterim(0)
	if strings.TrimSpace(status) != "" {
		mu.SetStatus(strings.TrimSpace(status))
	}
	if err := s.conv.PatchMessage(ctx, mu); err != nil {
		return fmt.Errorf("status: finalize failed: %w", err)
	}
	return nil
}
