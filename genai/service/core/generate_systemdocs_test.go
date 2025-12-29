package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/prompt"
)

func TestGenerateInput_Init_AppendsSystemDocuments_DataDriven(t *testing.T) {
	type testCase struct {
		name        string
		systemDocs  []*prompt.Document
		expectedMsg []llm.Message
	}

	cases := []testCase{
		{
			name:       "adds each system document as system message",
			systemDocs: []*prompt.Document{{PageContent: "playbook-1"}, {PageContent: "playbook-2"}},
			expectedMsg: []llm.Message{
				llm.NewTextMessage(llm.MessageRole("system"), "playbook-1"),
				llm.NewTextMessage(llm.MessageRole("system"), "playbook-2"),
			},
		},
		{
			name:        "no system documents yields no added messages",
			systemDocs:  nil,
			expectedMsg: []llm.Message{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &prompt.Binding{}
			if len(tc.systemDocs) > 0 {
				b.SystemDocuments.Items = tc.systemDocs
			}
			in := &GenerateInput{
				Prompt:  &prompt.Prompt{Text: "hello"},
				Binding: b,
			}
			err := in.Init(context.Background())
			assert.EqualValues(t, nil, err)

			got := make([]llm.Message, 0)
			for _, m := range in.Message {
				if m.Role == "system" && m.Content != "" {
					got = append(got, m)
				}
			}
			assert.EqualValues(t, tc.expectedMsg, got)
		})
	}
}
