package adapter

import (
	"context"
	baseemb "github.com/viant/agently/genai/embedder/provider/base"
)

// LangchainEmbedderAdapter adapts our base embedder to the older
// langchaingo-compatible method set expected by Embedius indexer.
// It intentionally does not import the langchaingo package; Go's
// structural typing ensures compatibility at the call site.
type LangchainEmbedderAdapter struct {
	Inner baseemb.Embedder
}

// EmbedDocuments embeds multiple texts; returns only vectors.
func (a LangchainEmbedderAdapter) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	vecs, _, err := a.Inner.Embed(ctx, texts)
	return vecs, err
}

// EmbedQuery embeds a single query string; returns a single vector.
func (a LangchainEmbedderAdapter) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vecs, _, err := a.Inner.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, nil
	}
	return vecs[0], nil
}
