package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/llm/provider/base"
	mcbuf "github.com/viant/agently/genai/modelcallctx"
)

// Scanner buffer sizes for SSE processing
const (
	sseInitialBuf = 64 * 1024
	sseMaxBuf     = 1024 * 1024
)

// publishUsageOnce notifies the usage listener exactly once per stream.
func (c *Client) publishUsageOnce(model string, usage *llm.Usage, published *bool) {
	if c == nil || c.UsageListener == nil || published == nil {
		return
	}
	if *published {
		return
	}
	if model == "" || usage == nil {
		return
	}
	c.UsageListener.OnUsage(model, usage)
	*published = true
}

// endObserverOnce emits OnCallEnd once with the provided final response.
func endObserverOnce(observer mcbuf.Observer, ctx context.Context, model string, lr *llm.GenerateResponse, usage *llm.Usage, ended *bool) {
	if ended == nil || *ended {
		return
	}
	if observer != nil {
		var respJSON []byte
		var finish string
		if lr != nil {
			respJSON, _ = json.Marshal(lr)
			if len(lr.Choices) > 0 {
				finish = lr.Choices[0].FinishReason
			}
		}
		observer.OnCallEnd(ctx, mcbuf.Info{Provider: "openai", Model: model, ModelKind: "chat", ResponseJSON: respJSON, CompletedAt: time.Now(), Usage: usage, FinishReason: finish, LLMResponse: lr})
		*ended = true
	}
}

// emitResponse wraps publishing a response event.
func emitResponse(out chan<- llm.StreamEvent, lr *llm.GenerateResponse) {
	if out == nil || lr == nil {
		return
	}
	out <- llm.StreamEvent{Response: lr}
}

func (c *Client) Implements(feature string) bool {
	switch feature {
	case base.CanUseTools:
		return true
	case base.CanStream:
		return true
	}
	return false
}

// Generate sends a chat request to the OpenAI API and returns the response
func (c *Client) Generate(ctx context.Context, request *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	// Prepare request
	req, err := c.prepareChatRequest(request)
	if err != nil {
		return nil, err
	}
	payload, err := c.marshalRequestBody(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.createHTTPChatRequest(ctx, payload)
	if err != nil {
		return nil, err
	}

	// Observer start: include generic llm request as Payload JSON
	observer := mcbuf.ObserverFromContext(ctx)
	if observer != nil {
		var genReqJSON []byte
		if request != nil {
			genReqJSON, _ = json.Marshal(request)
		}
		ctx = observer.OnCallStart(ctx, mcbuf.Info{Provider: "openai", Model: req.Model, ModelKind: "chat", RequestJSON: payload, Payload: genReqJSON, StartedAt: time.Now()})
	}
	// Execute
	c.HTTPClient.Timeout = 10 * time.Minute
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, respBytes)
	}
	lr, perr := c.parseGenerateResponse(req.Model, respBytes)
	// Observer end
	if observer != nil {
		info := mcbuf.Info{Provider: "openai", Model: req.Model, ModelKind: "chat", ResponseJSON: respBytes, CompletedAt: time.Now()}
		if lr != nil {
			info.Usage = lr.Usage
			// capture finish reason from first choice if available
			if len(lr.Choices) > 0 {
				info.FinishReason = lr.Choices[0].FinishReason
			}
			info.LLMResponse = lr
		}
		if err != nil {
			info.Err = err.Error()
		}
		observer.OnCallEnd(ctx, info)
	}
	return lr, perr
}

// prepareChatRequest converts a generic request and applies client/model defaults.
func (c *Client) prepareChatRequest(request *llm.GenerateRequest) (*Request, error) {
	req := ToRequest(request)
	if req.Model == "" {
		req.Model = c.Model
	}
	if req.MaxTokens == 0 && c.MaxTokens > 0 {
		req.MaxTokens = c.MaxTokens
	}
	if req.Temperature == nil && c.Temperature != nil {
		req.Temperature = c.Temperature
	}
	if req.Temperature == nil {
		if value, ok := modelTemperature[req.Model]; ok {
			req.Temperature = &value
		}
	}
	if req.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	//TODO only for test purpurposes - remove later
	//req.ParallelToolCalls = true

	return req, nil
}

func (c *Client) marshalRequestBody(req *Request) ([]byte, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	//fmt.Printf("req: %s\n=======\n", string(data))
	return data, nil
}

func (c *Client) createHTTPChatRequest(ctx context.Context, data []byte) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return httpReq, nil
}

func (c *Client) parseGenerateResponse(model string, respBytes []byte) (*llm.GenerateResponse, error) {
	var apiResp Response
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	llmResp := ToLLMSResponse(&apiResp)
	if c.UsageListener != nil && llmResp.Usage != nil && llmResp.Usage.TotalTokens > 0 {
		c.UsageListener.OnUsage(model, llmResp.Usage)
	}
	return llmResp, nil
}

