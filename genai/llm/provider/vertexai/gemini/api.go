package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/agently/genai/llm/provider/base"
	"io"
	"net/http"
	"strings"

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
	// Convert llm.GenerateRequest to wire request and enable streaming
	req, err := ToRequest(ctx, request)
	if err != nil {
		return nil, err
	}
	req.Stream = true

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	apiURL := fmt.Sprintf("%s/%s:generateContent?key=%s", c.BaseURL, c.Model, c.APIKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	events := make(chan llm.StreamEvent)
	go func() {
		defer resp.Body.Close()
		defer close(events)
		scanner := bufio.NewScanner(resp.Body)
		// Aggregators per choice index (Gemini typically returns a single candidate)
		type toolAgg struct {
			name string
			args map[string]interface{}
		}
		aggText := map[int]*strings.Builder{}
		aggTools := map[int][]toolAgg{}
		finish := map[int]string{}
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}
			var chunk Response
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				events <- llm.StreamEvent{Err: fmt.Errorf("failed to unmarshal stream response: %w", err)}
				return
			}
			for _, cand := range chunk.Candidates {
				idx := cand.Index
				if _, ok := aggText[idx]; !ok {
					aggText[idx] = &strings.Builder{}
				}
				// accumulate parts
				for _, p := range cand.Content.Parts {
					if p.Text != "" {
						aggText[idx].WriteString(p.Text)
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
						aggTools[idx] = append(aggTools[idx], toolAgg{name: p.FunctionCall.Name, args: args})
					}
				}
				if cand.FinishReason != "" {
					finish[idx] = cand.FinishReason
				}
			}
			// Emit only for choices that have finishReason
			if len(finish) > 0 {
				outChoices := make([]llm.Choice, 0, len(finish))
				for idx, reason := range finish {
					msg := llm.Message{Role: llm.RoleAssistant, Content: aggText[idx].String()}
					if calls, ok := aggTools[idx]; ok && len(calls) > 0 {
						tc := make([]llm.ToolCall, 0, len(calls))
						for _, t := range calls {
							tc = append(tc, llm.ToolCall{Name: t.name, Arguments: t.args})
						}
						msg.ToolCalls = tc
					}
					outChoices = append(outChoices, llm.Choice{Index: idx, Message: msg, FinishReason: reason})
				}
				events <- llm.StreamEvent{Response: &llm.GenerateResponse{Choices: outChoices, Model: c.Model}}
				// Clear finish map to avoid re-emitting
				finish = map[int]string{}
				aggText = map[int]*strings.Builder{}
				aggTools = map[int][]toolAgg{}
			}
		}
		if err := scanner.Err(); err != nil {
			events <- llm.StreamEvent{Err: fmt.Errorf("stream read error: %w", err)}
		}
	}()
	return events, nil
}
