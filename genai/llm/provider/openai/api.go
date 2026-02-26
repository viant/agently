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
	"github.com/viant/agently/genai/memory"
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
// If providerRespJSON is set, it is used as ResponseJSON; otherwise we marshal lr.
func endObserverOnce(observer mcbuf.Observer, ctx context.Context, model string, lr *llm.GenerateResponse, usage *llm.Usage, providerRespJSON []byte, ended *bool) error {
	if ended == nil || *ended {
		return nil
	}
	if observer != nil {
		respJSON := providerRespJSON
		var finish string
		if len(respJSON) == 0 && lr != nil {
			respJSON, _ = json.Marshal(lr)
		}
		if lr != nil {
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

type openAIErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param"`
	Code    string `json:"code"`
}

type openAIErrorEnvelope struct {
	Error    openAIErrorBody `json:"error"`
	Response struct {
		Error openAIErrorBody `json:"error"`
	} `json:"response"`
}

func parseOpenAIError(resp []byte) (string, string) {
	raw := bytes.TrimSpace(resp)
	if len(raw) == 0 {
		return "", ""
	}
	// Support both top-level error and Responses API wrapper: { response: { error: ... } }
	var w openAIErrorEnvelope
	if json.Unmarshal(raw, &w) != nil {
		return "", ""
	}
	errObj := w.Error
	if strings.TrimSpace(errObj.Message) == "" {
		errObj = w.Response.Error
	}
	msg := strings.TrimSpace(errObj.Message)
	code := strings.TrimSpace(errObj.Code)
	if msg == "" {
		return "", code
	}
	if strings.TrimSpace(errObj.Type) != "" || strings.TrimSpace(errObj.Param) != "" {
		msg = fmt.Sprintf("%s (type=%s, param=%s)", msg, strings.TrimSpace(errObj.Type), strings.TrimSpace(errObj.Param))
	}
	return msg, code
}

func endObserverErrorOnce(observer mcbuf.Observer, ctx context.Context, model string, respJSON []byte, errMsg, errCode string, ended *bool) error {
	if ended == nil || *ended {
		return nil
	}
	if observer == nil {
		return nil
	}
	info := mcbuf.Info{Provider: "openai", Model: model, ModelKind: "chat", ResponseJSON: respJSON, CompletedAt: time.Now(), Err: errMsg, ErrorCode: errCode}
	if err := observer.OnCallEnd(ctx, info); err != nil {
		return err
	}
	*ended = true
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
	case base.SupportsContextContinuation:
		// Default enabled; allow explicit disable via provider options.
		if c.ContextContinuation == nil {
			return true
		}
		return *c.ContextContinuation
	case base.SupportsInstructions:
		// Instructions are supported only when using the Responses API.
		if c.ContextContinuation == nil {
			return true
		}
		return *c.ContextContinuation
	}
	return false
}

// SupportsAnchorContinuation reports whether previous_response_id-style
// continuation should be attempted for this client.
func (c *Client) SupportsAnchorContinuation() bool {
	if c == nil || !c.Implements(base.SupportsContextContinuation) {
		return false
	}
	// chatgpt backend codex responses rejects or behaves inconsistently with
	// previous_response_id for this adapter flow; send full transcript instead.
	if isChatGPTBackendURL(c.BaseURL) {
		return false
	}
	return true
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
		continuationEnabled = core.IsContextContinuationEnabled(c)
	}

	if continuationEnabled {
		return c.generateViaResponses(ctx, request)
	}

	return c.generateViaChatCompletion(ctx, request)
}

func (c *Client) generateViaResponses(ctx context.Context, request *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	// Prepare request
	req, err := c.prepareChatRequest(request)
	if err != nil {
		return nil, err
	}
	c.applyBackendSessionDefaults(ctx, req)
	payload, err := c.marshalRequestBody(req)
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
		msg, _ := parseOpenAIError(respBytes)
		if msg == "" {
			msg = string(respBytes)
		}
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, msg)
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
func (c *Client) marshalRequestBody(req *Request) ([]byte, error) {
	if core.IsContextContinuationEnabled(c) {
		return c.marshalResponsesApiRequestBody(req)
	}

	return c.marshalChatCompletionApiRequestBody(req)
}

