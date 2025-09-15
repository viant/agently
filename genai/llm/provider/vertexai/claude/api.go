package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/llm/provider/base"
	mcbuf "github.com/viant/agently/genai/modelcallctx"
	"time"
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
	// Assume streaming unless known non-stream type (e.g., embeddings) – keep small blacklist.
	for _, kw := range []string{"embed", "embedding"} {
		if strings.Contains(m, kw) {
			return false
		}
	}
	return true
}

// Generate sends a chat request to the Claude API and returns the response
func (c *Client) Generate(ctx context.Context, request *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if c.ProjectID == "" {
		return nil, fmt.Errorf("project ID is required")
	}

	if c.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	// Convert llms.ChatRequest to Request
	req, err := ToRequest(ctx, request)
	if err != nil {
		return nil, err
	}
	if req.MaxTokens == 0 && c.MaxTokens > 0 {
		req.MaxTokens = c.MaxTokens
	}
	if req.Temperature == 0 && c.Temperature != nil {
		req.Temperature = *c.Temperature
	}
	// client defaults
	if req.MaxTokens == 0 && c.MaxTokens > 0 {
		req.MaxTokens = c.MaxTokens
	}
	if req.Temperature == 0 && c.Temperature != nil {
		req.Temperature = *c.Temperature
	}

	// Marshal the request to JSON
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the URL
	apiURL := c.GetEndpointURL()
	var resp *http.Response
	observer := mcbuf.ObserverFromContext(ctx)
	for i := 0; i < max(1, c.MaxRetries); i++ {
		// Observer start
		if observer != nil {
			var genReqJSON []byte
			if request != nil {
				genReqJSON, _ = json.Marshal(request)
			}
			if newCtx, obErr := observer.OnCallStart(ctx, mcbuf.Info{Provider: "vertex/claude", Model: c.Model, ModelKind: "chat", RequestJSON: data, Payload: genReqJSON, StartedAt: time.Now()}); obErr == nil {
				ctx = newCtx
			} else {
				return nil, fmt.Errorf("observer OnCallStart failed: %w", obErr)
			}
		}
		resp, err = c.sendRequest(ctx, apiURL, data)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusOK, http.StatusNotFound:
			break
		}
	}

	//fmt.Printf("req: %s\n", string(data))
	// Read the response body
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	//fmt.Printf("resp: %s\n", string(respBytes))

	// Check for non-200 status code
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Claude API error (status %d): %s", resp.StatusCode, respBytes)
	}

	// For streaming responses, we need to handle the response differently
	if strings.Contains(string(respBytes), "\n") {
		return handleStreamingResponse(respBytes)
	}

	// Try to unmarshal as VertexAI response first
	var vertexResp VertexAIResponse
	if err := json.Unmarshal(respBytes, &vertexResp); err == nil && vertexResp.ID != "" {
		// Successfully unmarshaled as VertexAI response
		llmsResp := VertexAIResponseToLLMS(&vertexResp)
		if c.UsageListener != nil && llmsResp.Usage != nil && llmsResp.Usage.TotalTokens > 0 {
			c.UsageListener.OnUsage(request.Options.Model, llmsResp.Usage)
		}
		if observer != nil {
			info := mcbuf.Info{Provider: "vertex/claude", Model: c.Model, ModelKind: "chat", ResponseJSON: respBytes, CompletedAt: time.Now(), Usage: llmsResp.Usage, LLMResponse: llmsResp}
			if llmsResp != nil && len(llmsResp.Choices) > 0 {
				info.FinishReason = llmsResp.Choices[0].FinishReason
			}
			observer.OnCallEnd(ctx, info)
		}
		return llmsResp, nil
	}

	// Fall back to standard response format
	var apiResp Response
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Convert Response to llms.ChatResponse
	llmsResp := ToLLMSResponse(&apiResp)
	if c.UsageListener != nil && llmsResp.Usage != nil && llmsResp.Usage.TotalTokens > 0 {
		c.UsageListener.OnUsage(request.Options.Model, llmsResp.Usage)
	}
	if observer != nil {
		info := mcbuf.Info{Provider: "vertex/claude", Model: c.Model, ModelKind: "chat", ResponseJSON: respBytes, CompletedAt: time.Now(), Usage: llmsResp.Usage, LLMResponse: llmsResp}
		if len(llmsResp.Choices) > 0 {
			info.FinishReason = llmsResp.Choices[0].FinishReason
		}
		observer.OnCallEnd(ctx, info)
	}
	return llmsResp, nil
}

func (c *Client) sendRequest(ctx context.Context, apiURL string, data []byte) (*http.Response, error) {
	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")

	// Send the request
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	return resp, nil
}

