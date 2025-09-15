package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/llm/provider/base"
	mcbuf "github.com/viant/agently/genai/modelcallctx"
	authAws "github.com/viant/scy/auth/aws"
	"strings"
	"time"
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
		ctx = observer.OnCallStart(ctx, mcbuf.Info{Provider: "bedrock/claude", Model: c.Model, ModelKind: "chat", RequestJSON: data, Payload: genReqJSON, StartedAt: time.Now()})
	}
	// Send the request to Bedrock
	var resp *bedrockruntime.InvokeModelOutput
	var invokeErr error
	for i := 0; i < max(1, c.MaxRetries); i++ {
		resp, invokeErr = c.BedrockClient.InvokeModel(ctx, invokeRequest)
		if invokeErr == nil {
			break
		}
	}

	if invokeErr != nil {
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
		observer.OnCallEnd(ctx, info)
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
		ctx = observer.OnCallStart(ctx, mcbuf.Info{Provider: "bedrock/claude", Model: c.Model, ModelKind: "chat", RequestJSON: data, Payload: genReqJSON, StartedAt: time.Now()})
	}
	output, err := c.BedrockClient.InvokeModelWithResponseStream(ctx, input)
	if err != nil {
		// Graceful fallback: return a channel that emits a single non-streaming result
		ch := make(chan llm.StreamEvent, 1)
		go func() {
			defer close(ch)
			resp, gerr := c.Generate(ctx, request)
			if gerr != nil {
				ch <- llm.StreamEvent{Err: fmt.Errorf("stream not supported, generate failed: %w", gerr)}
				return
			}
			ch <- llm.StreamEvent{Response: resp}
		}()
		return ch, nil
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

		for ev := range es.Events() {
			chunk, ok := ev.(*types.ResponseStreamMemberChunk)
			if !ok {
				continue
			}
			if observer != nil && len(chunk.Value.Bytes) > 0 {
				// Append raw JSON chunk bytes for full fidelity
				b := append([]byte{}, chunk.Value.Bytes...)
				b = append(b, '\n')
				observer.OnStreamDelta(ctx, b)
			}
			var raw map[string]interface{}
			if err := json.Unmarshal(chunk.Value.Bytes, &raw); err != nil {
				events <- llm.StreamEvent{Err: fmt.Errorf("failed to unmarshal stream chunk: %w", err)}
				return
			}
			t, _ := raw["type"].(string)
			switch t {
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
							observer.OnStreamDelta(ctx, []byte(txt))
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
					lr := &llm.GenerateResponse{Choices: []llm.Choice{{Index: 0, Message: msg, FinishReason: finishReason}}, Model: c.Model}
					if observer != nil {
						respJSON, _ := json.Marshal(lr)
						observer.OnCallEnd(ctx, mcbuf.Info{Provider: "bedrock/claude", Model: c.Model, ModelKind: "chat", ResponseJSON: respJSON, CompletedAt: time.Now(), FinishReason: finishReason, LLMResponse: lr})
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
			if lastLR != nil {
				respJSON, _ = json.Marshal(lastLR)
				if len(lastLR.Choices) > 0 {
					finishReason = lastLR.Choices[0].FinishReason
				}
			}
			observer.OnCallEnd(ctx, mcbuf.Info{Provider: "bedrock/claude", Model: c.Model, ModelKind: "chat", ResponseJSON: respJSON, CompletedAt: time.Now(), FinishReason: finishReason, LLMResponse: lastLR})
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

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
