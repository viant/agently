package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/llm/provider/base"
	mcbuf "github.com/viant/agently/genai/modelcallctx"
	authAws "github.com/viant/scy/auth/aws"
)

func (c *Client) Implements(feature string) bool {
	switch feature {
	case base.CanUseTools:
		return true
	case base.CanStream:
		return c.canStream()
	case base.IsMultimodal:
		return true
	case base.SupportsContextContinuation:
		return false
	case base.SupportsInstructions:
		return true
	}
	return false
}

// canStream returns whether this model supports streaming. By default we assume
// models can stream unless they match a known non-streaming category.
func (c *Client) canStream() bool {
	model := strings.ToLower(c.Model)
	// Known non-streaming categories on Bedrock include embeddings and image generators.
	blacklist := []string{"embed", "embedding", "image"}
	for _, kw := range blacklist {
		if strings.Contains(model, kw) {
			return false
		}
	}
	return true
}

// AdviseBackoff implements llm.BackoffAdvisor. It suggests a provider-specific
// retry/backoff when AWS Bedrock returns throttling errors. Per service guidance,
// we wait 30s before retrying on ThrottlingException.
func (c *Client) AdviseBackoff(err error, attempt int) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}
	// Specific modeled throttling error
	var throttling *types.ThrottlingException
	if errors.As(err, &throttling) {
		return 30 * time.Second, true
	}
	// Smithy API error code check
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := strings.ToLower(apiErr.ErrorCode())
		if strings.Contains(code, "throttling") || code == "throttlingexception" {
			return 30 * time.Second, true
		}
	}
	// String fallback (defensive)
	if msg := strings.ToLower(err.Error()); strings.Contains(msg, "throttlingexception") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "too many requests") {
		return 30 * time.Second, true
	}
	return 0, false
}

// Generate sends a chat request to the Claude API on AWS Bedrock and returns the response
func (c *Client) Generate(ctx context.Context, request *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if c.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	// Convert llms.GenerateRequest to Request
	req, err := ToRequest(ctx, request)
	if err != nil {
		return nil, err
	}

	model := c.Model
	if strings.Contains(model, "${AccountId}") {
		err = c.ensureAccountID(ctx)
		if err != nil {
			return nil, err
		}
		model = strings.ReplaceAll(model, "${AccountId}", c.AccountID)
	}

	// Set the Anthropic version
	req.AnthropicVersion = c.AnthropicVersion
	if req.MaxTokens == 0 {
		req.MaxTokens = c.MaxTokens
	}
	// Clamp to provider-safe maximum to avoid ValidationException
	if req.MaxTokens > 200000 {
		req.MaxTokens = 200000
	}
	if req.Temperature == 0 && c.Temperature != nil {
		req.Temperature = *c.Temperature
	}
	if req.Temperature == 0 && c.Temperature != nil {
		req.Temperature = *c.Temperature
	}

	// Marshal the request to JSON
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the Bedrock InvokeModel request
	invokeRequest := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(model),
		Body:        data,
		ContentType: aws.String("application/json"),
	}

	//	fmt.Printf("req: %v\n", string(data))

	// Observer start
	observer := mcbuf.ObserverFromContext(ctx)
	if observer != nil {
		var genReqJSON []byte
		if request != nil {
			genReqJSON, _ = json.Marshal(request)
		}
		if newCtx, obErr := observer.OnCallStart(ctx, mcbuf.Info{Provider: "bedrock/claude", Model: c.Model, ModelKind: "chat", LLMRequest: request, RequestJSON: data, Payload: genReqJSON, StartedAt: time.Now()}); obErr == nil {
			ctx = newCtx
		} else {
			return nil, fmt.Errorf("observer OnCallStart failed: %w", obErr)
		}
	}
	// Send the request to Bedrock
	var resp *bedrockruntime.InvokeModelOutput
	var invokeErr error

	resp, invokeErr = c.BedrockClient.InvokeModel(ctx, invokeRequest)
	if invokeErr != nil {
		// Ensure model-call is finalized for cancellation/error cases
		if observer != nil {
			_ = observer.OnCallEnd(ctx, mcbuf.Info{Provider: "bedrock/claude", Model: c.Model, ModelKind: "chat", CompletedAt: time.Now(), Err: invokeErr.Error()})
		}

		return nil, fmt.Errorf("failed to invoke Bedrock model: %w", invokeErr)
	}

	// Unmarshal the response
	var apiResp Response
	if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Set the model name in the response
	apiResp.Model = c.Model

	//fmt.Printf("resp: %v\n", string(resp.Body))

	// Convert Response to llms.GenerateResponse
	llmsResp := ToLLMSResponse(&apiResp)
	if c.UsageListener != nil && llmsResp.Usage != nil && llmsResp.Usage.TotalTokens > 0 {
		c.UsageListener.OnUsage(request.Options.Model, llmsResp.Usage)
	}
	if observer != nil {
		info := mcbuf.Info{Provider: "bedrock/claude", Model: c.Model, ModelKind: "chat", ResponseJSON: resp.Body, CompletedAt: time.Now(), Usage: llmsResp.Usage, LLMResponse: llmsResp}
		if llmsResp != nil && len(llmsResp.Choices) > 0 {
			info.FinishReason = llmsResp.Choices[0].FinishReason
		}

		if obErr := observer.OnCallEnd(ctx, info); obErr != nil {
			return nil, fmt.Errorf("observer OnCallEnd failed: %w", obErr)
		}
	}
	return llmsResp, nil
}

