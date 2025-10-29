package message

import (
	"context"
	"fmt"
	"strings"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
)

type RemoveTuple struct {
	Summary    string   `json:"summary" description:"summary to associate with removed messages"`
	MessageIds []string `json:"messageIds"`
	Role       string   `json:"role,omitempty"`
}

type RemoveInput struct {
	Tuples []RemoveTuple `json:"tuples"`
}

type RemoveOutput struct {
	CreatedSummaryMessageIds []string `json:"createdSummaryMessageIds"`
	ArchivedMessages         int      `json:"archivedMessages"`
}

// remove accepts tuples of {summary, messageIds} and for each tuple creates a new
// assistant summary message, then flags the listed messages as archived (soft-removed).
// This operates within the current conversation turn supplied via context.
func (s *Service) remove(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*RemoveInput)
	if !ok {
		return fmt.Errorf("invalid input")
	}
	output, ok := out.(*RemoveOutput)
	if !ok {
		return fmt.Errorf("invalid output")
	}
	if s == nil || s.conv == nil {
		return fmt.Errorf("conversation client not initialised")
	}
	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok || strings.TrimSpace(turn.ConversationID) == "" {
		return fmt.Errorf("missing conversation context")
	}
	// Identify and protect the last user message (do not remove)
	lastUserID := ""
	if conv, err := s.conv.GetConversation(ctx, turn.ConversationID, apiconv.WithIncludeToolCall(true)); err == nil && conv != nil {
		tr := conv.GetTranscript()
		for i := len(tr) - 1; i >= 0 && lastUserID == ""; i-- {
			t := tr[i]
			if t == nil || len(t.Message) == 0 {
				continue
			}
			for j := len(t.Message) - 1; j >= 0; j-- {
				m := t.Message[j]
				if m == nil || m.Interim != 0 || m.Content == nil || strings.TrimSpace(*m.Content) == "" {
					continue
				}
				if strings.EqualFold(strings.TrimSpace(m.Role), "user") {
					lastUserID = m.Id
					break
				}
			}
		}
	}
	var created []string
	archived := 0
	for _, tup := range input.Tuples {
		sum := strings.TrimSpace(tup.Summary)
		if sum != "" {
			role := strings.TrimSpace(tup.Role)
			if role == "" {
				role = "assistant"
			}
			if mm, err := apiconv.AddMessage(ctx, s.conv, &turn,
				apiconv.WithRole(role),
				apiconv.WithType("text"),
				apiconv.WithStatus("summary"),
				apiconv.WithContent(sum),
			); err == nil && mm != nil {
				created = append(created, mm.Id)
			} else if err != nil {
				return err
			}
		}
		// Truncate summary for per-message field if needed
		sumForField := sum

		sumForField = runeTruncate(sumForField, 500)

		for _, id := range tup.MessageIds {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if lastUserID != "" && id == lastUserID {
				continue
			}
			mm := apiconv.NewMessage()
			mm.SetId(id)
			if sumForField != "" {
				mm.SetSummary(sumForField)
			}
			mm.SetArchived(1)
			if err := s.conv.PatchMessage(ctx, mm); err != nil {
				return err
			}
			archived++
		}
	}
	output.CreatedSummaryMessageIds = created
	output.ArchivedMessages = archived
	return nil
}

// runeTruncate safely truncates a UTF-8 string to at most n runes without
// splitting a multi-byte character. When n <= 0 it returns an empty string.
func runeTruncate(s string, n int) string {
	if n <= 0 || s == "" {
		return ""
	}
	i := 0
	for idx := range s { // idx iterates over valid rune boundaries
		if i == n {
			return s[:idx]
		}
		i++
	}
	return s
}
