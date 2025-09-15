package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/agently/genai/llm"
	mcbuf "github.com/viant/agently/genai/modelcallctx"
	"io"
	"net/http"
	"strings"
	"time"
)

func (c *Client) Implements(feature string) bool {
	return false
}

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

	// Observer start
	observer := mcbuf.ObserverFromContext(ctx)
	if observer != nil {
		var genReqJSON []byte
		if request != nil {
			genReqJSON, _ = json.Marshal(request)
		}
		ctx = observer.OnCallStart(ctx, mcbuf.Info{Provider: "ollama", Model: req.Model, ModelKind: "chat", RequestJSON: data, Payload: genReqJSON, StartedAt: time.Now()})
	}
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
	if observer != nil {
		// We don't have the raw concatenated resp at this point; marshal the API response.
		if b, _ := json.Marshal(apiResp); len(b) > 0 {
			info := mcbuf.Info{Provider: "ollama", Model: req.Model, ModelKind: "chat", ResponseJSON: b, CompletedAt: time.Now(), Usage: llmsResp.Usage, LLMResponse: llmsResp}
			if llmsResp != nil && len(llmsResp.Choices) > 0 {
				info.FinishReason = llmsResp.Choices[0].FinishReason
			}
			observer.OnCallEnd(ctx, info)
		} else {
			observer.OnCallEnd(ctx, mcbuf.Info{Provider: "ollama", Model: req.Model, ModelKind: "chat", CompletedAt: time.Now(), Usage: llmsResp.Usage})
		}
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

	observer := mcbuf.ObserverFromContext(ctx)
	if observer != nil {
		var genReqJSON []byte
		if request != nil {
			genReqJSON, _ = json.Marshal(request)
		}
		ctx = observer.OnCallStart(ctx, mcbuf.Info{Provider: "ollama", Model: req.Model, ModelKind: "chat", RequestJSON: data, Payload: genReqJSON, StartedAt: time.Now()})
	}
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	events := make(chan llm.StreamEvent)
	go func() {
		defer resp.Body.Close()
		reader := bufio.NewReader(resp.Body)
		ended := false
		emit := func(lr *llm.GenerateResponse) {
			if lr != nil {
				events <- llm.StreamEvent{Response: lr}
			}
		}
		endObserverOnce := func(lr *llm.GenerateResponse) {
			if ended {
				return
			}
			if observer != nil {
				respJSON, _ := json.Marshal(lr)
				var finish string
				if lr != nil && len(lr.Choices) > 0 {
					finish = lr.Choices[0].FinishReason
				}
				observer.OnCallEnd(ctx, mcbuf.Info{Provider: "ollama", Model: req.Model, ModelKind: "chat", ResponseJSON: respJSON, CompletedAt: time.Now(), FinishReason: finish})
				ended = true
			}
		}
		var lastLR *llm.GenerateResponse
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				if observer != nil {
					// append raw line for full fidelity
					observer.OnStreamDelta(ctx, line)
				}
				var chunk Response
				if err := json.Unmarshal(line, &chunk); err != nil {
					events <- llm.StreamEvent{Err: fmt.Errorf("failed to unmarshal stream chunk: %w", err)}
					break
				}
				lr := ToLLMSResponse(&chunk)
				lastLR = lr
				// Emit delta to observer when plain text is present
				if observer != nil {
					// Extract plain text content from first choice
					if lr != nil && len(lr.Choices) > 0 {
						if txt := strings.TrimSpace(lr.Choices[0].Message.Content); txt != "" {
							observer.OnStreamDelta(ctx, []byte(txt))
						}
					}
				}
				if chunk.Done {
					endObserverOnce(lr)
					emit(lr)
					break
				}
				emit(lr)
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
		if !ended {
			endObserverOnce(lastLR)
		}
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
