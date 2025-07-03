package summary

import (
	"context"
	"strings"
	"text/template"

	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/memory"
)

const defaultPrompt = "Summarize the following conversation in a concise form:\n${conversation}"

// Build returns a function compatible with memory.NewSummaryPolicy.
func Build(llmCore *core.Service, model, promptTemplate, convID string) func(ctx context.Context, msgs []memory.Message) (memory.Message, error) {
	if strings.TrimSpace(promptTemplate) == "" {
		promptTemplate = defaultPrompt
	}
	tmpl, _ := template.New("sum").Parse(promptTemplate)

	return func(ctx context.Context, msgs []memory.Message) (memory.Message, error) {
		var raw strings.Builder
		for _, m := range msgs {
			raw.WriteString(m.Role)
			raw.WriteString(": ")
			raw.WriteString(m.Content)
			raw.WriteString("\n")
		}
		var buf strings.Builder
		_ = tmpl.Execute(&buf, map[string]string{"conversation": raw.String()})

		genOut := &core.GenerateOutput{}
		if err := llmCore.Generate(ctx, &core.GenerateInput{
			Model:        model,
			SystemPrompt: buf.String(),
		}, genOut); err != nil {
			return memory.Message{}, err
		}
		return memory.Message{Role: "system", Content: genOut.Content, ConversationID: convID}, nil
	}
}

// Summarize retrieves lastN messages and returns summary content.
func Summarize(ctx context.Context, hist memory.History, llmCore *core.Service, model, convID string, lastN int, prompt string) (string, error) {
	if hist == nil {
		return "", nil
	}
	msgs, err := hist.Retrieve(ctx, convID, memory.NewLastNPolicy(lastN))
	if err != nil {
		return "", err
	}
	sumFn := Build(llmCore, model, prompt, convID)
	msg, err := sumFn(ctx, msgs)
	if err != nil {
		return "", err
	}
	return msg.Content, nil
}
