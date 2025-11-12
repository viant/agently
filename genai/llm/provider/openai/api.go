package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/llm/provider/base"
	mcbuf "github.com/viant/agently/genai/modelcallctx"
	core "github.com/viant/agently/genai/service/core"
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
func endObserverOnce(observer mcbuf.Observer, ctx context.Context, model string, lr *llm.GenerateResponse, usage *llm.Usage, ended *bool) error {
	if ended == nil || *ended {
		return nil
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
		if err := observer.OnCallEnd(ctx, mcbuf.Info{Provider: "openai", Model: model, ModelKind: "chat", ResponseJSON: respJSON, CompletedAt: time.Now(), Usage: usage, FinishReason: finish, LLMResponse: lr}); err != nil {
			return err
		}
		*ended = true
	}
	return nil
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
	case base.IsMultimodal:
		return c.canMultimodal()
	case base.CanExecToolsInParallel:
		return true
	case base.SupportsContinuationByResponseID:
		return true
	}
	return false
}

func (c *Client) canMultimodal() bool {
	//TODO
	/*
	   m := strings.ToLower(c.Model)
	   // Heuristic: enable only on known vision-capable chat families.
	   // Examples: gpt-4o, gpt-4o-mini, gpt-4.1, gpt-4.1-mini, omni, vision
	   keywords := []string{"gpt-4o", "4o", "4.1", "-omni", "vision"}
	   for _, kw := range keywords {
	       if strings.Contains(m, kw) {
	           return true
	       }
	   }
	   return false
	*/
	return true
}

// Generate sends a chat request to the OpenAI API and returns the response
func (c *Client) Generate(ctx context.Context, request *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	continuationEnabled := false
	if request != nil {
		continuationEnabled = core.IsContinuationEnabled(c, request.Options)
	}

	if continuationEnabled {
		return c.generateViaResponses(ctx, request)
	}

	return c.generateViaChatCompletion(ctx, request)
}

func (c *Client) generateViaResponses(ctx context.Context, request *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	// Prepare request
	req, err := c.prepareChatRequest(request)
	if err != nil {
		return nil, err
	}
	payload, err := c.marshalRequestBody(req, request)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.createHTTPResponsesApiRequest(ctx, payload)
	if err != nil {
		return nil, err
	}

	// Observer start: include generic llm request as ResponsePayload JSON
	observer := mcbuf.ObserverFromContext(ctx)
	var genReqJSON []byte
	if observer != nil {
		if request != nil {
			genReqJSON, _ = json.Marshal(request)
		}
		if newCtx, obErr := observer.OnCallStart(ctx, mcbuf.Info{Provider: "openai", Model: req.Model, ModelKind: "chat", LLMRequest: request, RequestJSON: payload, Payload: genReqJSON, StartedAt: time.Now()}); obErr == nil {
			ctx = newCtx
		} else {
			return nil, fmt.Errorf("observer OnCallStart failed: %w", obErr)
		}
	}
	// Execute – honor configured timeout when provided
	if c.Timeout > 0 {
		c.HTTPClient.Timeout = c.Timeout
	} else {
		c.HTTPClient.Timeout = 10 * time.Minute
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		// Ensure model-call is finalized for cancellation/error cases
		if observer != nil {
			_ = observer.OnCallEnd(ctx, mcbuf.Info{Provider: "openai", Model: req.Model, ModelKind: "chat", CompletedAt: time.Now(), Err: err.Error()})
		}
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// Bubble continuation errors – do not fallback/summarize
		if isContinuationError(respBytes) {
			if observer != nil {
				_ = observer.OnCallEnd(ctx, mcbuf.Info{Provider: "openai", Model: req.Model, ModelKind: "chat", ResponseJSON: respBytes, CompletedAt: time.Now(), Err: "continuation error"})
			}
			return nil, fmt.Errorf("openai continuation error: %s", string(respBytes))
		}
		if observer != nil {
			_ = observer.OnCallEnd(ctx, mcbuf.Info{Provider: "openai", Model: req.Model, ModelKind: "chat", ResponseJSON: respBytes, CompletedAt: time.Now(), Err: fmt.Sprintf("status %d", resp.StatusCode)})
		}
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
		if perr != nil {
			info.Err = perr.Error()
		}

		if obErr := observer.OnCallEnd(ctx, info); obErr != nil {
			return nil, fmt.Errorf("observer OnCallEnd failed: %w", obErr)
		}
	}
	return lr, perr
}

// prepareChatRequest converts a generic request and applies client/model defaults.
func (c *Client) prepareChatRequest(request *llm.GenerateRequest) (*Request, error) {
	req, err := c.ToRequest(request)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to llm.Request: %w", err)
	}
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

