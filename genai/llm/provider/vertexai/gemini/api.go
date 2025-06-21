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
	}
	return false
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

	// Read the response body
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

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
			var chunk Response
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				events <- llm.StreamEvent{Err: fmt.Errorf("failed to unmarshal stream response: %w", err)}
				break
			}
			events <- llm.StreamEvent{Response: ToLLMSResponse(&chunk)}
		}
		if err := scanner.Err(); err != nil {
			events <- llm.StreamEvent{Err: fmt.Errorf("stream read error: %w", err)}
		}
		close(events)
	}()
	return events, nil
}
