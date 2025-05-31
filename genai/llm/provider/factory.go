package provider

import (
	"context"
	"fmt"
	"github.com/viant/agently/genai/llm"
	bedrockclaude "github.com/viant/agently/genai/llm/provider/bedrock/claude"
	"github.com/viant/agently/genai/llm/provider/inceptionlabs"
	"github.com/viant/agently/genai/llm/provider/ollama"
	"github.com/viant/agently/genai/llm/provider/openai"
	vertexaiclaude "github.com/viant/agently/genai/llm/provider/vertexai/claude"
	"github.com/viant/agently/genai/llm/provider/vertexai/gemini"
	"github.com/viant/scy/cred/secret"
)

type Factory struct {
	secrets *secret.Service
	// CreateModel creates a new language model instance
}

func (f *Factory) CreateModel(ctx context.Context, options *Options) (llm.Model, error) {
	if options.Provider == "" {
		return nil, fmt.Errorf("provider was empty")
	}
	switch options.Provider {
	case ProviderOpenAI:
		apiKey, err := f.apiKey(ctx, options.APIKeyURL)
		if err != nil {
			return nil, err
		}
		return openai.NewClient(apiKey, options.Model, openai.WithUsageListener(options.UsageListener)), nil
	case ProviderOllama:
		client, err := ollama.NewClient(ctx, options.Model,
			ollama.WithBaseURL(options.URL),
			ollama.WithUsageListener(options.UsageListener))
		if err != nil {
			return nil, err
		}
		return client, nil
	case ProviderGeminiAI:
		apiKey, err := f.apiKey(ctx, options.APIKeyURL)
		if err != nil {
			return nil, err
		}
		return gemini.NewClient(apiKey, options.Model,
			gemini.WithUsageListener(options.UsageListener)), nil
	case ProviderVertexAIClaude:
		client, err := vertexaiclaude.NewClient(ctx, options.Model,
			vertexaiclaude.WithProjectID(options.ProjectID),
			vertexaiclaude.WithUsageListener(options.UsageListener))
		if err != nil {
			return nil, err
		}
		return client, nil
	case ProviderBedrockClaude:
		client, err := bedrockclaude.NewClient(ctx, options.Model,
			bedrockclaude.WithRegion(options.Region),
			bedrockclaude.WithCredentialsURL(options.CredentialsURL),
			bedrockclaude.WithUsageListener(options.UsageListener))
		if err != nil {
			return nil, err
		}
		return client, nil
	case ProviderInceptionLabs:
		apiKey, err := f.apiKey(ctx, options.APIKeyURL)
		if err != nil {
			return nil, err
		}
		return inceptionlabs.NewClient(apiKey, options.Model,
			inceptionlabs.WithUsageListener(options.UsageListener)), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %v", options.Provider)
	}
}

func (o *Factory) apiKey(ctx context.Context, APIKeyURL string) (string, error) {
	if APIKeyURL == "" {
		return "", nil
	}
	key, err := o.secrets.GeyKey(ctx, APIKeyURL)
	if err != nil {
		return "", err
	}
	return key.Secret, nil
}

func New() *Factory {
	return &Factory{secrets: secret.New()}
}
