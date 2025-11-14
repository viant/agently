package grok

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
	oai "github.com/viant/agently/genai/llm/provider/openai"
	mcbuf "github.com/viant/agently/genai/modelcallctx"
)

// Stream sends a chat request to the Grok (xAI) API with streaming enabled and returns a channel of partial responses.
func (c *Client) Stream(ctx context.Context, request *llm.GenerateRequest) (<-chan llm.StreamEvent, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	// Build OpenAI-compatible request with stream=true using existing adapter
	req := oai.ToRequest(request)
	if req.Model == "" {
		req.Model = c.Model
	}
	req.Stream = true
	modelStr := req.Model
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	// Observer start
	observer := mcbuf.ObserverFromContext(ctx)
	if observer != nil {
		var genReqJSON []byte
		if request != nil {
			genReqJSON, _ = json.Marshal(request)
		}
		if newCtx, obErr := observer.OnCallStart(ctx, mcbuf.Info{Provider: "grok", LLMRequest: request, Model: modelStr, ModelKind: "chat", RequestJSON: payload, Payload: genReqJSON, StartedAt: time.Now()}); obErr == nil {
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
			_ = observer.OnCallEnd(ctx, mcbuf.Info{Provider: "grok", Model: modelStr, ModelKind: "chat", CompletedAt: time.Now(), Err: err.Error()})
		}
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	events := make(chan llm.StreamEvent)
	go func() {
		defer resp.Body.Close()
		defer close(events)

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			events <- llm.StreamEvent{Err: fmt.Errorf("grok API error (status %d): %s", resp.StatusCode, string(body))}
			return
		}

		// Stream-processing state
		type usageDetails struct {
			TextTokens   int `json:"text_tokens"`
			AudioTokens  int `json:"audio_tokens"`
			ImageTokens  int `json:"image_tokens"`
			CachedTokens int `json:"cached_tokens"`
		}
		type usage struct {
			PromptTokens        int          `json:"prompt_tokens"`
			CompletionTokens    int          `json:"completion_tokens"`
			TotalTokens         int          `json:"total_tokens"`
			PromptTokensDetails usageDetails `json:"prompt_tokens_details"`
		}
		type delta struct {
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		}
		type choice struct {
			Index        int     `json:"index"`
			Delta        delta   `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		}
		type chunk struct {
			ID                string   `json:"id"`
			Object            string   `json:"object"`
			Created           int64    `json:"created"`
			Model             string   `json:"model"`
			Choices           []choice `json:"choices"`
			Usage             usage    `json:"usage"`
			SystemFingerprint string   `json:"system_fingerprint"`
		}

		// Aggregator for partial content per choice index
		type aggChoice struct {
			role    llm.MessageRole
			content strings.Builder
		}
		aggregations := map[int]*aggChoice{}
		getAgg := func(idx int) *aggChoice {
			if ac, ok := aggregations[idx]; ok {
				return ac
			}
			ac := &aggChoice{}
			aggregations[idx] = ac
			return ac
		}

		var lastModel string
		var lastUsage *llm.Usage
		var lastLR *llm.GenerateResponse
		var published bool

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}
			// forward raw stream chunk to observer/persistence
			if observer != nil && strings.TrimSpace(data) != "" {
				if err := observer.OnStreamDelta(ctx, []byte(data+"\n")); err != nil {
					events <- llm.StreamEvent{Err: fmt.Errorf("observer OnStreamDelta failed: %w", err)}
					return
				}
			}
			var ch chunk
			if err := json.Unmarshal([]byte(data), &ch); err != nil {
				// tolerate unrecognized payloads
				continue
			}
			if strings.TrimSpace(ch.Model) != "" {
				lastModel = strings.TrimSpace(ch.Model)
			}
			// Update usage; publish later upon finalize to prefer final cumulative values
			if ch.Usage.TotalTokens > 0 {
				u := &llm.Usage{PromptTokens: ch.Usage.PromptTokens, CompletionTokens: ch.Usage.CompletionTokens, TotalTokens: ch.Usage.TotalTokens, PromptCachedTokens: ch.Usage.PromptTokensDetails.CachedTokens}
				lastUsage = u
			}

			finalized := make([]llm.Choice, 0)
			for _, cch := range ch.Choices {
				ac := getAgg(cch.Index)
				if strings.TrimSpace(cch.Delta.Role) != "" {
					ac.role = llm.MessageRole(cch.Delta.Role)
				}
				if cch.Delta.Content != "" {
					ac.content.WriteString(cch.Delta.Content)
				}
				if cch.FinishReason != nil {
					// finalize this choice
					msg := llm.Message{}
					if ac.role != "" {
						msg.Role = ac.role
					} else {
						msg.Role = llm.RoleAssistant
					}
					if ac.content.Len() > 0 {
						msg.Content = ac.content.String()
					}
					finalized = append(finalized, llm.Choice{Index: cch.Index, Message: msg, FinishReason: *cch.FinishReason})
					delete(aggregations, cch.Index)
				}
			}
			if len(finalized) > 0 {
				lr := &llm.GenerateResponse{Choices: finalized, Model: lastModel}
				if lastUsage != nil && lastUsage.TotalTokens > 0 {
					lr.Usage = lastUsage
				}
				// Publish usage once with the latest cumulative
				c.publishUsageOnce(lastModel, lastUsage, &published)
				events <- llm.StreamEvent{Response: lr}
				lastLR = lr
			}
		}
		if err := scanner.Err(); err != nil {
			events <- llm.StreamEvent{Err: fmt.Errorf("stream error: %w", err)}
		}

		// Observer end
		if observer != nil {
			var streamTxt string
			if lastLR != nil {
				for _, ch := range lastLR.Choices {
					if strings.TrimSpace(ch.Message.Content) != "" {
						streamTxt = strings.TrimSpace(ch.Message.Content)
						break
					}
				}
			}
			if err := observer.OnCallEnd(ctx, mcbuf.Info{Provider: "grok", Model: lastModel, ModelKind: "chat", CompletedAt: time.Now(), Usage: lastUsage, LLMResponse: lastLR, StreamText: streamTxt}); err != nil {
				events <- llm.StreamEvent{Err: fmt.Errorf("observer OnCallEnd failed: %w", err)}
			}
		}
	}()
	return events, nil
}
