package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/agently/genai/llm"
	"io"
	"net/http"
)

// Generate sends a chat request to the Ollama API and returns the response
func (c *Client) Generate(ctx context.Context, request *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if c.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	// Convert llms.ChatRequest to Request
	req, err := ToRequest(ctx, request, c.Model)
	if err != nil {
		return nil, err
	}
	req.Stream = true

	// Marshal the request to JSON
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/generate", c.BaseURL), bytes.NewBuffer(data))
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

	// Check for non-200 status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	apiResp := &Response{}

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			var chunk Response
			if err := json.Unmarshal(line, &chunk); err != nil {
				continue
			}
			apiResp.Response += chunk.Response
			apiResp.Context = append(apiResp.Context, chunk.Context...)
			apiResp.PromptEvalCount += chunk.PromptEvalCount
			apiResp.EvalCount += chunk.EvalCount
			apiResp.Done = chunk.Done
			apiResp.EvalDuration = chunk.EvalDuration
			apiResp.LoadDuration = chunk.LoadDuration
			apiResp.TotalDuration = chunk.TotalDuration
			apiResp.PromptEvalDuration = chunk.PromptEvalDuration
			apiResp.CreatedAt = chunk.CreatedAt
			apiResp.Model = chunk.Model
			if chunk.Done {
				break
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
	}

	// Convert Response to llms.ChatResponse
	llmsResp := ToLLMSResponse(apiResp)
	if c.UsageListener != nil && llmsResp.Usage != nil && llmsResp.Usage.TotalTokens > 0 {
		c.UsageListener.OnUsage(req.Model, llmsResp.Usage)
	}
	return llmsResp, nil
}

// Stream sends a chat request to the Ollama API with streaming enabled and returns a channel of partial responses.
func (c *Client) Stream(ctx context.Context, request *llm.GenerateRequest) (<-chan llm.StreamEvent, error) {
	if c.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	req, err := ToRequest(ctx, request, c.Model)
	if err != nil {
		return nil, err
	}
	req.Stream = true

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/generate", c.BaseURL), bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	events := make(chan llm.StreamEvent)
	go func() {
		defer resp.Body.Close()
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				var chunk Response
				if err := json.Unmarshal(line, &chunk); err != nil {
					events <- llm.StreamEvent{Err: fmt.Errorf("failed to unmarshal stream chunk: %w", err)}
					break
				}
				events <- llm.StreamEvent{Response: ToLLMSResponse(&chunk)}
				if chunk.Done {
					break
				}
			}
			if err != nil {
				if err == io.EOF {
					break
				}
				events <- llm.StreamEvent{Err: fmt.Errorf("stream read error: %w", err)}
				break
			}
		}
		close(events)
	}()
	return events, nil
}

// sendPullRequest sends a pull request to the Ollama API and returns the response
func (c *Client) sendPullRequest(ctx context.Context, request *PullRequest) (*PullResponse, error) {
	// Marshal the request to JSON
	data, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pull request: %w", err)
	}

	// Create the HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/pull", c.BaseURL), bytes.NewBuffer(data))
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

	// Check for non-200 status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Unmarshal the response
	var pullResp PullResponse
	if err := json.Unmarshal(body, &pullResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pull response: %w", err)
	}

	return &pullResp, nil
}
