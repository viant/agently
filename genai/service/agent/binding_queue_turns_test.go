package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/prompt"
	core "github.com/viant/agently/genai/service/core"
	agconv "github.com/viant/agently/pkg/agently/conversation"
)

func TestBuildHistory_SkipsQueuedTurns(t *testing.T) {
	now := time.Now().UTC()

	makeTurn := func(id, status, userContent string, createdAt time.Time) *apiconv.Turn {
		turn := &apiconv.Turn{
			Id:        id,
			Status:    status,
			CreatedAt: createdAt,
		}
		if strings.TrimSpace(userContent) == "" {
			return turn
		}
		msg := &apiconv.Message{
			Id:        id,
			Role:      "user",
			Type:      "text",
			Content:   strPtr(userContent),
			CreatedAt: createdAt,
		}
		turn.Message = []*agconv.MessageView{(*agconv.MessageView)(msg)}
		return turn
	}

	t.Run("skips queued turns not current", func(t *testing.T) {
		svc := &Service{}
		tr := apiconv.Transcript{
			makeTurn("turn-1", "succeeded", "first succeeded", now.Add(-3*time.Second)),
			makeTurn("turn-2", "running", "second running", now.Add(-2*time.Second)),
			makeTurn("turn-3", "queued", "third queued", now.Add(-1*time.Second)),
		}

		hist, err := svc.buildHistory(context.Background(), tr)
		require.NoError(t, err)

		var userMsgs []string
		for _, turn := range hist.Past {
			if turn == nil {
				continue
			}
			for _, m := range turn.Messages {
				if m == nil {
					continue
				}
				if strings.EqualFold(strings.TrimSpace(m.Role), "user") {
					userMsgs = append(userMsgs, strings.TrimSpace(m.Content))
				}
			}
		}
		require.Equal(t, []string{"first succeeded", "second running"}, userMsgs)
	})

	t.Run("keeps queued current turn only", func(t *testing.T) {
		svc := &Service{}
		tr := apiconv.Transcript{
			makeTurn("turn-1", "succeeded", "first succeeded", now.Add(-3*time.Second)),
			makeTurn("turn-2", "queued", "second running", now.Add(-2*time.Second)),
			makeTurn("turn-3", "queued", "third queued", now.Add(-1*time.Second)),
		}

		ctx := memory.WithTurnMeta(context.Background(), memory.TurnMeta{TurnID: "turn-3"})
		hist, err := svc.buildHistory(ctx, tr)
		require.NoError(t, err)

		var userMsgs []string
		for _, turn := range hist.Past {
			if turn == nil {
				continue
			}
			for _, m := range turn.Messages {
				if m == nil {
					continue
				}
				if strings.EqualFold(strings.TrimSpace(m.Role), "user") {
					userMsgs = append(userMsgs, strings.TrimSpace(m.Content))
				}
			}
		}
		require.Equal(t, []string{"first succeeded", "third queued"}, userMsgs)
	})
}

func TestContinuationRequest_QueuedPromptUsesTurnCreatedAt(t *testing.T) {
	now := time.Now().UTC()
	anchorAt := now.Add(-1 * time.Second)
	queuedMsgAt := now.Add(-2 * time.Second) // queued before anchor exists
	execAt := now.Add(100 * time.Millisecond)

	// Current prompt was queued while the previous turn was still running,
	// so its message.CreatedAt predates the previous assistant response.
	const promptText = "third queued"

	turn := &apiconv.Turn{
		Id:        "turn-3",
		Status:    "running",
		CreatedAt: execAt, // updated at execution time by startTurn
		Message: []*agconv.MessageView{(*agconv.MessageView)(&apiconv.Message{
			Id:        "turn-3",
			Role:      "user",
			Type:      "text",
			Content:   strPtr(promptText),
			CreatedAt: queuedMsgAt,
		})},
	}

	svc := &Service{}
	history := &prompt.History{
		Traces:       svc.buildTraces(apiconv.Transcript{turn}),
		LastResponse: &prompt.Trace{ID: "resp-2", At: anchorAt},
	}

	req := &llm.GenerateRequest{Messages: []llm.Message{{Role: llm.RoleUser, Content: promptText}}}
	ctx := memory.WithTurnMeta(context.Background(), memory.TurnMeta{ConversationID: "conv-1", TurnID: "turn-3"})
	cont := (&core.Service{}).BuildContinuationRequest(ctx, req, history)

	require.NotNil(t, cont)
	require.Equal(t, "resp-2", cont.PreviousResponseID)
	require.Len(t, cont.Messages, 1)
	require.Equal(t, llm.RoleUser, cont.Messages[0].Role)
	require.Equal(t, promptText, cont.Messages[0].Content)
}

func TestBuildHistory_SkipsCanceledTurns(t *testing.T) {
	now := time.Now().UTC()

	cancel := "cancel"

	makeTurn := func(id, status, userContent string, msgStatus *string, createdAt time.Time) *apiconv.Turn {
		turn := &apiconv.Turn{
			Id:        id,
			Status:    status,
			CreatedAt: createdAt,
		}
		if strings.TrimSpace(userContent) == "" {
			return turn
		}
		msg := &apiconv.Message{
			Id:        id,
			Role:      "user",
			Type:      "text",
			Content:   strPtr(userContent),
			Status:    msgStatus,
			CreatedAt: createdAt,
		}
		turn.Message = []*agconv.MessageView{(*agconv.MessageView)(msg)}
		return turn
	}

	svc := &Service{}
	tr := apiconv.Transcript{
		makeTurn("turn-1", "succeeded", "prompt-01", nil, now.Add(-4*time.Second)),
		makeTurn("turn-3", "canceled", "prompt-03", &cancel, now.Add(-2*time.Second)),
		makeTurn("turn-4", "running", "prompt-04", nil, now.Add(-1*time.Second)),
	}

	hist, err := svc.buildHistory(context.Background(), tr)
	require.NoError(t, err)

	var userMsgs []string
	for _, turn := range hist.Past {
		if turn == nil {
			continue
		}
		for _, m := range turn.Messages {
			if m == nil {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(m.Role), "user") {
				userMsgs = append(userMsgs, strings.TrimSpace(m.Content))
			}
		}
	}
	require.Equal(t, []string{"prompt-01", "prompt-04"}, userMsgs)
}