// marshalRequestBody builds the request body for the OpenAI Responses API or legacy chat/completions API.
func (c *Client) marshalRequestBody(req *Request, orig *llm.GenerateRequest) ([]byte, error) {
	continuationEnabled := false
	if orig != nil {
		continuationEnabled = core.IsContinuationEnabled(c, orig.Options)
	}

	if continuationEnabled {
		return c.marshalResponsesApiRequestBody(req)
	}

	return c.marshalChatCompletionApiRequestBody(req)
}

// marshalResponsesApiRequestBody marshals a Responses API payload from Request.
func (c *Client) marshalResponsesApiRequestBody(req *Request) ([]byte, error) {
	payload := ToResponsesPayload(req)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	return data, nil
}

// marshalChatCompletionApiRequestBody marshals a legacy chat/completions payload from Request.
func (c *Client) marshalChatCompletionApiRequestBody(req *Request) ([]byte, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chat/completions request: %w", err)
	}
	return data, nil
}

func (c *Client) createHTTPResponsesApiRequest(ctx context.Context, data []byte) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/responses", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return httpReq, nil
}

func (c *Client) createHTTPChatCompletionsApiRequest(ctx context.Context, data []byte) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return httpReq, nil
}

func isContinuationError(body []byte) bool {
	msg := strings.ToLower(string(body))
	if strings.Contains(msg, "previous_response_id") && (strings.Contains(msg, "invalid") || strings.Contains(msg, "unknown")) {
		return true
	}
	if strings.Contains(msg, "no tool call found for function call output") {
		return true
	}
	if strings.Contains(msg, "function_call_output") && strings.Contains(msg, "no tool call") {
		return true
	}
	if strings.Contains(msg, "no tool output found for function call") {
		return true
	}
	return false
}

func debugOpenAIEnabled() bool { return os.Getenv("AGENTLY_DEBUG_OPENAI") == "1" }
func openaiNoFallback() bool   { return os.Getenv("AGENTLY_OPENAI_NO_FALLBACK") == "1" }