// marshalResponsesApiRequestBody marshals a Responses API payload from Request.
func (c *Client) marshalResponsesApiRequestBody(req *Request) ([]byte, error) {
	if isChatGPTBackendURL(c.BaseURL) {
		backendPayload := ToChatGPTBackendResponsesPayload(req)
		data, err := json.Marshal(backendPayload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal backend request: %w", err)
		}
		return data, nil
	}
	payload := ToResponsesPayload(req)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	return data, nil
}

// adaptSystemMessagesForChatGPTBackend preserves system guidance without sending
// forbidden role=system items to chatgpt.com/backend-api/codex/responses.
// Strategy:
// - fold system text into top-level instructions (Codex-style),
// - remap any remaining system message role to developer.
func adaptSystemMessagesForChatGPTBackend(payload *ResponsesPayload) {
	if payload == nil || len(payload.Input) == 0 {
		return
	}
	baseInstructions := strings.TrimSpace(payload.Instructions)
	systemChunks := make([]string, 0, 4)
	filtered := make([]InputItem, 0, len(payload.Input))

	for i := range payload.Input {
		item := &payload.Input[i]
		if strings.ToLower(strings.TrimSpace(item.Type)) != "message" {
			filtered = append(filtered, *item)
			continue
		}
		if strings.ToLower(strings.TrimSpace(item.Role)) != "system" {
			filtered = append(filtered, *item)
			continue
		}
		kept := make([]ResponsesContentItem, 0, len(item.Content))
		for _, part := range item.Content {
			// Preserve textual system guidance in instructions.
			if txt := strings.TrimSpace(part.Text); txt != "" {
				systemChunks = append(systemChunks, txt)
				continue
			}
			kept = append(kept, part)
		}
		item.Content = kept
		// Backend rejects role=system; developer is the closest semantic role.
		item.Role = "developer"
		// Drop transformed messages that ended up with no content.
		if len(item.Content) == 0 {
			continue
		}
		filtered = append(filtered, *item)
	}
	payload.Input = filtered

	if len(systemChunks) == 0 {
		return
	}
	joined := strings.TrimSpace(strings.Join(systemChunks, "\n\n"))
	if joined == "" {
		return
	}
	if baseInstructions == "" {
		payload.Instructions = joined
		return
	}
	if normalizeText(baseInstructions) == normalizeText(joined) {
		payload.Instructions = baseInstructions
		return
	}
	payload.Instructions = baseInstructions + "\n\n" + joined
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
	apiKey, err := c.apiKey(ctx)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/responses", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	if ua := c.userAgentOverride(); ua != "" {
		httpReq.Header.Set("User-Agent", ua)
	}
	if originator := c.originatorHeader(); originator != "" {
		httpReq.Header.Set("originator", originator)
	}
	if features := c.codexBetaFeaturesHeader(); features != "" {
		httpReq.Header.Set("x-codex-beta-features", features)
	}
	if isChatGPTBackendURL(c.BaseURL) {
		if accountID, err := c.chatGPTAccountID(ctx); err == nil && accountID != "" {
			httpReq.Header.Set("ChatGPT-Account-Id", accountID)
		}
		if conversationID := strings.TrimSpace(memory.ConversationIDFromContext(ctx)); conversationID != "" {
			// Codex parity: bind backend requests to a stable conversation/session identity.
			httpReq.Header.Set("session_id", conversationID)
			// Replay turn-state token captured during backend websocket handshake.
			state := getBackendWSState(c.BaseURL, conversationID)
			state.mu.Lock()
			turnState := strings.TrimSpace(state.turnState)
			state.mu.Unlock()
			if turnState != "" {
				httpReq.Header.Set("x-codex-turn-state", turnState)
			}
		}
	}
	return httpReq, nil
}

