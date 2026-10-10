package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/viant/agently-core/sdk"
)

var cliReportFence = regexp.MustCompile("(?s)```forge-report\\s*\\n(.*?)\\n```")

// Render an authored report's committed summary, not inferred chart values or
// executable HTML. The native transcript remains the source for the full result.
func printCLIReportSummary(ctx context.Context, client *sdk.HTTPClient, conversationID, turnID string) {
	if turnID == "" {
		return
	}
	state, err := client.GetTranscript(ctx, &sdk.GetTranscriptInput{ConversationID: conversationID})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[report] Summary unavailable; inspect transcript for conversation %s.\n", conversationID)
		return
	}
	if state.Conversation == nil || len(state.Conversation.Turns) == 0 {
		return
	}
	// Decode only the public final content; never print model-call reasoning.
	var turn struct {
		TurnID    string `json:"turnId"`
		Assistant struct {
			Final struct{ Content string } `json:"final"`
		} `json:"assistant"`
	}
	found := false
	for _, candidate := range state.Conversation.Turns {
		raw, marshalErr := json.Marshal(candidate)
		if marshalErr == nil && json.Unmarshal(raw, &turn) == nil && turn.TurnID == turnID {
			found = true
			break
		}
	}
	if !found {
		return
	}
	for _, summary := range committedCLIReportSummaries(turn.Assistant.Final.Content) {
		fmt.Printf("[report] %s\n%s\n", summary.title, summary.text)
	}
}

type cliReportSummary struct{ title, text string }

func committedCLIReportSummaries(content string) []cliReportSummary {
	pending := map[string]cliReportSummary{}
	var result []cliReportSummary
	for _, fence := range cliReportFence.FindAllStringSubmatch(content, -1) {
		var report struct {
			Scope, ID, Mode, Title string
			Blocks                 []struct{ Kind, Markdown string }
		}
		if json.Unmarshal([]byte(fence[1]), &report) != nil {
			continue
		}
		key := report.Scope + "\x00" + report.ID
		switch report.Mode {
		case "start":
			delete(pending, key)
			for _, block := range report.Blocks {
				if block.Kind == "markdownBlock" && block.Markdown != "" {
					text := []rune(block.Markdown)
					if len(text) > 8192 {
						text = append(text[:8192], []rune("\n[Summary truncated; see transcript for the full report.]")...)
					}
					pending[key] = cliReportSummary{report.Title, string(text)}
					break
				}
			}
		case "commit":
			if summary, ok := pending[key]; ok {
				result = append(result, summary)
				delete(pending, key)
			}
		}
	}
	return result
}
