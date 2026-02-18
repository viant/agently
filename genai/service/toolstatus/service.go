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
	debugf("status start parent_convo=%q parent_turn=%q tool=%q role=%q actor=%q mode=%q", strings.TrimSpace(parent.ConversationID), strings.TrimSpace(parent.TurnID), strings.TrimSpace(toolName), strings.TrimSpace(role), strings.TrimSpace(actor), strings.TrimSpace(mode))
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
		errorf("status start error parent_convo=%q tool=%q err=%v", strings.TrimSpace(parent.ConversationID), strings.TrimSpace(toolName), err)
		return "", fmt.Errorf("status: start failed: %w", err)
	}
	debugf("status start ok parent_convo=%q tool=%q message_id=%q", strings.TrimSpace(parent.ConversationID), strings.TrimSpace(toolName), strings.TrimSpace(m.Id))
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
	debugf("status update parent_convo=%q parent_turn=%q message_id=%q content_len=%d content_head=%q content_tail=%q", strings.TrimSpace(parent.ConversationID), strings.TrimSpace(parent.TurnID), strings.TrimSpace(messageID), len(content), headString(content, 512), tailString(content, 512))
	mu := apiconv.NewMessage()
	mu.SetId(messageID)
	mu.SetConversationID(parent.ConversationID)
	mu.SetTurnID(parent.TurnID)
	mu.SetContent(content)
	mu.SetInterim(1)
	if err := s.conv.PatchMessage(ctx, mu); err != nil {
		errorf("status update error parent_convo=%q message_id=%q err=%v", strings.TrimSpace(parent.ConversationID), strings.TrimSpace(messageID), err)
		return fmt.Errorf("status: update failed: %w", err)
	}
	debugf("status update ok parent_convo=%q message_id=%q", strings.TrimSpace(parent.ConversationID), strings.TrimSpace(messageID))
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
	debugf("status finalize parent_convo=%q parent_turn=%q message_id=%q status=%q preview_len=%d preview_head=%q preview_tail=%q", strings.TrimSpace(parent.ConversationID), strings.TrimSpace(parent.TurnID), strings.TrimSpace(messageID), strings.TrimSpace(status), len(preview), headString(preview, 512), tailString(preview, 512))
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
		errorf("status finalize error parent_convo=%q message_id=%q err=%v", strings.TrimSpace(parent.ConversationID), strings.TrimSpace(messageID), err)
		return fmt.Errorf("status: finalize failed: %w", err)
	}
	debugf("status finalize ok parent_convo=%q message_id=%q status=%q", strings.TrimSpace(parent.ConversationID), strings.TrimSpace(messageID), strings.TrimSpace(status))
	return nil
}
