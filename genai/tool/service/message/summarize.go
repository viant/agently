package message

import (
	"context"
	"fmt"
	"strings"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/prompt"
	core "github.com/viant/agently/genai/service/core"
)

type SummarizeInput struct {
	Body      string `json:"body" internal:"true"`
	MessageID string `json:"messageId"`
	Chunk     int    `json:"chunk,omitempty"`
	Page      int    `json:"page,omitempty"`
	PerPage   int    `json:"perPage,omitempty"`
}

type SummarizeChunk struct {
	Offset  int    `json:"offset"`
	Limit   int    `json:"limit"`
	Summary string `json:"summary"`
}

type SummarizeOutput struct {
	Size        int              `json:"size"`
	Chunks      []SummarizeChunk `json:"chunks"`
	Summary     string           `json:"summary"`
	TotalChunks int              `json:"totalChunks"`
	TotalPages  int              `json:"totalPages"`
	Page        int              `json:"page"`
	PerPage     int              `json:"perPage"`
}

func (s *Service) summarize(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*SummarizeInput)
	if !ok {
		return fmt.Errorf("invalid input")
	}
	output, ok := out.(*SummarizeOutput)
	if !ok {
		return fmt.Errorf("invalid output")
	}
	if s.core == nil {
		return fmt.Errorf("summarizer not initialised")
	}
	body := strings.TrimSpace(input.Body)
	if body == "" && strings.TrimSpace(input.MessageID) != "" && s.conv != nil {
		if msg, _ := s.conv.GetMessage(ctx, input.MessageID, apiconv.WithIncludeToolCall(true)); msg != nil {
			body = msg.GetContent()
		}
	}
	size := len(body)
	chunk := effectiveChunkSize(input.Chunk, s.summarizeChunk)
	chunks, err := s.summarizeChunksParallel(ctx, body, chunk)
	if err != nil {
		return err
	}
	pageChunks, total, totalPages, page, perPage := paginateSummaries(chunks, input.Page, input.PerPage)
	output.Size = size
	output.Chunks = pageChunks
	output.Summary = joinSummaries(pageChunks)
	output.TotalChunks = total
	output.TotalPages = totalPages
	output.Page = page
	output.PerPage = perPage
	// Lazy persist summary on the message when an id is provided and we computed a page summary
	if strings.TrimSpace(input.MessageID) != "" && strings.TrimSpace(output.Summary) != "" {
		mm := apiconv.NewMessage()
		mm.SetId(input.MessageID)
		mm.SetSummary(output.Summary)
		err = s.conv.PatchMessage(ctx, mm)
	}
	return err
}

func (s *Service) summarizeChunksParallel(ctx context.Context, body string, chunk int) ([]SummarizeChunk, error) {
	size := len(body)
	type item struct {
		idx, off, end int
		sum           string
		err           error
	}
	n := (size + chunk - 1) / chunk
	items := make([]item, n)
	for i := 0; i < n; i++ {
		off := i * chunk
		end := off + chunk
		if end > size {
			end = size
		}
		items[i] = item{idx: i, off: off, end: end}
	}
	sem := make(chan struct{}, 8)
	done := make(chan item, n)
	for _, it := range items {
		sem <- struct{}{}
		go func(it item) {
			defer func() { <-sem }()
			sys := &prompt.Prompt{Text: strings.TrimSpace(firstNonEmpty(s.summaryPrompt, "Summarize the following content concisely. Focus on key points."))}
			pr := &prompt.Prompt{Text: body[it.off:it.end]}
			var genIn core.GenerateInput
			genIn.SystemPrompt = sys
			genIn.Prompt = pr
			genIn.Binding = &prompt.Binding{}
			model := strings.TrimSpace(s.summaryModel)
			if model == "" {
				model = strings.TrimSpace(s.defaultModel)
			}
			genIn.Model = model
			genIn.UserID = "system"
			var genOut core.GenerateOutput
			it.err = s.core.Generate(ctx, &genIn, &genOut)
			if it.err != nil && strings.Contains(it.err.Error(), "failed to find model") {
				fallback := strings.TrimSpace(s.defaultModel)
				if fallback != "" && fallback != model {
					genIn.Model = fallback
					it.err = s.core.Generate(ctx, &genIn, &genOut)
				}
			}
			if it.err == nil {
				it.sum = strings.TrimSpace(genOut.Content)
			}
			done <- it
		}(it)
	}
	results := make([]item, n)
	for i := 0; i < n; i++ {
		it := <-done
		results[it.idx] = it
	}
	chunks := make([]SummarizeChunk, 0, n)
	for _, it := range results {
		if it.err != nil {
			return nil, it.err
		}
		chunks = append(chunks, SummarizeChunk{Offset: it.off, Limit: it.end - it.off, Summary: it.sum})
	}
	return chunks, nil
}

func effectiveChunkSize(req, def int) int {
	min := 4096
	if req > min {
		return req
	}
	if def > min {
		return def
	}
	return min
}

func paginateSummaries(chunks []SummarizeChunk, page, perPage int) ([]SummarizeChunk, int, int, int, int) {
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	if page <= 0 {
		page = 1
	}
	total := len(chunks)
	totalPages := (total + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * perPage
	if start < 0 {
		start = 0
	}
	end := start + perPage
	if end > total {
		end = total
	}
	var pageChunks []SummarizeChunk
	if total > 0 {
		pageChunks = chunks[start:end]
	}
	return pageChunks, total, totalPages, page, perPage
}
func joinSummaries(chunks []SummarizeChunk) string {
	var b strings.Builder
	for i, c := range chunks {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(c.Summary)
	}
	return b.String()
}
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
