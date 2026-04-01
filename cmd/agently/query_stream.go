package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	coreplan "github.com/viant/agently-core/protocol/agent/plan"
	streamingrt "github.com/viant/agently-core/runtime/streaming"
	"github.com/viant/agently-core/sdk"
	agentsvc "github.com/viant/agently-core/service/agent"
)

func (c *ChatCmd) executeQuery(ctx context.Context, client *sdk.HTTPClient, input *agentsvc.QueryInput, defaultPayload map[string]interface{}, seedPayload *map[string]interface{}) (*agentsvc.QueryOutput, bool, error) {
	if err := ensureConversation(ctx, client, input, strings.TrimSpace(input.Query)); err != nil {
		return nil, false, err
	}
	inlineElicitation := len(defaultPayload) > 0 || stdinIsTTY()
	if inlineElicitation {
		input.ElicitationMode = ""
	} else {
		input.ElicitationMode = "deferred"
	}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	streamer, err := startChatStream(streamCtx, client, strings.TrimSpace(input.ConversationID))
	if err != nil {
		return nil, false, err
	}
	defer streamer.Close()

	startedAt := time.Now().UTC()
	resolverCtx, stopResolver := context.WithCancel(ctx)
	defer stopResolver()
	resolverErr := make(chan error, 1)
	if inlineElicitation {
		go watchPendingElicitations(resolverCtx, client, strings.TrimSpace(input.ConversationID), defaultPayload, seedPayload, resolverErr)
	}
	out, err := client.Query(ctx, input)
	if err != nil {
		return nil, false, err
	}
	select {
	case err := <-resolverErr:
		if err != nil {
			return nil, false, err
		}
	default:
	}
	stopResolver()
	if out == nil {
		return nil, false, fmt.Errorf("query returned no response")
	}
	if strings.TrimSpace(out.ConversationID) != "" {
		input.ConversationID = strings.TrimSpace(out.ConversationID)
	}
	if strings.TrimSpace(out.Content) != "" {
		streamer.Close()
		return out, streamer.Flush(out.Content), nil
	}
	elicitation := out.Elicitation
	if elicitation == nil && out.Plan != nil {
		elicitation = out.Plan.Elicitation
	}
	if elicitation != nil && !inlineElicitation {
		streamer.Close()
		return nil, false, fmt.Errorf("elicitation required; run interactively or provide --elicitation-default")
	}
	content, err := waitForAssistantContent(ctx, client, streamer, strings.TrimSpace(out.ConversationID), startedAt, defaultPayload, seedPayload)
	if err != nil {
		return nil, false, err
	}
	out.Content = content
	streamer.Close()
	return out, streamer.Flush(out.Content), nil
}

type chatStreamer struct {
	sub     streamingrt.Subscription
	done    chan struct{}
	tracker *sdk.ConversationStreamTracker
	content strings.Builder
	printed bool
}

func startChatStream(ctx context.Context, client *sdk.HTTPClient, conversationID string) (*chatStreamer, error) {
	sub, err := client.StreamEvents(ctx, &sdk.StreamEventsInput{ConversationID: conversationID})
	if err != nil {
		return nil, fmt.Errorf("stream events: %w", err)
	}
	streamer := &chatStreamer{
		sub:     sub,
		done:    make(chan struct{}),
		tracker: sdk.NewConversationStreamTracker(conversationID),
	}
	go streamer.consume()
	return streamer, nil
}

func (s *chatStreamer) consume() {
	defer close(s.done)
	if s == nil || s.sub == nil {
		return
	}
	for event := range s.sub.C() {
		if event == nil {
			continue
		}
		if s.tracker != nil {
			s.tracker.ApplyEvent(event)
		}
		switch event.Type {
		case streamingrt.EventTypeTextDelta:
			if event.Content == "" {
				continue
			}
			fmt.Print(event.Content)
			s.content.WriteString(event.Content)
			s.printed = true
		case streamingrt.EventTypeError:
			if strings.TrimSpace(event.Error) != "" {
				fmt.Fprintf(os.Stderr, "[stream-error] %s\n", strings.TrimSpace(event.Error))
			}
		}
	}
}