func (c *Client) ensureAccountID(ctx context.Context) error {
	if c.AccountID != "" {
		return nil
	}
	cfg, err := c.loadAwsConfig(ctx)
	if err != nil {
		return err
	}
	stsClient := sts.NewFromConfig(*cfg)
	output, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return err
	}
	c.AccountID = *output.Account
	return nil
}

// Stream sends a chat request to the Claude API on AWS Bedrock with streaming enabled
// and returns a channel of partial responses.
func (c *Client) Stream(ctx context.Context, request *llm.GenerateRequest) (<-chan llm.StreamEvent, error) {
	if c.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	req, err := ToRequest(ctx, request)
	if err != nil {
		return nil, err
	}
	req.AnthropicVersion = c.AnthropicVersion
	if req.MaxTokens == 0 {
		req.MaxTokens = c.MaxTokens
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 8192
	}
	if req.MaxTokens > 200000 {
		req.MaxTokens = 200000
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Resolve account placeholder if present (align with Generate)
	modelID := c.Model
	if strings.Contains(modelID, "${AccountId}") {
		if err := c.ensureAccountID(ctx); err != nil {
			return nil, err
		}
		modelID = strings.ReplaceAll(modelID, "${AccountId}", c.AccountID)
	}

	input := &bedrockruntime.InvokeModelWithResponseStreamInput{
		ModelId:     aws.String(modelID),
		Body:        data,
		ContentType: aws.String("application/json"),
	}
	observer := mcbuf.ObserverFromContext(ctx)
	if observer != nil {
		var genReqJSON []byte
		if request != nil {
			genReqJSON, _ = json.Marshal(request)
		}
		if newCtx, obErr := observer.OnCallStart(ctx, mcbuf.Info{Provider: "bedrock/claude", LLMRequest: request, Model: c.Model, ModelKind: "chat", RequestJSON: data, Payload: genReqJSON, StartedAt: time.Now()}); obErr == nil {
			ctx = newCtx
		} else {
			return nil, fmt.Errorf("observer OnCallStart failed: %w", obErr)
		}
	}
	output, err := c.BedrockClient.InvokeModelWithResponseStream(ctx, input)
	if err != nil {
		if observer != nil {
			_ = observer.OnCallEnd(ctx, mcbuf.Info{Provider: "bedrock/claude", Model: c.Model, ModelKind: "chat", CompletedAt: time.Now(), Err: err.Error()})
		}
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	events := make(chan llm.StreamEvent)
	go func() {
		es := output.GetStream()
		defer es.Close()
		defer close(events)
		var lastLR *llm.GenerateResponse
		ended := false
		emit := func(lr *llm.GenerateResponse) {
			if lr != nil {
				events <- llm.StreamEvent{Response: lr}
			}
		}
		// endObserverOnce removed; directly call OnCallEnd when final response is assembled.

		// Aggregator for Claude streaming events
		type toolAgg struct {
			id, name string
			json     string
		}
		aggText := strings.Builder{}
		tools := map[int]*toolAgg{}
		finishReason := ""
		// Aggregate token usage from streaming events
		var aggregatedUsage *llm.Usage

		for ev := range es.Events() {
			chunk, ok := ev.(*types.ResponseStreamMemberChunk)
			if !ok {
				continue
			}
			var raw map[string]interface{}
			if err := json.Unmarshal(chunk.Value.Bytes, &raw); err != nil {
				events <- llm.StreamEvent{Err: fmt.Errorf("failed to unmarshal stream chunk: %w", err)}
				return
			}
			t, _ := raw["type"].(string)
			switch t {
			case "message_start":
				// Extract usage from message_start event
				if msg, ok := raw["message"].(map[string]interface{}); ok {
					if usageMap, ok := msg["usage"].(map[string]interface{}); ok {
						aggregatedUsage = &llm.Usage{}
						if inputTokens, ok := usageMap["input_tokens"].(float64); ok {
							aggregatedUsage.PromptTokens = int(inputTokens)
						}
						if outputTokens, ok := usageMap["output_tokens"].(float64); ok {
							aggregatedUsage.CompletionTokens = int(outputTokens)
						}
						aggregatedUsage.TotalTokens = aggregatedUsage.PromptTokens + aggregatedUsage.CompletionTokens
					}
				}
			case "content_block_start":
				// Tool use start carries content_block with name/id
				if cb, ok := raw["content_block"].(map[string]interface{}); ok {
					if cb["type"] == "tool_use" {
						index := intFromMap(raw, "index")
						id, _ := cb["id"].(string)
						name, _ := cb["name"].(string)
						tools[index] = &toolAgg{id: id, name: name}
					}
				}
			case "content_block_delta":
				index := intFromMap(raw, "index")
				if delta, ok := raw["delta"].(map[string]interface{}); ok {
					// Text delta
					if txt, _ := delta["text"].(string); txt != "" {
						aggText.WriteString(txt)
						if observer != nil {
							if obErr := observer.OnStreamDelta(ctx, []byte(txt)); obErr != nil {
								events <- llm.StreamEvent{Err: fmt.Errorf("observer OnStreamDelta failed: %w", obErr)}
								return
							}
						}
					}
					// Tool input partial JSON delta
					if part, _ := delta["partial_json"].(string); part != "" {
						if ta, ok := tools[index]; ok {
							ta.json += part
						}
					}
				}
			case "message_delta":
				// Update usage from message_delta if present
				if usageMap, ok := raw["usage"].(map[string]interface{}); ok {
					if outputTokens, ok := usageMap["output_tokens"].(float64); ok {
						if aggregatedUsage != nil {
							aggregatedUsage.CompletionTokens = int(outputTokens)
							aggregatedUsage.TotalTokens = aggregatedUsage.PromptTokens + aggregatedUsage.CompletionTokens
						}
					}
				}
				if delta, ok := raw["delta"].(map[string]interface{}); ok {
					if sr, _ := delta["stop_reason"].(string); sr != "" {
						finishReason = sr
					}
				}
				// When stop reason arrives, emit a single aggregated event
				if finishReason != "" {
					msg := llm.Message{Role: llm.RoleAssistant, Content: aggText.String()}
					// Build tool calls in order of index
					if len(tools) > 0 {
						// gather keys
						idxs := make([]int, 0, len(tools))
						for i := range tools {
							idxs = append(idxs, i)
						}
						// simple insertion sort
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
					lr := &llm.GenerateResponse{
						Choices: []llm.Choice{{Index: 0, Message: msg, FinishReason: finishReason}},
						Model:   c.Model,
						Usage:   aggregatedUsage,
					}
					// Call UsageListener if configured; use client model id to avoid nil options deref
					if c.UsageListener != nil && lr.Usage != nil && lr.Usage.TotalTokens > 0 {
						c.UsageListener.OnUsage(c.Model, lr.Usage)
					}
					if observer != nil {
						respJSON, _ := json.Marshal(lr)
						if obErr := observer.OnCallEnd(ctx, mcbuf.Info{Provider: "bedrock/claude", Model: c.Model, ModelKind: "chat", ResponseJSON: respJSON, CompletedAt: time.Now(), FinishReason: finishReason, Usage: lr.Usage, LLMResponse: lr}); obErr != nil {
							events <- llm.StreamEvent{Err: fmt.Errorf("observer OnCallEnd failed: %w", obErr)}
							return
						}
						ended = true
					}
					lastLR = lr
					emit(lr)
				}
			default:
				// ignore other types
			}
		}
		if err := es.Err(); err != nil {
			events <- llm.StreamEvent{Err: err}
		}
		if !ended && observer != nil {
			var respJSON []byte
			var finishReason string
			var usage *llm.Usage
			var llmr *llm.GenerateResponse
			if lastLR != nil {
				llmr = lastLR
				usage = lastLR.Usage
				respJSON, _ = json.Marshal(lastLR)
				if len(lastLR.Choices) > 0 {
					finishReason = lastLR.Choices[0].FinishReason
				}
			}
			if obErr := observer.OnCallEnd(ctx, mcbuf.Info{Provider: "bedrock/claude", Model: c.Model, ModelKind: "chat", ResponseJSON: respJSON, CompletedAt: time.Now(), FinishReason: finishReason, Usage: usage, LLMResponse: llmr}); obErr != nil {
				events <- llm.StreamEvent{Err: fmt.Errorf("observer OnCallEnd failed: %w", obErr)}
				return
			}
		}
	}()
	return events, nil
}

// helper to read integer index fields that may be float64 from JSON
func intFromMap(m map[string]interface{}, key string) int {
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

func (c *Client) loadAwsConfig(ctx context.Context) (*aws.Config, error) {
	var awsConfig *aws.Config
	if c.CredentialsURL != "" {
		generic, err := c.secrets.GetCredentials(ctx, c.CredentialsURL)
		if err != nil {
			return nil, err
		}
		if awsConfig, err = authAws.NewConfig(ctx, &generic.Aws); err != nil {
			return nil, err
		}
	}
	if awsConfig == nil {
		var err error
		defaultConfig, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return nil, err
		}
		awsConfig = &defaultConfig
	}
	return awsConfig, nil
}
