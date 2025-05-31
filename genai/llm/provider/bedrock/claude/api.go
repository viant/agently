package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/viant/agently/genai/llm"
	authAws "github.com/viant/scy/auth/aws"
)

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
