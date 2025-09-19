package inceptionlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/agently/genai/llm/provider/openai"
	mcbuf "github.com/viant/agently/genai/modelcallctx"
)

func (c *Client) Implements(feature string) bool {
	switch feature {
	case base.CanUseTools:
		return true
	}
	return false
}

// Generate sends a chat request to the InceptionLabs API and returns the response
func (c *Client) Generate(ctx context.Context, request *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	// Convert llms.ChatRequest to Request
	req, err := openai.ToRequest(request)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to llm.Request: %w", err)
	}
	// Set model from client if not specified in options
	if req.Model == "" {
		req.Model = c.Model
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

	// Observer start
	observer := mcbuf.ObserverFromContext(ctx)
	if observer != nil {
		var genReqJSON []byte
		if request != nil {
			genReqJSON, _ = json.Marshal(request)
		}
		if newCtx, obErr := observer.OnCallStart(ctx, mcbuf.Info{Provider: "inceptionlabs", Model: req.Model, ModelKind: "chat", RequestJSON: data, Payload: genReqJSON, StartedAt: time.Now()}); obErr == nil {
			ctx = newCtx
		} else {
			return nil, fmt.Errorf("observer OnCallStart failed: %w", obErr)
		}
	}
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
		return nil, fmt.Errorf("InceptionLabs API error (status %d): %s", resp.StatusCode, respBytes)
	}

	// Unmarshal the response
	var apiResp openai.Response
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Convert Response to llms.ChatResponse
	llmsResp := openai.ToLLMSResponse(&apiResp)
	if c.UsageListener != nil && llmsResp.Usage != nil && llmsResp.Usage.TotalTokens > 0 {
		c.UsageListener.OnUsage(req.Model, llmsResp.Usage)
	}
	if observer != nil {
		info := mcbuf.Info{Provider: "inceptionlabs", Model: req.Model, ModelKind: "chat", ResponseJSON: respBytes, CompletedAt: time.Now(), Usage: llmsResp.Usage, LLMResponse: llmsResp}
		if llmsResp != nil && len(llmsResp.Choices) > 0 {
			info.FinishReason = llmsResp.Choices[0].FinishReason
		}

		if obErr := observer.OnCallEnd(ctx, info); obErr != nil {
			return nil, fmt.Errorf("observer OnCallEnd failed: %w", obErr)
		}
	}
	return llmsResp, nil
}