func (c *Client) generateViaChatCompletion(ctx context.Context, request *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	// Prepare request
	req, err := c.prepareChatRequest(request)
	if err != nil {
		return nil, err
	}

	// Scrub fields unsupported by chat/completions
	req.PreviousResponseID = ""
	req.Stream = false
	req.StreamOptions = nil

	payload, err := c.marshalChatCompletionApiRequestBody(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.createHTTPChatCompletionsApiRequest(ctx, payload)
	if err != nil {
		return nil, err
	}

	// Observer start: include generic llm request as ResponsePayload JSON
	observer := mcbuf.ObserverFromContext(ctx)
	var genReqJSON []byte
	if observer != nil {
		if request != nil {
			genReqJSON, _ = json.Marshal(request)
		}
		if newCtx, obErr := observer.OnCallStart(ctx, mcbuf.Info{Provider: "openai", Model: req.Model, ModelKind: "chat", LLMRequest: request, RequestJSON: payload, Payload: genReqJSON, StartedAt: time.Now()}); obErr == nil {
			ctx = newCtx
		} else {
			return nil, fmt.Errorf("observer OnCallStart (chat.completions) failed: %w", obErr)
		}
	}
	// Execute – honor configured timeout when provided
	if c.Timeout > 0 {
		c.HTTPClient.Timeout = c.Timeout
	} else {
		c.HTTPClient.Timeout = 10 * time.Minute
	}
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		// Ensure model-call is finalized for cancellation/error cases
		if observer != nil {
			_ = observer.OnCallEnd(ctx, mcbuf.Info{Provider: "openai", Model: req.Model, ModelKind: "chat", CompletedAt: time.Now(), Err: err.Error()})
		}
		return nil, fmt.Errorf("failed to send chat.completions request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read chat.completions response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		/* TODO add this Response API like error handling if needed
		// Bubble continuation errors – do not fallback/summarize
		if isContinuationError(respBytes) {
			if observer != nil {
				_ = observer.OnCallEnd(ctx, mcbuf.Info{Provider: "openai", Model: req.Model, ModelKind: "chat", ResponseJSON: respBytes, CompletedAt: time.Now(), Err: "continuation error"})
			}
			return nil, fmt.Errorf("openai continuation error: %s", string(respBytes))
		}
		*/
		if observer != nil {
			_ = observer.OnCallEnd(ctx, mcbuf.Info{Provider: "openai", Model: req.Model, ModelKind: "chat", ResponseJSON: respBytes, CompletedAt: time.Now(), Err: fmt.Sprintf("status %d", resp.StatusCode)})
		}
		return nil, fmt.Errorf("OpenAI Chat API (chat.completions) error (status %d): %s", resp.StatusCode, respBytes)
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
		if perr != nil {
			info.Err = perr.Error()
		}

		if obErr := observer.OnCallEnd(ctx, info); obErr != nil {
			return nil, fmt.Errorf("observer OnCallEnd failed (chat.completions): %w", obErr)
		}
	}
	return lr, perr
}

func (c *Client) parseGenerateResponse(model string, respBytes []byte) (*llm.GenerateResponse, error) {
	// Best‑effort: tolerate SSE-style payload delivered to non-stream path.
	// Some gateways may return a pre-buffered SSE transcript where the final
	// response is embedded in a "response.completed" data chunk.
	if bytes.Contains(respBytes, []byte("data:")) && bytes.Contains(respBytes, []byte("event:")) {
		if lr, ok := parseCompletedFromSSE(respBytes); ok {
			if c.UsageListener != nil && lr.Usage != nil && lr.Usage.TotalTokens > 0 {
				c.UsageListener.OnUsage(model, lr.Usage)
			}
			return lr, nil
		}
	}
	// Try legacy chat/completions shape first
	var apiResp Response
	if err := json.Unmarshal(respBytes, &apiResp); err == nil && (apiResp.Object != "" || len(apiResp.Choices) > 0) {
		llmResp := ToLLMSResponse(&apiResp)
		if c.UsageListener != nil && llmResp.Usage != nil && llmResp.Usage.TotalTokens > 0 {
			c.UsageListener.OnUsage(model, llmResp.Usage)
		}
		return llmResp, nil
	}

	// Try Responses API direct form
	var r2 ResponsesResponse
	if err := json.Unmarshal(respBytes, &r2); err == nil && (r2.ID != "" || len(r2.Output) > 0) {
		llmResp := ToLLMSFromResponses(&r2)
		if c.UsageListener != nil && llmResp.Usage != nil && llmResp.Usage.TotalTokens > 0 {
			c.UsageListener.OnUsage(model, llmResp.Usage)
		}
		return llmResp, nil
	}

	// Some streams may wrap final response under a "response" field
	var wrap struct {
		Response *ResponsesResponse `json:"response,omitempty"`
	}
	if err := json.Unmarshal(respBytes, &wrap); err == nil && wrap.Response != nil {
		llmResp := ToLLMSFromResponses(wrap.Response)
		if c.UsageListener != nil && llmResp.Usage != nil && llmResp.Usage.TotalTokens > 0 {
			c.UsageListener.OnUsage(model, llmResp.Usage)
		}
		return llmResp, nil
	}
	// Improve diagnostics while still bubbling error up (no stdout printing).
	snippet := string(respBytes)
	if len(snippet) > 240 {
		snippet = snippet[:240]
	}
	return nil, fmt.Errorf("failed to unmarshal response: unknown format (body=%q)", strings.TrimSpace(snippet))
}

// parseCompletedFromSSE attempts to extract a final response from a pre-buffered
// SSE transcript by locating a response.completed data JSON chunk and
// converting it to llm.GenerateResponse.
func parseCompletedFromSSE(body []byte) (*llm.GenerateResponse, bool) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	buf := make([]byte, 0, sseInitialBuf)
	scanner.Buffer(buf, sseMaxBuf)
	lastEvent := ""
	var lastData string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			lastEvent = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			continue
		}
		// Prefer response.completed, but remember last data otherwise
		lastData = data
		if lastEvent == "response.completed" {
			if lr := parseAnyFinal(data); lr != nil {
				return lr, true
			}
		}
	}
	// Fallback to the last data payload if it parses as a final response
	if lr := parseAnyFinal(lastData); lr != nil {
		return lr, true
	}
	return nil, false
}

