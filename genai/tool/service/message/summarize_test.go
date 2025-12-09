package message

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	core "github.com/viant/agently/genai/service/core"
)

// fakeCore echoes a simple summary from the prompt text
type fakeCore struct{}

func (f *fakeCore) Generate(ctx context.Context, in *core.GenerateInput, out *core.GenerateOutput) error {
	if in == nil || in.Prompt == nil {
		out.Content = ""
		return nil
	}
	// small deterministic summary: prefix + first 8 chars
	txt := in.Prompt.Text
	if len(txt) > 8 {
		txt = txt[:8]
	}
	out.Content = "SUM:" + txt
	return nil
}

func TestSummarize_Chunks(t *testing.T) {
	svc := NewWithDeps(nil, &fakeCore{}, nil, 0, 0, "", "", "", "")
	body := "abcdefghABCDEFGH"
	in := &SummarizeInput{Body: body, Chunk: 4, Page: 1, PerPage: 10}
	var out SummarizeOutput
	err := svc.summarize(context.Background(), in, &out)
	assert.NoError(t, err)
	assert.EqualValues(t, len(body), out.Size)
	// effectiveChunkSize enforces a minimum chunk size; with a short body this
	// results in a single chunk regardless of requested Chunk.
	assert.EqualValues(t, 1, len(out.Chunks))
	assert.True(t, len(out.Summary) > 0)
}
