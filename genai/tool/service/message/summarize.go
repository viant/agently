package message

import (
	"context"
	"fmt"
	"strings"
	"time"

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
		// Fetch with tool calls to allow fallback to tool payload when textual content is absent.
		if msg, _ := s.conv.GetMessage(ctx, input.MessageID, apiconv.WithIncludeToolCall(true)); msg != nil {
			used := ""
			if c := strings.TrimSpace(msg.GetContent()); c != "" {
				body = c
				used = c
			} else if alt, _ := preferToolPayload(msg, used); len(alt) > 0 {
				body = strings.TrimSpace(string(alt))
			}
		}
	}
	size := len(strings.TrimSpace(body))
	if size == 0 {
		// Nothing to summarize; return empty output without model calls.
		output.Size = 0
		output.Chunks = nil
		output.Summary = ""
		output.TotalChunks = 0
		output.TotalPages = 1
		output.Page = effectivePage(input.Page)
		output.PerPage = effectivePerPage(input.PerPage)
		return nil
	}
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
	// Prepare work items (text ranges per chunk)
	items := makeChunkItems(len(body), chunk)
	n := len(items)
	if n == 0 {
		return nil, nil
	}

	// Burst limiter: allow up to 3 Generate calls per second (with initial burst of 3)
	limiter := newBurstLimiter(ctx, 3, time.Second)

	// Concurrency control and result channel
	sem := make(chan struct{}, 8)
	done := make(chan chunkItem, n)

	for _, it := range items {
		sem <- struct{}{}
		go func(it chunkItem) {
			defer func() { <-sem }()
			sum, blank, err := s.generateSummaryForRange(ctx, body, it.off, it.end, limiter)
			it.sum, it.err, it.blank = sum, err, blank
			done <- it
		}(it)
	}

	results := make([]chunkItem, n)
	for i := 0; i < n; i++ {
		it := <-done
		results[it.idx] = it
	}

	chunks := make([]SummarizeChunk, 0, n)
	for _, it := range results {
		if it.err != nil {
			return nil, it.err
		}
		if it.blank {
			// Skip empty segments (whitespace-only input)
			continue
		}
		chunks = append(chunks, SummarizeChunk{Offset: it.off, Limit: it.end - it.off, Summary: it.sum})
	}
	return chunks, nil
}

// chunkItem represents a body segment to summarize.
type chunkItem struct {
	idx, off, end int
	sum           string
	err           error
	blank         bool
}

// makeChunkItems splits the body into chunk-sized ranges and returns items with indices.
func makeChunkItems(size, chunk int) []chunkItem {
	if chunk <= 0 {
		return nil
	}
	n := (size + chunk - 1) / chunk
	items := make([]chunkItem, n)
	for i := 0; i < n; i++ {
		off := i * chunk
		end := off + chunk
		if end > size {
			end = size
		}
		items[i] = chunkItem{idx: i, off: off, end: end}
	}
	return items
}

// generateSummaryForRange summarizes body[off:end], honoring the provided rate limiter.
func (s *Service) generateSummaryForRange(ctx context.Context, body string, off, end int, limiter <-chan struct{}) (string, bool, error) {
	text := body[off:end]
	if strings.TrimSpace(text) == "" {
		return "", true, nil
	}
	sysPrompt := strings.TrimSpace(firstNonEmpty(s.summaryPrompt, "Summarize the following content concisely. Focus on key points."))
	model := s.activeModel()

	genIn := s.buildGenerateInput(sysPrompt, text, model)
	var genOut core.GenerateOutput

	// First attempt (rate-limited)
	if err := waitForPermit(ctx, limiter); err != nil {
		return "", false, err
	}
	err := s.core.Generate(ctx, &genIn, &genOut)
	if err == nil {
		return strings.TrimSpace(genOut.Content), false, nil
	}

	// Retry with fallback model when applicable (also rate-limited)
	if strings.Contains(err.Error(), "failed to find model") {
		fallback := strings.TrimSpace(s.defaultModel)
		if fallback != "" && fallback != model {
			genIn.Model = fallback
			if err := waitForPermit(ctx, limiter); err != nil {
				return "", false, err
			}
			if ferr := s.core.Generate(ctx, &genIn, &genOut); ferr == nil {
				return strings.TrimSpace(genOut.Content), false, nil
			} else {
				return "", false, ferr
			}
		}
	}
	return "", false, err
}

// activeModel resolves the preferred model for summarization.
func (s *Service) activeModel() string {
	model := strings.TrimSpace(s.summaryModel)
	if model == "" {
		model = strings.TrimSpace(s.defaultModel)
	}
	return model
}

// buildGenerateInput constructs a GenerateInput for the given parameters.
func (s *Service) buildGenerateInput(systemPrompt, userText, model string) core.GenerateInput {
	var in core.GenerateInput
	in.SystemPrompt = &prompt.Prompt{Text: systemPrompt}
	in.Prompt = &prompt.Prompt{Text: userText}
	in.Binding = &prompt.Binding{}
	in.Model = model
	in.UserID = "system"
	return in
}

// newBurstLimiter returns a token channel initially filled up to capacity and
// refilled to capacity every refillInterval. Consumers must receive a token
// before performing a rate-limited operation. When ctx is done, the refiller exits.
func newBurstLimiter(ctx context.Context, capacity int, refillInterval time.Duration) <-chan struct{} {
	if capacity <= 0 {
		capacity = 1
	}
	tokens := make(chan struct{}, capacity)
	// Initial burst fill
	for i := 0; i < capacity; i++ {
		tokens <- struct{}{}
	}
	// Refill loop
	go func() {
		ticker := time.NewTicker(refillInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Top up to full capacity
				for len(tokens) < capacity {
					select {
					case tokens <- struct{}{}:
					default:
						break
					}
				}
			}
		}
	}()
	return tokens
}

// waitForPermit blocks until either a token is available or the context is done.
func waitForPermit(ctx context.Context, tokens <-chan struct{}) error {
	if tokens == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-tokens:
		return nil
	}
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
	perPage = effectivePerPage(perPage)
	page = effectivePage(page)
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
func effectivePerPage(perPage int) int {
	if perPage <= 0 {
		return 20
	}
	if perPage > 100 {
		return 100
	}
	return perPage
}
func effectivePage(page int) int {
	if page <= 0 {
		return 1
	}
	return page
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
