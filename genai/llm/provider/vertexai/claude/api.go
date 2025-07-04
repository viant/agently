package claude

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
)

func (c *Client) Implements(feature string) bool {
	switch feature {
	case base.CanUseTools:
		return true
	}
	return false
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
	for i := 0; i < max(1, c.MaxRetries); i++ {
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
	// Marshal request
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	// Send HTTP request
	apiURL := c.GetEndpointURL()
	resp, err := c.sendRequest(ctx, apiURL, data)
	if err != nil {
		return nil, err
	}
	// Stream response body
	events := make(chan llm.StreamEvent)
	go func() {
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			part := scanner.Text()
			if part == "" {
				continue
			}
			var chunk Response
			if err := json.Unmarshal([]byte(part), &chunk); err != nil {
				events <- llm.StreamEvent{Err: fmt.Errorf("failed to unmarshal stream part: %w", err)}
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
			// End of the stream
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
