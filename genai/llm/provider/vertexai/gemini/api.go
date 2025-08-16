package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/viant/agently/genai/llm/provider/base"

	"github.com/viant/agently/genai/llm"
)

func (c *Client) Implements(feature string) bool {
	switch feature {
	case base.CanUseTools:
		return true
	case base.CanStream:
		return c.canStream()
	}
	return false
}

func (c *Client) canStream() bool {
	m := strings.ToLower(c.Model)
	// Gemini embedding endpoints do not stream
	if strings.Contains(m, "embed") || strings.Contains(m, "embedding") {
		return false
	}
	return true
}

// Generate generates a response using the Gemini API
func (c *Client) Generate(ctx context.Context, request *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	if c.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	// Convert llms.ChatRequest to Request
	req, err := ToRequest(ctx, request)
	if err != nil {
		return nil, err
	}
	// client defaults
	if req.GenerationConfig != nil {
		if req.GenerationConfig.MaxOutputTokens == 0 && c.MaxTokens > 0 {
			req.GenerationConfig.MaxOutputTokens = c.MaxTokens
		}
		if req.GenerationConfig.Temperature == 0 && c.Temperature != nil {
			req.GenerationConfig.Temperature = *c.Temperature
		}
	} else {
		gc := &GenerationConfig{}
		if c.MaxTokens > 0 {
			gc.MaxOutputTokens = c.MaxTokens
		}
		if c.Temperature != nil {
			gc.Temperature = *c.Temperature
		}
		if gc.MaxOutputTokens > 0 || gc.Temperature != 0 {
			req.GenerationConfig = gc
		}
	}

	// apply client defaults
	if req.GenerationConfig != nil {
		if req.GenerationConfig.MaxOutputTokens == 0 && c.MaxTokens > 0 {
			req.GenerationConfig.MaxOutputTokens = c.MaxTokens
		}
		if req.GenerationConfig.Temperature == 0 && c.Temperature != nil {
			req.GenerationConfig.Temperature = *c.Temperature
		}
	} else {
		gc := &GenerationConfig{}
		if c.MaxTokens > 0 {
			gc.MaxOutputTokens = c.MaxTokens
		}
		if c.Temperature != nil {
			gc.Temperature = *c.Temperature
		}
		if gc.MaxOutputTokens > 0 || gc.Temperature != 0 {
			req.GenerationConfig = gc
		}
	}

	// Marshal the request to JSON
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the URL with API key as query parameter
	// c.Find should be the full resource name, e.g., "projects/{project}/locations/{location}/models/{model}"
	apiURL := fmt.Sprintf("%s/%s:generateContent?key=%s", c.BaseURL, c.Model, c.APIKey)

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	//fmt.Printf("req: %s\n", string(data))
	// Read the response body
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	//fmt.Printf("resp: %s\n", string(respBytes))

	// Check for non-200 status code
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Gemini API error (status %d): %s", resp.StatusCode, respBytes)
	}

	// Unmarshal the response
	var apiResp Response
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Convert Response to llms.ChatResponse
	llmsResp := ToLLMSResponse(&apiResp)
	if c.UsageListener != nil && llmsResp.Usage != nil && llmsResp.Usage.TotalTokens > 0 {
		c.UsageListener.OnUsage(request.Options.Model, llmsResp.Usage)
	}
	return llmsResp, nil
}

