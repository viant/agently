package agent

import (
    "context"
    "github.com/viant/agently/genai/extension/fluxor/llm/core"
    "github.com/viant/agently/genai/memory"
    "strings"
    "text/template"
)

func summarize(s *Service, input *QueryInput) func(ctx context.Context, msgs []memory.Message) (memory.Message, error) {
    summarizer := func(ctx context.Context, msgs []memory.Message) (memory.Message, error) {
        // concatenate conversation
        var raw strings.Builder
        for _, m := range msgs {
            raw.WriteString(m.Role)
            raw.WriteString(": ")
            raw.WriteString(m.Content)
            raw.WriteString("\n")
        }

        // build prompt from template
        prompt := "Summarize the following conversation in a concise form:\n${conversation}"
        if s.summaryPrompt != "" {
            prompt = s.summaryPrompt
        }
        // simple template replacement
        tpl, _ := template.New("sum").Parse(prompt)
        var buf strings.Builder
        _ = tpl.Execute(&buf, map[string]string{"conversation": raw.String()})

        genOut := &core.GenerateOutput{}
        if err := s.llm.Generate(ctx, &core.GenerateInput{
            Model:        input.Agent.Model,
            SystemPrompt: buf.String(),
        }, genOut); err != nil {
            return memory.Message{}, err
        }
        return memory.Message{Role: "system", Content: genOut.Content}, nil
    }
	return summarizer
}
