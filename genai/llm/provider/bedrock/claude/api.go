package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/llm/provider/base"
	authAws "github.com/viant/scy/auth/aws"
)

func (c *Client) Implements(feature string) bool {
	switch feature {
	case base.CanUseTools:
		return true
	}
	return false
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

	// Set the Anthropic version
	req.AnthropicVersion = c.AnthropicVersion

	// Marshal the request to JSON
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the Bedrock InvokeModel request
	invokeRequest := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(c.Model),
		Body:        data,
		ContentType: aws.String("application/json"),
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

	// Convert Response to llms.GenerateResponse
	llmsResp := ToLLMSResponse(&apiResp)
	if c.UsageListener != nil && llmsResp.Usage != nil && llmsResp.Usage.TotalTokens > 0 {
		c.UsageListener.OnUsage(request.Options.Model, llmsResp.Usage)
	}
	return llmsResp, nil
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

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	input := &bedrockruntime.InvokeModelWithResponseStreamInput{
		ModelId:     aws.String(c.Model),
		Body:        data,
		ContentType: aws.String("application/json"),
	}
	output, err := c.BedrockClient.InvokeModelWithResponseStream(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke Bedrock stream model: %w", err)
	}

	events := make(chan llm.StreamEvent)
	go func() {
		es := output.GetStream()
		defer es.Close()
		for ev := range es.Events() {
			switch chunk := ev.(type) {
			case *types.ResponseStreamMemberChunk:
				var apiResp Response
				if err := json.Unmarshal(chunk.Value.Bytes, &apiResp); err != nil {
					events <- llm.StreamEvent{Err: fmt.Errorf("failed to unmarshal stream chunk: %w", err)}
					return
				}
				apiResp.Model = c.Model
				events <- llm.StreamEvent{Response: ToLLMSResponse(&apiResp)}
			default:
				// ignore unknown events
			}
		}
		if err := es.Err(); err != nil {
			events <- llm.StreamEvent{Err: err}
		}
		close(events)
	}()
	return events, nil
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
