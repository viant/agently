package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/viant/agently/genai/llm"
	bedrockclaude "github.com/viant/agently/genai/llm/provider/bedrock/claude"
	"github.com/viant/agently/genai/llm/provider/grok"
	"github.com/viant/agently/genai/llm/provider/inceptionlabs"
	"github.com/viant/agently/genai/llm/provider/ollama"
	"github.com/viant/agently/genai/llm/provider/openai"
	vertexaiclaude "github.com/viant/agently/genai/llm/provider/vertexai/claude"
	"github.com/viant/agently/genai/llm/provider/vertexai/gemini"
	"github.com/viant/agently/internal/genai/provider/openai/chatgptauth"
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
		opts := []openai.ClientOption{openai.WithUsageListener(options.UsageListener)}
		if apiKey == "" && options.ChatGPTOAuth != nil {
			manager, err := f.chatgptOAuthManager(options.ChatGPTOAuth)
			if err != nil {
				return nil, err
			}
			opts = append(opts, openai.WithAPIKeyProvider(manager.APIKey))
		}
		if options.MaxTokens > 0 {
			opts = append(opts, openai.WithMaxTokens(options.MaxTokens))
		}
		if options.Temperature != nil {
			opts = append(opts, openai.WithTemperature(*options.Temperature))
		}
		if options.UserAgent != "" {
			opts = append(opts, openai.WithUserAgent(options.UserAgent))
		}
		// Pass through continuation flag; nil means default enabled.
		opts = append(opts, openai.WithContextContinuation(options.ContextContinuation))
		return openai.NewClient(apiKey, options.Model, opts...), nil
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
		opts := []gemini.ClientOption{gemini.WithUsageListener(options.UsageListener)}
		if options.MaxTokens > 0 {
			opts = append(opts, gemini.WithMaxTokens(options.MaxTokens))
		}
		if options.Temperature != nil {
			opts = append(opts, gemini.WithTemperature(*options.Temperature))
		}
		return gemini.NewClient(apiKey, options.Model, opts...), nil
	case ProviderVertexAIClaude:
		vOpts := []vertexaiclaude.ClientOption{
			vertexaiclaude.WithProjectID(options.ProjectID),
			vertexaiclaude.WithUsageListener(options.UsageListener),
		}
		if options.MaxTokens > 0 {
			vOpts = append(vOpts, vertexaiclaude.WithMaxTokens(options.MaxTokens))
		}
		if options.Temperature != nil {
			vOpts = append(vOpts, vertexaiclaude.WithTemperature(*options.Temperature))
		}
		client, err := vertexaiclaude.NewClient(ctx, options.Model, vOpts...)
		if err != nil {
			return nil, err
		}
		return client, nil
	case ProviderBedrockClaude:
		bedrockOpts := []bedrockclaude.ClientOption{
			bedrockclaude.WithRegion(options.Region),
			bedrockclaude.WithCredentialsURL(options.CredentialsURL),
			bedrockclaude.WithUsageListener(options.UsageListener),
		}
		if options.MaxTokens > 0 {
			bedrockOpts = append(bedrockOpts, bedrockclaude.WithMaxTokens(options.MaxTokens))
		}
		if options.Temperature != nil {
			bedrockOpts = append(bedrockOpts, bedrockclaude.WithTemperature(*options.Temperature))
		}
		client, err := bedrockclaude.NewClient(ctx, options.Model, bedrockOpts...)
		if err != nil {
			return nil, err
		}
		return client, nil
	case ProviderInceptionLabs:
		apiKey, err := f.apiKey(ctx, options.APIKeyURL)
		if err != nil {
			return nil, err
		}
		// Fallback to environment variable when not provided via secrets.
		if apiKey == "" {
			// Prefer explicitly provided EnvKey; otherwise default to INCEPTIONLABS_API_KEY
			if envKey := options.EnvKey; envKey != "" {
				apiKey = os.Getenv(envKey)
			}
			if apiKey == "" {
				apiKey = os.Getenv("INCEPTIONLABS_API_KEY")
			}
		}
		return inceptionlabs.NewClient(apiKey, options.Model,
			inceptionlabs.WithUsageListener(options.UsageListener)), nil
	case ProviderGrok:
		apiKey, err := f.apiKey(ctx, options.APIKeyURL)
		if err != nil {
			return nil, err
		}
		// Fallback to environment variable when not provided via secrets.
		if apiKey == "" {
			// Prefer explicitly provided EnvKey; otherwise default to XAI_API_KEY
			if envKey := options.EnvKey; envKey != "" {
				apiKey = os.Getenv(envKey)
			}
			if apiKey == "" {
				apiKey = os.Getenv("XAI_API_KEY")
			}
		}
		return grok.NewClient(apiKey, options.Model,
			grok.WithUsageListener(options.UsageListener)), nil
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

func (o *Factory) chatgptOAuthManager(options *ChatGPTOAuthOptions) (*chatgptauth.Manager, error) {
	if options == nil {
		return nil, fmt.Errorf("chatgptOAuth options were nil")
	}
	if options.ClientURL == "" {
		return nil, fmt.Errorf("chatgptOAuth.clientURL was empty")
	}
	if options.TokensURL == "" {
		return nil, fmt.Errorf("chatgptOAuth.tokensURL was empty")
	}
	clientLoader := chatgptauth.NewScyOAuthClientLoader(options.ClientURL)
	tokenStore := chatgptauth.NewScyTokenStateStore(options.TokensURL)
	return chatgptauth.NewManager(
		&chatgptauth.Options{
			ClientURL:          options.ClientURL,
			TokensURL:          options.TokensURL,
			Issuer:             options.Issuer,
			AllowedWorkspaceID: options.AllowedWorkspaceID,
		},
		clientLoader,
		tokenStore,
		nil,
	)
}

func New() *Factory {
	return &Factory{secrets: secret.New()}
}
