package message

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	apiconv "github.com/viant/agently/client/conversation"
	baseemb "github.com/viant/agently/genai/embedder/provider/base"
	cover "github.com/viant/gds/tree/cover"
)

type MatchInput struct {
	Body      string `json:"body" internal:"true"`
	MessageID string `json:"messageId"`
	Query     string `json:"query"`
	TopK      int    `json:"topK,omitempty"`
	Chunk     int    `json:"chunk,omitempty"`
}
type MatchFragment struct {
	Offset  int     `json:"offset"`
	Limit   int     `json:"limit"`
	Score   float64 `json:"score"`
	Content string  `json:"content"`
}
type MatchOutput struct {
	Size      int             `json:"size"`
	Fragments []MatchFragment `json:"fragments"`
}

func (s *Service) match(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*MatchInput)
	if !ok {
		return fmt.Errorf("invalid input")
	}
	output, ok := out.(*MatchOutput)
	if !ok {
		return fmt.Errorf("invalid output")
	}
	if s.embedder == nil {
		return fmt.Errorf("embedder not initialised")
	}
	body := strings.TrimSpace(input.Body)
	if body == "" && strings.TrimSpace(input.MessageID) != "" && s.conv != nil {
		if msg, _ := s.conv.GetMessage(ctx, input.MessageID, apiconv.WithIncludeToolCall(true)); msg != nil {
			body = msg.GetContent()
		}
	}
	size := len(body)
	chunk := effectiveMatchChunk(input.Chunk, s.matchChunk)
	topK := effectiveTopK(input.TopK)
	emb, err := resolveEmbedder(ctx, s.embedder, s.embedModel)
	if err != nil {
		return err
	}
	parts := splitParts(body, chunk)
	tree, err := buildTree(ctx, parts, emb)
	if err != nil {
		return err
	}
	frags, err := searchTopK(ctx, tree, emb, parts, body, input.Query, topK)
	if err != nil {
		return err
	}
	// Lazy persist embedding index if message id provided
	if id := strings.TrimSpace(input.MessageID); id != "" {
		var buf bytes.Buffer
		if err := tree.ExportDocuments(&buf); err == nil {
			mm := apiconv.NewMessage()
			mm.SetId(id)
			mm.SetEmbeddingIndex(buf.Bytes())
			_ = s.conv.PatchMessage(ctx, mm)
		}
	}
	output.Size = size
	output.Fragments = frags
	return nil
}

func effectiveMatchChunk(req, def int) int {
	if req > 0 {
		return req
	}
	if def > 0 {
		return def
	}
	return 1024
}
func effectiveTopK(req int) int {
	if req > 0 {
		return req
	}
	return 5
}

type textPart struct {
	off, end int
	text     string
}

func splitParts(body string, chunk int) []textPart {
	size := len(body)
	var parts []textPart
	for off := 0; off < size; off += chunk {
		end := off + chunk
		if end > size {
			end = size
		}
		parts = append(parts, textPart{off: off, end: end, text: body[off:end]})
	}
	return parts
}

func resolveEmbedder(ctx context.Context, finder interface {
	Find(context.Context, string) (baseemb.Embedder, error)
	Ids() []string
}, model string) (baseemb.Embedder, error) {
	id := model
	if id == "" {
		ids := finder.Ids()
		if len(ids) == 0 {
			return nil, fmt.Errorf("no embedder configured")
		}
		id = ids[0]
	}
	return finder.Find(ctx, id)
}

func buildTree(ctx context.Context, parts []textPart, emb baseemb.Embedder) (*cover.Tree[int], error) {
	tree := cover.NewTree[int](1.3, cover.DistanceFunctionCosine)
	texts := make([]string, len(parts))
	for i, tp := range parts {
		texts[i] = tp.text
	}
	vecs, _, err := emb.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	for i := range vecs {
		v := make([]float32, len(vecs[i]))
		copy(v, vecs[i])
		tree.Insert(i, cover.NewPoint(v...))
	}
	return tree, nil
}

func searchTopK(ctx context.Context, tree *cover.Tree[int], emb baseemb.Embedder, parts []textPart, body, query string, topK int) ([]MatchFragment, error) {
	qvVecs, _, err := emb.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(qvVecs) == 0 {
		return nil, fmt.Errorf("empty embedding for query")
	}
	neighbors := tree.KNearestNeighborsBestFirst(cover.NewPoint(qvVecs[0]...), topK)
	if len(neighbors) < topK {
		topK = len(neighbors)
	}
	frags := make([]MatchFragment, 0, topK)
	for i := 0; i < topK; i++ {
		idx := tree.Value(neighbors[i].Point)
		start := parts[idx].off
		end := parts[idx].end
		if idx > 0 {
			start = parts[idx-1].off
		}
		if idx+1 < len(parts) {
			end = parts[idx+1].end
		}
		frags = append(frags, MatchFragment{Offset: start, Limit: end - start, Score: 1 - float64(neighbors[i].Distance), Content: body[start:end]})
	}
	return frags, nil
}
