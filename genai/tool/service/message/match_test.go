package message

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	baseemb "github.com/viant/agently/genai/embedder/provider/base"
)

// fakeEmbedder produces 1-D vectors equal to text length
type fakeEmbedder struct{ baseemb.UsageListener }

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, int, error) {
	out := make([][]float32, len(texts))
	for i, s := range texts {
		out[i] = []float32{float32(len(s))}
	}
	return out, 0, nil
}

// fakeFinder returns our fake embedder
type fakeFinder struct{ e baseemb.Embedder }

func (f *fakeFinder) Find(ctx context.Context, id string) (baseemb.Embedder, error) { return f.e, nil }
func (f *fakeFinder) Ids() []string                                                 { return []string{"fake"} }

func TestMatch_Simple(t *testing.T) {
	svc := NewWithDeps(nil, nil, &fakeFinder{e: &fakeEmbedder{}}, 0, 0, "", "", "", "")
	body := "aaaa\nbb\ncccccc\n"
	in := &MatchInput{Body: body, Query: "xxxx", Chunk: 4, TopK: 2}
	var out MatchOutput
	err := svc.match(context.Background(), in, &out)
	assert.NoError(t, err)
	assert.EqualValues(t, len(body), out.Size)
	assert.EqualValues(t, 2, len(out.Fragments))
	// Fragments should be contiguous around highest-length chunk
	assert.NotEmpty(t, out.Fragments[0].Content)
}
