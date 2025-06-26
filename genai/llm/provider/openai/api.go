package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/llm/provider/base"
	"io"
	"net/http"
	"strings"
	"time"
)

func (c *Client) Implements(feature string) bool {
	switch feature {
	case base.CanUseTools:
		return true
	}
	return false
}

// Generate sends a chat request to the OpenAI API and returns the response
func (c *Client) Generate(ctx context.Context, request *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	// Convert llms.GenerateRequest to Request
	req := ToRequest(request)
	// Set model from client if not specified in options
	if req.Model == "" {
		req.Model = c.Model
	}
	// If the request didn't specify temperature, apply fallback for models
	// that require a specific default different from 1.0. Currently we do
	// not override for o4-mini because omitting the field is mandatory.
	if value, ok := modelTemperature[req.Model]; ok {
		req.Temperature = &value
	}

	if req.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	// Marshal the request to JSON
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	c.HTTPClient.Timeout = 10 * time.Minute
	// Send the request
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	fmt.Println(string(respBytes))

	// Check for non-200 status code
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, respBytes)
	}

	// Unmarshal the response
	var apiResp Response
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	respLLM := ToLLMSResponse(&apiResp)
	if c.UsageListener != nil && respLLM.Usage != nil && respLLM.Usage.TotalTokens > 0 {
		c.UsageListener.OnUsage(req.Model, respLLM.Usage)
	}
	return respLLM, nil
}

// Stream sends a chat request to the OpenAI API with streaming enabled and returns a channel of partial responses.
func (c *Client) Stream(ctx context.Context, request *llm.GenerateRequest) (<-chan llm.StreamEvent, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	// Prepare request
	req := ToRequest(request)
	// Set model from client if not specified
	if req.Model == "" {
		req.Model = c.Model
	}
	// Apply default temperature for models requiring non-default
	if value, ok := modelTemperature[req.Model]; ok && req.Temperature == nil {
		req.Temperature = &value
	}
	if req.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	// Marshal the request
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	// Set headers
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	// Send the request
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	// Prepare streaming channel
	events := make(chan llm.StreamEvent)
	go func() {
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			// Stream event lines are prefixed with "data: "
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}
			var apiResp Response
			if err := json.Unmarshal([]byte(data), &apiResp); err != nil {
				events <- llm.StreamEvent{Err: fmt.Errorf("failed to unmarshal stream response: %w", err)}
				break
			}
			// Convert to generic LLM response
			events <- llm.StreamEvent{Response: ToLLMSResponse(&apiResp)}
		}
		if err := scanner.Err(); err != nil {
			events <- llm.StreamEvent{Err: fmt.Errorf("stream read error: %w", err)}
		}
		close(events)
	}()
	return events, nil
}
