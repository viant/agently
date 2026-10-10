package agently

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/viant/agently-core/protocol/agui"
	"github.com/viant/agently-core/sdk"
	agentsvc "github.com/viant/agently-core/service/agent"
)

const maxCLIAGUIReattachments = 3

// Reattach observation without repeating chat admission or already printed text.
func collectCLIAGUI(ctx context.Context, client *sdk.HTTPClient, conversationID string, request agui.RunAgentInput, options *sdk.AGUIRunOptions, printed *bool) (*sdk.AGUIRunResult, error) {
	stream, err := client.RunAGUI(ctx, &request, options)
	seen := map[string]bool{}
	texts := map[string]*strings.Builder{}
	order := []string{}
	nativeTurnID := ""
	for attempt := 0; ; attempt++ {
		if err == nil {
			var result *sdk.AGUIRunResult
			result, err = sdk.CollectAGUI(stream, conversationID, func(event sdk.AGUIEvent) error {
				if event.ID != "" && seen[event.ID] {
					return nil
				}
				var text struct {
					Type, ThreadID, RunID, MessageID, Delta string
					SubagentRunID                           *string `json:"subagentRunId"`
					Metadata                                struct {
						Agently struct{ IdentityVersion, NativeTurnID string } `json:"agently"`
					} `json:"metadata"`
				}
				if err := event.Decode(&text); err != nil {
					return err
				}
				if event.ID != "" {
					seen[event.ID] = true
				}
				root := text.SubagentRunID == nil && (text.RunID == "" || text.RunID == request.RunID) && (text.ThreadID == "" || text.ThreadID == request.ThreadID)
				if root && text.Type == "RUN_STARTED" && text.Metadata.Agently.IdentityVersion == "1" {
					nativeTurnID = text.Metadata.Agently.NativeTurnID
				}
				if root && (text.Type == "TEXT_MESSAGE_CONTENT" || text.Type == "TEXT_MESSAGE_CHUNK") && text.Delta != "" {
					if texts[text.MessageID] == nil {
						texts[text.MessageID] = &strings.Builder{}
						order = append(order, text.MessageID)
					}
					texts[text.MessageID].WriteString(text.Delta)
					fmt.Fprint(os.Stdout, text.Delta)
					*printed = true
				}
				return nil
			})
			_ = stream.Close()
			if err == nil {
				if result.TurnID == "" {
					result.TurnID = nativeTurnID
				}
				parts := make([]string, 0, len(order))
				for _, id := range order {
					parts = append(parts, texts[id].String())
				}
				if len(parts) > 0 {
					result.Content = strings.Join(parts, "\n")
				}
				return result, nil
			}
		}
		var interrupted *sdk.AGUIObservationError
		if !errors.As(err, &interrupted) {
			return nil, err
		}
		if ctx.Err() != nil || attempt >= maxCLIAGUIReattachments || !isCLIAGUITransportInterruption(interrupted.Cause) {
			return nil, fmt.Errorf("%w; thread=%q run=%q after-event-id=%q (recover with --conv %s --attach-run %s)", err, interrupted.ThreadID, interrupted.RunID, interrupted.LastEventID, conversationID, interrupted.RunID)
		}
		fmt.Fprintf(os.Stderr, "[ag-ui-reattach] run=%s attempt=%d\n", interrupted.RunID, attempt+1)
		stream, err = client.AttachAGUI(ctx, interrupted.ThreadID, interrupted.RunID, interrupted.LastEventID)
	}
}

func isCLIAGUITransportInterruption(err error) bool {
	var network net.Error
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.As(err, &network)
}

func (c *ChatCmd) attachExistingQuery(ctx context.Context, client *sdk.HTTPClient, defaults map[string]interface{}, seed *map[string]interface{}) (*agentsvc.QueryOutput, bool, error) {
	state, err := client.GetTranscript(ctx, &sdk.GetTranscriptInput{ConversationID: c.ConvID})
	if err != nil {
		return nil, false, err
	}
	threadID := c.ConvID
	if state.AguiThreadID != nil {
		threadID = *state.AguiThreadID
		if threadID == "" {
			return nil, false, fmt.Errorf("empty AG-UI thread mapping")
		}
	}
	forwarded, _ := json.Marshal(map[string]any{"agently": map[string]any{"version": "1", "operation": "run.attach", "requestId": c.AttachRun, "target": map[string]string{"threadId": threadID}, "payload": map[string]any{}}})
	request := agui.RunAgentInput{ThreadID: threadID, RunID: c.AttachRun, Messages: []agui.Message{}, ForwardedProps: forwarded}
	return c.observeAGUIQuery(ctx, client, c.ConvID, request, &sdk.AGUIRunOptions{AfterEventID: c.AfterEventID}, defaults, seed)
}