// parseAnyFinal tries several known final object shapes from a JSON string.
func parseAnyFinal(data string) *llm.GenerateResponse {
	// Wrapped ResponsesResponse
	var w struct {
		Response *ResponsesResponse `json:"response"`
	}
	if json.Unmarshal([]byte(data), &w) == nil && w.Response != nil {
		return ToLLMSFromResponses(w.Response)
	}
	// Direct ResponsesResponse
	var r2 ResponsesResponse
	if json.Unmarshal([]byte(data), &r2) == nil && (r2.ID != "" || len(r2.Output) > 0) {
		return ToLLMSFromResponses(&r2)
	}
	// Legacy chat/completions Response
	var r1 Response
	if json.Unmarshal([]byte(data), &r1) == nil && (r1.Object != "" || len(r1.Choices) > 0) {
		return ToLLMSResponse(&r1)
	}
	return nil
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
	payload, err := c.marshalRequestBody(req, request)
	if err != nil {
		return nil, err
	}

	var httpReq *http.Request
	if core.IsContinuationEnabled(c, request.Options) {
		httpReq, err = c.createHTTPResponsesApiRequest(ctx, payload)
	} else {
		httpReq, err = c.createHTTPChatCompletionsApiRequest(ctx, payload)
	}

	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	// Observer start
	observer := mcbuf.ObserverFromContext(ctx)
	var genReqJSON []byte

	if observer != nil {
		if request != nil {
			genReqJSON, _ = json.Marshal(request)
		}
		if newCtx, obErr := observer.OnCallStart(ctx, mcbuf.Info{Provider: "openai", Model: req.Model, LLMRequest: request, ModelKind: "chat", RequestJSON: payload, Payload: genReqJSON, StartedAt: time.Now()}); obErr == nil {
			ctx = newCtx
		} else {
			return nil, fmt.Errorf("observer OnCallStart failed: %w", obErr)
		}
	}
	// Honor configured timeout for streaming as well
	if c.Timeout > 0 {
		c.HTTPClient.Timeout = c.Timeout
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		if observer != nil {
			_ = observer.OnCallEnd(ctx, mcbuf.Info{Provider: "openai", Model: req.Model, ModelKind: "chat", CompletedAt: time.Now(), Err: err.Error()})
		}
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	events := make(chan llm.StreamEvent)
	go func() {
		defer resp.Body.Close()
		defer close(events)
		proc := &streamProcessor{
			client:   c,
			ctx:      ctx,
			observer: observer,
			events:   events,
			agg:      newStreamAggregator(),
			state:    &streamState{},
			req:      req,
			orig:     request,
		}
		// Read response body
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			events <- llm.StreamEvent{Err: fmt.Errorf("failed to read response body: %w", readErr)}
			return
		}
		// If Responses stream returned an immediate JSON error and it's
		// a continuation error, bubble up an error and do not fallback.
		if !bytes.Contains(respBody, []byte("data: ")) {
			type apiErr struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			var e apiErr
			if json.Unmarshal(bytes.TrimSpace(respBody), &e) == nil && e.Error.Message != "" {
				if isContinuationError(respBody) {
					events <- llm.StreamEvent{Err: fmt.Errorf("openai continuation error: %s", string(respBody))}
					return
				}
				events <- llm.StreamEvent{Err: fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(respBody))}
				return
			}
		}
		// Normal SSE handling
		proc.respBody = respBody
		// Prepare scanner
		scanner := bufio.NewScanner(bytes.NewReader(respBody))
		buf := make([]byte, 0, sseInitialBuf)
		scanner.Buffer(buf, sseMaxBuf)
		currentEvent := ""
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ") {
				currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
				continue
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}
			if ok := proc.handleEvent(currentEvent, data); !ok {
				return
			}
		}
		proc.finalize(scanner.Err())
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
	// Always include tool calls in final aggregation when present (even if already emitted as events)
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
	// Preserve any accumulated content as assistant text
	if ca != nil && ca.content.Len() > 0 {
		msg.Content = ca.content.String()
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
			events <- llm.StreamEvent{Err: fmt.Errorf("openai error: %s (type=%s, param=%s, code=%s)", e.Error.Message, e.Error.Type, e.Error.Param, e.Error.Code)}
			// Bubble error to caller and close stream
			if proc.observer != nil && !proc.state.ended {
				if obErr := proc.observer.OnCallEnd(proc.ctx, mcbuf.Info{Provider: "openai", Model: proc.state.lastModel, ModelKind: "chat", ResponseJSON: respBody, CompletedAt: time.Now()}); obErr != nil {
					events <- llm.StreamEvent{Err: fmt.Errorf("observer OnCallEnd failed: %w", obErr)}
					return
				}
				proc.state.ended = true
			}
			return
		}
	}

	// Prepare scanner
	scanner := bufio.NewScanner(bytes.NewReader(respBody))
	buf := make([]byte, 0, sseInitialBuf)
	scanner.Buffer(buf, sseMaxBuf)

	// Scan each SSE line, track event types (Responses API)
	currentEvent := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		if ok := proc.handleEvent(currentEvent, data); !ok {
			// Stop further processing on parsing error to match previous behavior.
			return
		}
	}

	// Finalize
	proc.finalize(scanner.Err())
}
