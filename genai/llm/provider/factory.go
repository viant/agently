package provider

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

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
		loggingEnabled := true
		if options.OpenAILogging != nil {
			loggingEnabled = *options.OpenAILogging
		}
		apiKey, err := f.apiKey(ctx, options.APIKeyURL)
		if err != nil {
			return nil, err
		}
		envAPIKey := strings.TrimSpace(os.Getenv(defaultEnvKey(options.EnvKey, "OPENAI_API_KEY")))
		opts := []openai.ClientOption{openai.WithUsageListener(options.UsageListener)}
		authSource := "openai:apiKeyURL_or_env"
		if strings.TrimSpace(options.URL) != "" {
			opts = append(opts, openai.WithBaseURL(strings.TrimSpace(options.URL)))
		}
		openAILogf(loggingEnabled, "[openai-auth] model=%s provider=%s chatgptOAuth=%t apiKeyURL_set=%t envKey=%s env_present=%t",
			options.Model,
			options.Provider,
			options.ChatGPTOAuth != nil,
			strings.TrimSpace(options.APIKeyURL) != "",
			defaultEnvKey(options.EnvKey, "OPENAI_API_KEY"),
			envAPIKey != "",
		)
		if options.ChatGPTOAuth != nil {
			manager, err := f.chatgptOAuthManager(options.ChatGPTOAuth)
			if err != nil {
				return nil, err
			}
			fallbackKey := strings.TrimSpace(apiKey)
			if fallbackKey == "" {
				fallbackKey = envAPIKey
			}
			backendMode := isChatGPTBackendURL(options.URL)
			authSource = "openai:chatgptOAuth"
			if backendMode {
				authSource += ":backend-api"
			}
			if fallbackKey != "" {
				authSource += "+fallback"
			}
			if options.ChatGPTOAuth.UseAccessTokenFallback {
				authSource += "+access-token"
			}
			opts = append(opts, openai.WithAPIKeyProvider(func(ctx context.Context) (string, error) {
				// For ChatGPT backend API, prefer direct access token and avoid API key mint attempt.
				if backendMode {
					if accessToken, err := manager.AccessToken(ctx); err == nil && strings.TrimSpace(accessToken) != "" {
						openAILogf(loggingEnabled, "[openai-auth] source=chatgptOAuth outcome=access_token")
						return accessToken, nil
					} else if err != nil {
						openAILogf(loggingEnabled, "[openai-auth] source=chatgptOAuth outcome=access_token_error fallback_available=false err=%v", err)
					}
					openAILogf(loggingEnabled, "[openai-auth] source=chatgptOAuth outcome=no_key")
					return "", fmt.Errorf("OpenAI access token unavailable from chatgptOAuth for backend API")
				}

				// For OpenAI API endpoint, prefer minted API keys; optional access-token fallback.
				if oauthKey, err := manager.APIKey(ctx); err == nil && strings.TrimSpace(oauthKey) != "" {
					openAILogf(loggingEnabled, "[openai-auth] source=chatgptOAuth outcome=oauth_key")
					return oauthKey, nil
				} else if err != nil {
					openAILogf(loggingEnabled, "[openai-auth] source=chatgptOAuth outcome=oauth_error fallback_available=%t err=%v", fallbackKey != "", err)
				}
				if options.ChatGPTOAuth.UseAccessTokenFallback {
					if accessToken, err := manager.AccessToken(ctx); err == nil && strings.TrimSpace(accessToken) != "" {
						openAILogf(loggingEnabled, "[openai-auth] source=chatgptOAuth outcome=access_token")
						return accessToken, nil
					} else if err != nil {
						openAILogf(loggingEnabled, "[openai-auth] source=chatgptOAuth outcome=access_token_error fallback_available=%t err=%v", fallbackKey != "", err)
					}
				}
				if fallbackKey != "" {
					openAILogf(loggingEnabled, "[openai-auth] source=chatgptOAuth outcome=fallback_key")
					return fallbackKey, nil
				}
				openAILogf(loggingEnabled, "[openai-auth] source=chatgptOAuth outcome=no_key")
				return "", fmt.Errorf("OpenAI API key unavailable from chatgptOAuth and no fallback key was configured")
			}))
			opts = append(opts, openai.WithAuthDiagnosticsProvider(func(ctx context.Context) string {
				return manager.Diagnostics(ctx)
			}))
			opts = append(opts, openai.WithChatGPTAccountIDProvider(func(ctx context.Context) (string, error) {
				return manager.AccountID(ctx)
			}))
			// Force key resolution through provider so oauth can override env/static key.
			apiKey = ""
		} else if strings.TrimSpace(apiKey) == "" {
			apiKey = envAPIKey
		}
		opts = append(opts, openai.WithAuthSource(authSource))
		opts = append(opts, openai.WithLoggingEnabled(loggingEnabled))
		if options.MaxTokens > 0 {
			opts = append(opts, openai.WithMaxTokens(options.MaxTokens))
		}
		if options.Temperature != nil {
			opts = append(opts, openai.WithTemperature(*options.Temperature))
		}
		if options.UserAgent != "" {
			opts = append(opts, openai.WithUserAgent(options.UserAgent))
		}
		if strings.TrimSpace(options.Originator) != "" {
			opts = append(opts, openai.WithOriginator(strings.TrimSpace(options.Originator)))
		}
		if strings.TrimSpace(options.CodexBetaFeatures) != "" {
			opts = append(opts, openai.WithCodexBetaFeatures(strings.TrimSpace(options.CodexBetaFeatures)))
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

func openAILogf(enabled bool, format string, args ...interface{}) {
	if !enabled {
		return
	}
	log.Printf(format, args...)
}

func isChatGPTBackendURL(baseURL string) bool {
	base := strings.ToLower(strings.TrimSpace(baseURL))
	if base == "" {
		return false
	}
	return strings.Contains(base, "chatgpt.com/backend-api") || strings.Contains(base, "chat.openai.com/backend-api")
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

func defaultEnvKey(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
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