// Stream sends a chat request to the Claude API with streaming enabled and returns a channel of partial responses.
func (c *Client) Stream(ctx context.Context, request *llm.GenerateRequest) (<-chan llm.StreamEvent, error) {
	if c.ProjectID == "" {
		return nil, fmt.Errorf("project ID is required")
	}
	if c.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	// Prepare request
	req, err := ToRequest(ctx, request)
	if err != nil {
		return nil, err
	}
	// Vertex AI Claude requires max_tokens on streaming requests.
	if req.MaxTokens == 0 {
		if c.MaxTokens > 0 {
			req.MaxTokens = c.MaxTokens
		} else {
			return nil, fmt.Errorf("streaming requires max_tokens: set Options.MaxTokens or client WithMaxTokens")
		}
	}
	if req.Temperature == 0 && c.Temperature != nil {
		req.Temperature = *c.Temperature
	}
	// Marshal request
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	// Send HTTP request
	apiURL := c.GetEndpointURL()
	events := make(chan llm.StreamEvent)

	observer := mcbuf.ObserverFromContext(ctx)
	if observer != nil {
		var genReqJSON []byte
		if request != nil {
			genReqJSON, _ = json.Marshal(request)
		}
		if newCtx, obErr := observer.OnCallStart(ctx, mcbuf.Info{Provider: "vertex/claude", Model: c.Model, ModelKind: "chat", RequestJSON: data, Payload: genReqJSON, StartedAt: time.Now()}); obErr == nil {
			ctx = newCtx
		} else {
			return nil, fmt.Errorf("observer OnCallStart failed: %w", obErr)
		}
	}
	resp, err := c.sendRequest(ctx, apiURL, data)
	if err != nil {
		events <- llm.StreamEvent{Err: err}
		close(events)
		return events, err
	}
	// Stream response body with aggregation and finish-only emission
	go func() {
		defer resp.Body.Close()
		defer close(events)
		scanner := bufio.NewScanner(resp.Body)
		type toolAgg struct {
			id, name  string
			json      string
			completed bool
			emitted   bool
		}
		aggText := strings.Builder{}
		tools := map[int]*toolAgg{}
		finishReason := ""
		var usage *llm.Usage
		var promptTokens, completionTokens int
		ended := false
		emit := func(lr *llm.GenerateResponse) {
			if lr != nil {
				events <- llm.StreamEvent{Response: lr}
			}
		}
		// endObserverOnce removed; directly call OnCallEnd when final response is assembled.

		emitToolsIfAny := func() {
			idxs := make([]int, 0, len(tools))
			for i, ta := range tools {
				if ta != nil && ta.completed && !ta.emitted {
					idxs = append(idxs, i)
				}
			}
			if len(idxs) == 0 {
				return
			}
			for i := 1; i < len(idxs); i++ {
				j := i
				for j > 0 && idxs[j-1] > idxs[j] {
					idxs[j-1], idxs[j] = idxs[j], idxs[j-1]
					j--
				}
			}
			calls := make([]llm.ToolCall, 0, len(idxs))
			for _, i := range idxs {
				ta := tools[i]
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(ta.json), &args); err != nil {
					args = map[string]interface{}{"raw": ta.json}
				}
				calls = append(calls, llm.ToolCall{ID: ta.id, Name: ta.name, Arguments: args})
				ta.emitted = true
			}
			msg := llm.Message{Role: llm.RoleAssistant, Content: aggText.String(), ToolCalls: calls}
			lr := &llm.GenerateResponse{Choices: []llm.Choice{{Index: 0, Message: msg, FinishReason: finishReason}}, Model: c.Model}
			events <- llm.StreamEvent{Response: lr}
		}
		var lastLR *llm.GenerateResponse
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			if strings.HasPrefix(line, "event:") {
				continue
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" {
				continue
			}
			if observer != nil {
				observer.OnStreamDelta(ctx, []byte(payload+"\n"))
			}
			var evt Response
			if err := json.Unmarshal([]byte(payload), &evt); err != nil {
				events <- llm.StreamEvent{Err: fmt.Errorf("failed to unmarshal stream part: %w", err)}
				return
			}
			switch evt.Type {
			case "ping":
				continue
			case "content_block_start":
				if evt.ContentBlock != nil && evt.ContentBlock.Type == "tool_use" {
					tools[evt.Index] = &toolAgg{id: evt.ContentBlock.ID, name: evt.ContentBlock.Name}
				}
			case "content_block_delta":
				if evt.Delta != nil {
					if evt.Delta.Text != "" {
						aggText.WriteString(evt.Delta.Text)
						if observer != nil {
							observer.OnStreamDelta(ctx, []byte(evt.Delta.Text))
						}
					}
					if evt.Delta.PartialJSON != "" {
						if ta, ok := tools[evt.Index]; ok {
							ta.json += evt.Delta.PartialJSON
						}
					}
				}
			case "content_block_stop":
				if ta, ok := tools[evt.Index]; ok && ta != nil {
					ta.completed = true
					emitToolsIfAny()
				}
			case "message_delta":
				if evt.Delta != nil && evt.Delta.StopReason != "" {
					finishReason = evt.Delta.StopReason
				}
				if evt.Usage != nil {
					// VertexAI streams output_tokens incrementally here
					completionTokens += evt.Usage.OutputTokens
				}
			case "message_stop":
				usage = &llm.Usage{PromptTokens: promptTokens, CompletionTokens: completionTokens, TotalTokens: promptTokens + completionTokens}
				if c.UsageListener != nil {
					c.UsageListener.OnUsage(c.Model, usage)
				}
				msg := llm.Message{Role: llm.RoleAssistant, Content: aggText.String()}
				if len(tools) > 0 {
					// include all tool calls aggregated so far, in index order
					idxs := make([]int, 0, len(tools))
					for i := range tools {
						idxs = append(idxs, i)
					}
					for i := 1; i < len(idxs); i++ {
						j := i
						for j > 0 && idxs[j-1] > idxs[j] {
							idxs[j-1], idxs[j] = idxs[j], idxs[j-1]
							j--
						}
					}
					calls := make([]llm.ToolCall, 0, len(idxs))
					for _, i := range idxs {
						ta := tools[i]
						var args map[string]interface{}
						if err := json.Unmarshal([]byte(ta.json), &args); err != nil {
							args = map[string]interface{}{"raw": ta.json}
						}
						calls = append(calls, llm.ToolCall{ID: ta.id, Name: ta.name, Arguments: args})
					}
					msg.ToolCalls = calls
				}
				lr := &llm.GenerateResponse{Choices: []llm.Choice{{Index: 0, Message: msg, FinishReason: finishReason}}, Model: c.Model, Usage: usage}
				if observer != nil {
					respJSON, _ := json.Marshal(lr)
					observer.OnCallEnd(ctx, mcbuf.Info{Provider: "vertex/claude", Model: c.Model, ModelKind: "chat", ResponseJSON: respJSON, CompletedAt: time.Now(), Usage: usage, FinishReason: finishReason, LLMResponse: lr})
					ended = true
				}
				emit(lr)
				lastLR = lr
			case "message_start":
				// read prompt tokens from nested message.usage if available
				type msgStart struct {
					Message struct {
						Usage *Usage `json:"usage"`
					} `json:"message"`
				}
				var ms msgStart
				if err := json.Unmarshal([]byte(payload), &ms); err == nil && ms.Message.Usage != nil {
					promptTokens = ms.Message.Usage.InputTokens
					completionTokens += ms.Message.Usage.OutputTokens
				}
			case "error":
				if evt.Error != nil && evt.Error.Message != "" {
					events <- llm.StreamEvent{Err: fmt.Errorf("claude stream error: %s", evt.Error.Message)}
					return
				}
			default:
				// ignore others: message_start, content_block_stop, message_stop
			}
		}
		if err := scanner.Err(); err != nil {
			events <- llm.StreamEvent{Err: fmt.Errorf("stream read error: %w", err)}
		}
		if !ended && observer != nil {
			var respJSON []byte
			var finishReason string
			if lastLR != nil {
				respJSON, _ = json.Marshal(lastLR)
				if len(lastLR.Choices) > 0 {
					finishReason = lastLR.Choices[0].FinishReason
				}
			}
			observer.OnCallEnd(ctx, mcbuf.Info{Provider: "vertex/claude", Model: c.Model, ModelKind: "chat", ResponseJSON: respJSON, CompletedAt: time.Now(), Usage: usage, FinishReason: finishReason, LLMResponse: lastLR})
		}
	}()
	return events, nil
}