// Stream sends a chat request to the Gemini API with streaming enabled and returns a channel of partial responses.
func (c *Client) Stream(ctx context.Context, request *llm.GenerateRequest) (<-chan llm.StreamEvent, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if c.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	// Convert llm.GenerateRequest to wire request; for streaming we must use the
	// streamGenerateContent endpoint (no "stream" field in the request body).
	req, err := ToRequest(ctx, request)
	if err != nil {
		return nil, err
	}
	// Ensure we do not send an unsupported field
	req.Stream = false

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	apiURL := fmt.Sprintf("%s/%s:streamGenerateContent?key=%s", c.BaseURL, c.Model, c.APIKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Vertex Gemini stream returns application/json; request JSON explicitly.
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gemini stream error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	events := make(chan llm.StreamEvent)
	go func() {
		defer resp.Body.Close()
		defer close(events)
		_ = resp.Header.Get("Content-Type")
		agg := newGeminiAggregator(c.Model, c.UsageListener)
		// Gemini uses application/json streams; decode with JSON decoder.
		c.streamJSON(resp.Body, events, agg)
		// drain any leftover on disconnect
		agg.emitRemainder(events)
	}()
	return events, nil
}

// geminiAggregator accumulates per-candidate content/tool calls and emits only when finished.
type geminiAggregator struct {
	model     string
	text      map[int]*strings.Builder
	tools     map[int][]llm.ToolCall
	finish    map[int]string
	usage     *llm.Usage
	listener  base.UsageListener
	published bool
}

func newGeminiAggregator(model string, listener base.UsageListener) *geminiAggregator {
	return &geminiAggregator{
		model:    model,
		text:     map[int]*strings.Builder{},
		tools:    map[int][]llm.ToolCall{},
		finish:   map[int]string{},
		listener: listener,
	}
}

func (a *geminiAggregator) addResponse(resp *Response) {
	// capture usage if provided in this chunk
	if resp.UsageMetadata != nil {
		u := &llm.Usage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		}
		// Record only when meaningful
		if u.TotalTokens > 0 || u.PromptTokens > 0 || u.CompletionTokens > 0 {
			a.usage = u
			if a.listener != nil && !a.published && a.model != "" {
				a.listener.OnUsage(a.model, a.usage)
				a.published = true
			}
		}
	}
	for _, cand := range resp.Candidates {
		idx := cand.Index
		if _, ok := a.text[idx]; !ok {
			a.text[idx] = &strings.Builder{}
		}
		for _, p := range cand.Content.Parts {
			if p.Text != "" {
				a.text[idx].WriteString(p.Text)
			}
			if p.FunctionCall != nil {
				var args map[string]interface{}
				if p.FunctionCall.Args != nil {
					if m, ok := p.FunctionCall.Args.(map[string]interface{}); ok {
						args = m
					}
				} else if p.FunctionCall.Arguments != "" {
					_ = json.Unmarshal([]byte(p.FunctionCall.Arguments), &args)
				}
				a.tools[idx] = append(a.tools[idx], llm.ToolCall{Name: p.FunctionCall.Name, Arguments: args})
			}
		}
		if cand.FinishReason != "" {
			a.finish[idx] = cand.FinishReason
		}
	}
}

// emitFinished builds and emits a response only for completed candidates, then clears them.
func (a *geminiAggregator) emitFinished(events chan<- llm.StreamEvent) {
	if len(a.finish) == 0 {
		return
	}
	out := &llm.GenerateResponse{Model: a.model}
	for idx, reason := range a.finish {
		msg := llm.Message{Role: llm.RoleAssistant, Content: a.text[idx].String()}
		if calls := a.tools[idx]; len(calls) > 0 {
			msg.ToolCalls = calls
		}
		out.Choices = append(out.Choices, llm.Choice{Index: idx, Message: msg, FinishReason: reason})
		delete(a.text, idx)
		delete(a.tools, idx)
	}
	a.finish = map[int]string{}
	if a.usage != nil && a.usage.TotalTokens > 0 {
		out.Usage = a.usage
	}
	if len(out.Choices) > 0 {
		events <- llm.StreamEvent{Response: out}
	}
}