func (c *Client) createHTTPChatCompletionsApiRequest(ctx context.Context, data []byte) (*http.Request, error) {
	apiKey, err := c.apiKey(ctx)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	if ua := c.userAgentOverride(); ua != "" {
		httpReq.Header.Set("User-Agent", ua)
	}
	if isChatGPTBackendURL(c.BaseURL) {
		if accountID, err := c.chatGPTAccountID(ctx); err == nil && accountID != "" {
			httpReq.Header.Set("ChatGPT-Account-Id", accountID)
		}
	}
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
	// Prepare request
	req, err := c.prepareChatRequest(request)
	if err != nil {
		return nil, err
	}

	// Scrub fields unsupported by chat/completions
	req.PreviousResponseID = ""
	req.Stream = false
	req.StreamOptions = nil
	req.Instructions = ""
	req.PromptCacheKey = ""
	req.Text = nil

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
		msg, _ := parseOpenAIError(respBytes)
		if msg == "" {
			msg = string(respBytes)
		}
		return nil, fmt.Errorf("OpenAI Chat API (chat.completions) error (status %d): %s", resp.StatusCode, msg)
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
	continuationEnabled := core.IsContextContinuationEnabled(c)

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

	if !continuationEnabled {
		// Try legacy chat/completions shape first
		var apiResp Response
		if err := json.Unmarshal(respBytes, &apiResp); err == nil && (apiResp.Object != "" || len(apiResp.Choices) > 0) {
			llmResp := ToLLMSResponse(&apiResp)
			if c.UsageListener != nil && llmResp.Usage != nil && llmResp.Usage.TotalTokens > 0 {
				c.UsageListener.OnUsage(model, llmResp.Usage)
			}
			return llmResp, nil
		}
	} else {
		// Try Responses API direct form
		var r2 ResponsesResponse
		if err := json.Unmarshal(respBytes, &r2); err == nil && (r2.ID != "" || len(r2.Output) > 0) {
			llmResp := ToLLMSFromResponses(&r2)
			if c.UsageListener != nil && llmResp.Usage != nil && llmResp.Usage.TotalTokens > 0 {
				c.UsageListener.OnUsage(model, llmResp.Usage)
			}
			return llmResp, nil
		}
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
	// Prepare request
	req, err := c.prepareChatRequest(request)
	if err != nil {
		return nil, err
	}
	c.applyBackendSessionDefaults(ctx, req)
	req.Stream = true
	req.EnableCodeInterpreter = true
	// Ask OpenAI to include usage in the final stream event if supported
	req.StreamOptions = &StreamOptions{IncludeUsage: true}

	// Scrub fields unsupported by chat/completions when continuation is disabled.
	if !core.IsContextContinuationEnabled(c) {
		req.PreviousResponseID = ""
		req.Instructions = ""
		req.PromptCacheKey = ""
		req.Text = nil
	}
	payload, err := c.marshalRequestBody(req)
	if err != nil {
		return nil, err
	}

	var httpReq *http.Request
	if core.IsContextContinuationEnabled(c) {
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

	// ChatGPT backend websocket parity path (persistent, conversation-keyed cache).
	// Falls back to current HTTP/SSE flow on any websocket setup/stream error.
	if core.IsContextContinuationEnabled(c) && isChatGPTBackendURL(c.BaseURL) && backendWebsocketEnabled() {
		if disabled, until := isBackendWSDisabled(c.BaseURL); disabled {
			c.logf("[openai-ws] backend websocket suppressed until=%s; using HTTP/SSE", until.Format(time.RFC3339))
		} else {
			events := make(chan llm.StreamEvent)
			go func() {
				defer close(events)
				if err := c.streamViaBackendWebSocket(ctx, req, request, events); err != nil {
					c.logf("[openai-ws] backend websocket failed, fallback to HTTP/SSE: %v", err)
					if shouldDisableBackendWS(err) {
						markBackendWSDisabled(c.BaseURL, err, c)
					}
					// Start fallback stream and bridge events through this channel.
					copiedReq := *req
					copiedReq.PreviousResponseID = ""
					copiedReq.Stream = true
					copiedReq.StreamOptions = &StreamOptions{IncludeUsage: true}
					fallbackPayload, ferr := c.marshalRequestBody(&copiedReq)
					if ferr != nil {
						events <- llm.StreamEvent{Err: fmt.Errorf("websocket fallback marshal failed: %w", ferr)}
						return
					}
					fallbackHTTPReq, ferr := c.createHTTPResponsesApiRequest(ctx, fallbackPayload)
					if ferr != nil {
						events <- llm.StreamEvent{Err: fmt.Errorf("websocket fallback request build failed: %w", ferr)}
						return
					}
					fallbackHTTPReq.Header.Set("Accept", "text/event-stream")
					// Honor configured timeout for fallback stream
					if c.Timeout > 0 {
						c.HTTPClient.Timeout = c.Timeout
					}
					resp, ferr := c.HTTPClient.Do(fallbackHTTPReq)
					if ferr != nil {
						events <- llm.StreamEvent{Err: fmt.Errorf("websocket fallback send failed: %w", ferr)}
						return
					}
					defer resp.Body.Close()

					proc := &streamProcessor{
						client:   c,
						ctx:      ctx,
						observer: observer,
						events:   events,
						agg:      newStreamAggregator(),
						state:    &streamState{},
						req:      &copiedReq,
						orig:     request,
					}
					respBody, readErr := io.ReadAll(resp.Body)
					if readErr != nil {
						events <- llm.StreamEvent{Err: fmt.Errorf("websocket fallback read failed: %w", readErr)}
						return
					}
					proc.respBody = respBody
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
				}
			}()
			return events, nil
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

		if resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusGatewayTimeout {
			msg, code := parseOpenAIError(respBody)
			if msg == "" {
				msg = fmt.Sprintf("OpenAI API error (status %d): %s", resp.StatusCode, string(respBody))
			} else {
				msg = fmt.Sprintf("OpenAI API error (status %d): %s", resp.StatusCode, msg)
			}
			events <- llm.StreamEvent{Err: fmt.Errorf("%s", msg)}
			if proc.observer != nil && !proc.state.ended {
				if obErr := endObserverErrorOnce(proc.observer, proc.ctx, proc.state.lastModel, respBody, msg, code, &proc.state.ended); obErr != nil {
					events <- llm.StreamEvent{Err: fmt.Errorf("observer OnCallEnd failed: %w", obErr)}
					return
				}
			}
			return
		}

		// If Responses stream returned an immediate JSON error and it's
		// a continuation error, bubble up an error and do not fallback.
		if !bytes.Contains(respBody, []byte("data: ")) {
			if msg, code := parseOpenAIError(respBody); msg != "" {
				if isContinuationError(respBody) {
					full := fmt.Sprintf("openai continuation error: %s", msg)
					events <- llm.StreamEvent{Err: fmt.Errorf("%s", full)}
					if proc.observer != nil && !proc.state.ended {
						if obErr := endObserverErrorOnce(proc.observer, proc.ctx, proc.state.lastModel, respBody, full, code, &proc.state.ended); obErr != nil {
							events <- llm.StreamEvent{Err: fmt.Errorf("observer OnCallEnd failed: %w", obErr)}
							return
						}
					}
					return
				}
				full := fmt.Sprintf("OpenAI API error (status %d): %s", resp.StatusCode, msg)
				events <- llm.StreamEvent{Err: fmt.Errorf("%s", full)}
				if proc.observer != nil && !proc.state.ended {
					if obErr := endObserverErrorOnce(proc.observer, proc.ctx, proc.state.lastModel, respBody, full, code, &proc.state.ended); obErr != nil {
						events <- llm.StreamEvent{Err: fmt.Errorf("observer OnCallEnd failed: %w", obErr)}
						return
					}
				}
				return
			}
		}
		if resp.StatusCode == http.StatusUnauthorized {
			msg, code := parseOpenAIError(respBody)
			if msg == "" {
				msg = strings.TrimSpace(string(respBody))
			}
			full := fmt.Sprintf("OpenAI API error (status %d): %s", resp.StatusCode, msg)
			events <- llm.StreamEvent{Err: fmt.Errorf("%s", full)}
			if proc.observer != nil && !proc.state.ended {
				if obErr := endObserverErrorOnce(proc.observer, proc.ctx, proc.state.lastModel, respBody, full, code, &proc.state.ended); obErr != nil {
					events <- llm.StreamEvent{Err: fmt.Errorf("observer OnCallEnd failed: %w", obErr)}
				}
			}
			return
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

func (c *Client) applyBackendSessionDefaults(ctx context.Context, req *Request) {
	if c == nil || req == nil || !isChatGPTBackendURL(c.BaseURL) {
		return
	}
	if strings.TrimSpace(req.PromptCacheKey) != "" {
		return
	}
	if conversationID := strings.TrimSpace(memory.ConversationIDFromContext(ctx)); conversationID != "" {
		req.PromptCacheKey = conversationID
	}
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
