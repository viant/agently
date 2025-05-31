package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/viant/agently/genai/llm"
)

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