// emitRemainder flushes any non-finished candidates on stream end, using STOP finish reason.
func (a *geminiAggregator) emitRemainder(events chan<- llm.StreamEvent) {
	if len(a.text) == 0 && len(a.tools) == 0 {
		return
	}
	out := &llm.GenerateResponse{Model: a.model}
	for idx, b := range a.text {
		msg := llm.Message{Role: llm.RoleAssistant, Content: b.String()}
		if calls := a.tools[idx]; len(calls) > 0 {
			msg.ToolCalls = calls
		}
		out.Choices = append(out.Choices, llm.Choice{Index: idx, Message: msg, FinishReason: "STOP"})
	}
	// clear state
	a.text = map[int]*strings.Builder{}
	a.tools = map[int][]llm.ToolCall{}
	a.finish = map[int]string{}
	if a.usage != nil && a.usage.TotalTokens > 0 {
		out.Usage = a.usage
	}
	if len(out.Choices) > 0 {
		events <- llm.StreamEvent{Response: out}
	}
}

func (c *Client) emitPayloadAggregate(payload string, events chan<- llm.StreamEvent, agg *geminiAggregator) {
	s := strings.TrimSpace(payload)
	if s == "" || s == "[DONE]" {
		return
	}
	if strings.HasPrefix(s, "[") {
		var arr []Response
		if err := json.Unmarshal([]byte(s), &arr); err != nil {
			events <- llm.StreamEvent{Err: fmt.Errorf("failed to unmarshal stream array: %w", err)}
			return
		}
		for i := range arr {
			agg.addResponse(&arr[i])
		}
		agg.emitFinished(events)
		return
	}
	var obj Response
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		events <- llm.StreamEvent{Err: fmt.Errorf("failed to unmarshal stream object: %w", err)}
		return
	}
	agg.addResponse(&obj)
	agg.emitFinished(events)
}

// streamJSON handles application/json streams. It supports:
// - a single JSON array where each element is a Response
// - multiple top-level JSON objects (sequential), separated by whitespace
func (c *Client) streamJSON(r io.Reader, events chan<- llm.StreamEvent, agg *geminiAggregator) {
	br := bufio.NewReader(r)
	isSpace := func(b byte) bool { return b == ' ' || b == '\n' || b == '\r' || b == '\t' }
	for {
		// skip leading whitespace between top-level values
		for {
			b, err := br.Peek(1)
			if err != nil {
				if err == io.EOF {
					return
				}
				events <- llm.StreamEvent{Err: fmt.Errorf("stream read error: %w", err)}
				return
			}
			if isSpace(b[0]) {
				_, _ = br.ReadByte()
				continue
			}
			break
		}

		b, err := br.Peek(1)
		if err != nil {
			if err == io.EOF {
				return
			}
			events <- llm.StreamEvent{Err: fmt.Errorf("stream read error: %w", err)}
			return
		}

		switch b[0] {
		case '[':
			dec := json.NewDecoder(br)
			// read opening array token
			tok, err := dec.Token()
			if err != nil {
				if err == io.EOF {
					return
				}
				events <- llm.StreamEvent{Err: fmt.Errorf("json decode error: %w", err)}
				return
			}
			if d, ok := tok.(json.Delim); !ok || d != '[' {
				events <- llm.StreamEvent{Err: fmt.Errorf("unexpected JSON, expected array start")}
				return
			}
			for dec.More() {
				var obj Response
				if err := dec.Decode(&obj); err != nil {
					if err == io.EOF {
						break
					}
					events <- llm.StreamEvent{Err: fmt.Errorf("json decode error: %w", err)}
					return
				}
				agg.addResponse(&obj)
				agg.emitFinished(events)
			}
			// consume closing ']'
			_, _ = dec.Token()
		case '{':
			dec := json.NewDecoder(br)
			var obj Response
			if err := dec.Decode(&obj); err != nil {
				if err == io.EOF {
					return
				}
				events <- llm.StreamEvent{Err: fmt.Errorf("json decode error: %w", err)}
				return
			}
			agg.addResponse(&obj)
			agg.emitFinished(events)
		default:
			// Unexpected leading byte; return an error to surface malformed stream
			events <- llm.StreamEvent{Err: fmt.Errorf("unexpected JSON stream start: %q", string(b[0]))}
			return
		}
	}
}