func (s *chatStreamer) Flush(final string) bool {
	if s == nil {
		return false
	}
	streamed := s.content.String()
	final = strings.TrimSpace(final)
	if final == "" {
		if s.printed {
			fmt.Print("\n")
		}
		return s.printed
	}
	if streamed == "" {
		fmt.Print(final)
		fmt.Print("\n")
		s.printed = true
		return true
	}
	if strings.HasPrefix(final, streamed) {
		if remainder := final[len(streamed):]; remainder != "" {
			fmt.Print(remainder)
		}
		fmt.Print("\n")
		return true
	}
	if strings.TrimSpace(streamed) == final || normalizeCLIContent(streamed) == normalizeCLIContent(final) {
		fmt.Print("\n")
		return true
	}
	fmt.Print("\n")
	fmt.Print(final)
	fmt.Print("\n")
	return true
}

func (s *chatStreamer) Close() {
	if s == nil {
		return
	}
	if s.sub != nil {
		_ = s.sub.Close()
	}
	if s.done != nil {
		select {
		case <-s.done:
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func waitForAssistantContent(ctx context.Context, client *sdk.HTTPClient, streamer *chatStreamer, conversationID string, startedAt time.Time, defaultPayload map[string]interface{}, seedPayload *map[string]interface{}) (string, error) {
	_ = startedAt
	if strings.TrimSpace(conversationID) == "" {
		return "", nil
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			if streamer != nil && streamer.tracker != nil {
				if content, ok, err := latestAssistantContentFromState(streamer.tracker.State()); err != nil {
					return "", err
				} else if ok {
					return content, nil
				}
			}
			if handled, err := handlePendingElicitation(ctx, client, conversationID, defaultPayload, seedPayload); err != nil {
				return "", err
			} else if handled {
				continue
			}
			transcript, err := client.GetTranscript(ctx, &sdk.GetTranscriptInput{
				ConversationID:    conversationID,
				IncludeModelCalls: false,
				IncludeToolCalls:  false,
			})
			if err != nil {
				return "", err
			}
			if transcript == nil || len(transcript.Turns) == 0 {
				continue
			}
			turn := transcript.Turns[len(transcript.Turns)-1]
			if turn == nil {
				continue
			}
			status := strings.ToLower(string(turn.Status))
			if status == "failed" {
				return "", fmt.Errorf("turn failed")
			}
			if status == "canceled" {
				return "", fmt.Errorf("turn canceled")
			}
			if turn.Assistant != nil && turn.Assistant.Final != nil {
				if content := strings.TrimSpace(turn.Assistant.Final.Content); content != "" {
					return content, nil
				}
			}
		}
	}
}

func latestAssistantContentFromState(state *sdk.ConversationState) (string, bool, error) {
	if state == nil || len(state.Turns) == 0 {
		return "", false, nil
	}
	turn := state.Turns[len(state.Turns)-1]
	if turn == nil {
		return "", false, nil
	}
	switch strings.ToLower(strings.TrimSpace(string(turn.Status))) {
	case "failed":
		return "", false, fmt.Errorf("turn failed")
	case "canceled":
		return "", false, fmt.Errorf("turn canceled")
	}
	if turn.Assistant != nil && turn.Assistant.Final != nil {
		if content := strings.TrimSpace(turn.Assistant.Final.Content); content != "" {
			return content, true, nil
		}
	}
	return "", false, nil
}

func handlePendingElicitation(ctx context.Context, client *sdk.HTTPClient, conversationID string, defaultPayload map[string]interface{}, seedPayload *map[string]interface{}) (bool, error) {
	rows, err := client.ListPendingElicitations(ctx, &sdk.ListPendingElicitationsInput{ConversationID: conversationID})
	if err != nil {
		return false, err
	}
	var pending *sdk.PendingElicitation
	for _, row := range rows {
		if row == nil || strings.TrimSpace(row.ElicitationID) == "" {
			continue
		}
		if pending == nil || row.CreatedAt.After(pending.CreatedAt) {
			pending = row
		}
	}
	if pending == nil {
		return false, nil
	}
	req := plannedElicitationFromPending(pending)
	if req == nil {
		return false, nil
	}
	if err := resolvePlannedElicitation(ctx, client, conversationID, req, defaultPayload, seedPayload); err != nil {
		return false, err
	}
	return true, nil
}

func watchPendingElicitations(ctx context.Context, client *sdk.HTTPClient, conversationID string, defaultPayload map[string]interface{}, seedPayload *map[string]interface{}, errs chan<- error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	resolved := map[string]struct{}{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rows, err := client.ListPendingElicitations(ctx, &sdk.ListPendingElicitationsInput{ConversationID: conversationID})
			if err != nil {
				select {
				case errs <- err:
				default:
				}
				return
			}
			var pending *sdk.PendingElicitation
			for _, row := range rows {
				if row == nil || strings.TrimSpace(row.ElicitationID) == "" {
					continue
				}
				elicitationID := strings.TrimSpace(row.ElicitationID)
				if _, ok := resolved[elicitationID]; ok {
					continue
				}
				if pending == nil || row.CreatedAt.After(pending.CreatedAt) {
					pending = row
				}
			}
			if pending == nil {
				continue
			}
			req := plannedElicitationFromPending(pending)
			if req == nil {
				continue
			}
			resolved[strings.TrimSpace(req.ElicitationId)] = struct{}{}
			if err := resolvePlannedElicitation(ctx, client, conversationID, req, defaultPayload, seedPayload); err != nil {
				select {
				case errs <- err:
				default:
				}
				return
			}
		}
	}
}

func plannedElicitationFromPending(input *sdk.PendingElicitation) *coreplan.Elicitation {
	if input == nil {
		return nil
	}
	req := &coreplan.Elicitation{}
	if len(input.Elicitation) > 0 {
		if data, err := json.Marshal(input.Elicitation); err == nil {
			_ = json.Unmarshal(data, req)
		}
	}
	if strings.TrimSpace(req.Message) == "" {
		req.Message = strings.TrimSpace(input.Content)
	}
	if strings.TrimSpace(req.ElicitationId) == "" {
		req.ElicitationId = strings.TrimSpace(input.ElicitationID)
	}
	if strings.TrimSpace(req.RequestedSchema.Type) == "" && len(req.RequestedSchema.Properties) == 0 {
		return nil
	}
	if strings.TrimSpace(req.RequestedSchema.Type) == "" {
		req.RequestedSchema.Type = "object"
	}
	return req
}

func resolvePlannedElicitation(ctx context.Context, client *sdk.HTTPClient, conversationID string, req *coreplan.Elicitation, defaultPayload map[string]interface{}, seedPayload *map[string]interface{}) error {
	if req == nil || strings.TrimSpace(req.ElicitationId) == "" {
		return nil
	}
	if seedPayload != nil {
		applyElicitationDefaults(req, *seedPayload)
	}
	if len(defaultPayload) > 0 {
		return client.ResolveElicitation(ctx, &sdk.ResolveElicitationInput{
			ConversationID: conversationID,
			ElicitationID:  req.ElicitationId,
			Action:         "accept",
			Payload:        defaultPayload,
		})
	}
	if !stdinIsTTY() {
		return fmt.Errorf("elicitation required; run interactively or provide --elicitation-default")
	}
	result, err := awaitCoreElicitation(ctx, req)
	if err != nil || result == nil {
		return err
	}
	switch result.Action {
	case coreplan.ElicitResultActionAccept:
		if seedPayload != nil {
			mergePayload(seedPayload, result.Payload)
		}
		return client.ResolveElicitation(ctx, &sdk.ResolveElicitationInput{
			ConversationID: conversationID,
			ElicitationID:  req.ElicitationId,
			Action:         "accept",
			Payload:        result.Payload,
		})
	case coreplan.ElicitResultActionDecline:
		return client.ResolveElicitation(ctx, &sdk.ResolveElicitationInput{
			ConversationID: conversationID,
			ElicitationID:  req.ElicitationId,
			Action:         "decline",
		})
	default:
		return nil
	}
}

func applyElicitationDefaults(req *coreplan.Elicitation, seed map[string]interface{}) {
	if req == nil || len(seed) == 0 {
		return
	}
	for key, value := range seed {
		property, ok := req.RequestedSchema.Properties[key]
		if !ok {
			continue
		}
		asMap, ok := property.(map[string]interface{})
		if !ok {
			continue
		}
		if _, exists := asMap["default"]; !exists {
			asMap["default"] = value
		}
	}
}

func mergePayload(dst *map[string]interface{}, src map[string]interface{}) {
	if dst == nil || len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = map[string]interface{}{}
	}
	for key, value := range src {
		(*dst)[key] = value
	}
}

func buildQueryContext(base map[string]interface{}, defaultPayload, seedPayload map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for key, value := range base {
		out[key] = value
	}
	for key, value := range defaultPayload {
		out[key] = value
	}
	for key, value := range seedPayload {
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stdinIsTTY() bool {
	info, err := os.Stdin.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}
