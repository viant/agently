package claude

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	basecfg "github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/scy/cred/secret"
)

const (
	defaultAnthropicVersion = "bedrock-2023-05-31"
)

// Client represents a Claude API client for AWS Bedrock
type Client struct {
	BedrockClient    *bedrockruntime.Client
	MaxTokens        int
	Model            string
	AnthropicVersion string
	Config           *aws.Config
	// UsageListener receives token usage information per invocation
	UsageListener  basecfg.UsageListener
	secrets        *secret.Service
	Region         string
	MaxRetries     int
	CredentialsURL string
	AccountID      string
	Temperature    *float64
}

//    inferenceProfileId: "arn:aws:bedrock:us-west-2:458197927229:inference-profile/us.anthropic.claude-3-7-sonnet-20250219-v1:0"

// NewClient creates a new Claude client for AWS Bedrock
func NewClient(ctx context.Context, model string, options ...ClientOption) (*Client, error) {
	client := &Client{
		Model:            model,
		AnthropicVersion: defaultAnthropicVersion,
		MaxRetries:       2,
		secrets:          secret.New(),
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	if client.CredentialsURL != "" {
		cfg, err := client.loadAwsConfig(ctx)
		if err != nil {
			return nil, err
		}
		client.Config = cfg
	}

	if client.Config == nil {
		cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(client.Region))
		if err != nil {
			return nil, err
		}
		client.Config = &cfg
	}
	client.BedrockClient = bedrockruntime.NewFromConfig(*client.Config)
	return client, nil
}