// Stream sends a chat request to the OpenAI API with streaming enabled and returns a channel of partial responses.
func (c *Client) Stream(ctx context.Context, request *llm.GenerateRequest) (<-chan llm.StreamEvent, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	// Prepare request
	req, err := c.prepareChatRequest(request)
	if err != nil {
		return nil, err
	}
	req.Stream = true
	// Ask OpenAI to include usage in the final stream event if supported
	req.StreamOptions = &StreamOptions{IncludeUsage: true}
	payload, err := c.marshalRequestBody(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.createHTTPChatRequest(ctx, payload)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	// Observer start
	observer := mcbuf.ObserverFromContext(ctx)
	if observer != nil {
		var genReqJSON []byte
		if request != nil {
			genReqJSON, _ = json.Marshal(request)
		}
		ctx = observer.OnCallStart(ctx, mcbuf.Info{Provider: "openai", Model: req.Model, ModelKind: "chat", RequestJSON: payload, Payload: genReqJSON, StartedAt: time.Now()})
	}
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	events := make(chan llm.StreamEvent)
	go func() {
		defer resp.Body.Close()
		defer close(events)
		c.consumeStream(ctx, resp.Body, events)
	}()
	return events, nil
}

// ---- Streaming helpers ----

type aggTC struct {
	id    string
	index int
	name  string
	args  string
}

type choiceAgg struct {
	role    llm.MessageRole
	content strings.Builder
	tools   map[int]*aggTC
}

type streamAggregator struct {
	choices map[int]*choiceAgg
}

func newStreamAggregator() *streamAggregator { return &streamAggregator{choices: map[int]*choiceAgg{}} }

func (a *streamAggregator) updateDelta(ch StreamChoice) {
	ca, ok := a.choices[ch.Index]
	if !ok {
		ca = &choiceAgg{tools: map[int]*aggTC{}}
		a.choices[ch.Index] = ca
	}
	if ch.Delta.Role != "" {
		ca.role = llm.MessageRole(ch.Delta.Role)
	}
	if ch.Delta.Content != nil {
		ca.content.WriteString(*ch.Delta.Content)
	}
	for _, tc := range ch.Delta.ToolCalls {
		tca, ok := ca.tools[tc.Index]
		if !ok {
			tca = &aggTC{index: tc.Index}
			ca.tools[tc.Index] = tca
		}
		if tc.ID != "" {
			tca.id = tc.ID
		}
		if tc.Function.Name != "" {
			tca.name = tc.Function.Name
		}
		if tc.Function.Arguments != "" {
			tca.args += tc.Function.Arguments
		}
	}
}

func (a *streamAggregator) finalizeChoice(idx int, finish string) llm.Choice {
	ca := a.choices[idx]
	msg := llm.Message{}
	if ca != nil && ca.role != "" {
		msg.Role = ca.role
	} else {
		msg.Role = llm.RoleAssistant
	}
	if finish == "tool_calls" {
		if ca != nil && len(ca.tools) > 0 {
			type idxAgg struct {
				idx int
				a   *aggTC
			}
			items := make([]idxAgg, 0, len(ca.tools))
			for _, t := range ca.tools {
				items = append(items, idxAgg{idx: t.index, a: t})
			}
			for i := 1; i < len(items); i++ {
				j := i
				for j > 0 && items[j-1].idx > items[j].idx {
					items[j-1], items[j] = items[j], items[j-1]
					j--
				}
			}
			out := make([]llm.ToolCall, 0, len(items))
			for _, it := range items {
				t := it.a
				var arguments map[string]interface{}
				if err := json.Unmarshal([]byte(t.args), &arguments); err != nil {
					arguments = map[string]interface{}{"raw": t.args}
				}
				out = append(out, llm.ToolCall{ID: t.id, Name: t.name, Arguments: arguments, Type: "function", Function: llm.FunctionCall{Name: t.name, Arguments: t.args}})
			}
			msg.ToolCalls = out
		}
	} else {
		if ca != nil {
			msg.Content = ca.content.String()
		}
	}
	delete(a.choices, idx)
	return llm.Choice{Index: idx, Message: msg, FinishReason: finish}
}

func (c *Client) consumeStream(ctx context.Context, body io.Reader, events chan<- llm.StreamEvent) {
	proc := &streamProcessor{
		client:   c,
		ctx:      ctx,
		observer: mcbuf.ObserverFromContext(ctx),
		events:   events,
		agg:      newStreamAggregator(),
		state:    &streamState{},
	}

	// Read response body
	respBody, readErr := io.ReadAll(body)
	if readErr != nil {
		events <- llm.StreamEvent{Err: fmt.Errorf("failed to read response body: %w", readErr)}
		return
	}
	proc.respBody = respBody

	// Handle non-SSE error envelopes (OpenAI may return a JSON error instead of SSE)
	// Detect when the body does not contain any SSE data lines and looks like a JSON object
	if !bytes.Contains(respBody, []byte("data: ")) {
		type apiErr struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Param   string `json:"param"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		var e apiErr
		if err := json.Unmarshal(bytes.TrimSpace(respBody), &e); err == nil && e.Error.Message != "" {
			// Bubble error to caller and close stream
			if proc.observer != nil && !proc.state.ended {
				proc.observer.OnCallEnd(proc.ctx, mcbuf.Info{Provider: "openai", Model: proc.state.lastModel, ModelKind: "chat", ResponseJSON: respBody, CompletedAt: time.Now()})
				proc.state.ended = true
			}
			events <- llm.StreamEvent{Err: fmt.Errorf("openai error: %s (type=%s, param=%s, code=%s)", e.Error.Message, e.Error.Type, e.Error.Param, e.Error.Code)}
			return
		}
	}

	// Prepare scanner
	scanner := bufio.NewScanner(bytes.NewReader(respBody))
	buf := make([]byte, 0, sseInitialBuf)
	scanner.Buffer(buf, sseMaxBuf)

	// Scan each SSE line
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		if ok := proc.handleData(data); !ok {
			// Stop further processing on parsing error to match previous behavior.
			return
		}
	}

	// Finalize
	proc.finalize(scanner.Err())
}