// helper for integer index
func vtxInt(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case float64:
			return int(t)
		case int:
			return t
		}
	}
	return 0
}

// handleStreamingResponse processes a streaming response from the Claude API
func handleStreamingResponse(respBytes []byte) (*llm.GenerateResponse, error) {
	// Split the response by newlines to get individual JSON objects
	parts := strings.Split(string(respBytes), "\n")

	var fullText string

	for _, part := range parts {
		if part == "" {
			continue
		}

		var resp Response
		if err := json.Unmarshal([]byte(part), &resp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal streaming response part: %w", err)
		}

		// Handle different response types
		if resp.Type == "message_delta" && resp.Delta != nil && resp.Delta.Type == "text_delta" {
			fullText += resp.Delta.Text
		} else if resp.Type == "message_stop" {
			// EndedAt of the stream
			break
		} else if resp.Type == "error" && resp.Error != nil {
			return nil, fmt.Errorf("Claude API streaming error: %s", resp.Error.Message)
		}
	}

	// Create a response with the accumulated text
	return &llm.GenerateResponse{
		Choices: []llm.Choice{
			{
				Index: 0,
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: fullText,
					Items: []llm.ContentItem{
						{
							Type:   llm.ContentTypeText,
							Source: llm.SourceRaw,
							Data:   fullText,
							Text:   fullText,
						},
					},
				},
				FinishReason: "stop",
			},
		},
	}, nil
}
