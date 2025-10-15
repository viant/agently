package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/prompt"
)

func TestGenerateInput_Init_IncludesTaskAttachments(t *testing.T) {
	tcs := []struct {
		name         string
		attachments  []*prompt.Attachment
		wantBinaries int
	}{
		{
			name:         "no attachments",
			attachments:  nil,
			wantBinaries: 0,
		},
		{
			name:         "single attachment",
			attachments:  []*prompt.Attachment{{Name: "a.txt", Data: []byte("hello")}},
			wantBinaries: 1,
		},
		{
			name:         "two attachments",
			attachments:  []*prompt.Attachment{{Name: "a.txt", Data: []byte("hello")}, {Name: "b.md", Data: []byte("world")}},
			wantBinaries: 2,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			in := &GenerateInput{
				Prompt:         &prompt.Prompt{Text: "question"},
				Binding:        &prompt.Binding{Task: prompt.Task{Attachments: tc.attachments}},
				ModelSelection: llm.ModelSelection{Model: "dummy"},
				UserID:         "u",
			}
			t.Logf("task attachments before init: %d", len(in.Binding.Task.Attachments))
			err := in.Init(context.Background())
			assert.NoError(t, err)
			// Count binaries
			binaries := 0
			for _, m := range in.Message {
				for _, it := range m.Items {
					if it.Type == llm.ContentTypeBinary {
						binaries++
					}
				}
			}
			t.Logf("messages=%d, binaries=%d", len(in.Message), binaries)
			assert.EqualValues(t, tc.wantBinaries, binaries)
		})
	}
}
