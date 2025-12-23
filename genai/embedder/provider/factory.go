package provider

import (
	"context"
	"fmt"

	"github.com/viant/agently/genai/embedder/provider/base"
	"github.com/viant/agently/genai/embedder/provider/ollama"
	"github.com/viant/agently/genai/embedder/provider/openai"
	"github.com/viant/agently/genai/embedder/provider/vertexai"
	"github.com/viant/agently/internal/genai/provider/openai/chatgptauth"
	"github.com/viant/scy/cred/secret"
)

type Factory struct {
	secrets *secret.Service
}

func (f *Factory) CreateEmbedder(ctx context.Context, options *Options) (base.Embedder, error) {

	if options.Provider == "" {
		return nil, fmt.Errorf("provider was empty")
	}
	switch options.Provider {
	case ProviderOpenAI:
		apiKey, err := f.apiKey(ctx, options.APIKeyURL)
		if err != nil {
			return nil, err
		}
		var clientOptions []openai.ClientOption
		clientOptions = append(clientOptions, openai.WithHTTPClient(options.httpClient))
		clientOptions = append(clientOptions, openai.WithUsageListener(options.usageListener))
		if apiKey == "" && options.ChatGPTOAuth != nil {
			manager, err := f.chatgptOAuthManager(options.ChatGPTOAuth)
			if err != nil {
				return nil, err
			}
			clientOptions = append(clientOptions, openai.WithAPIKeyProvider(manager.APIKey))
		}
		return openai.NewClient(apiKey, options.Model, clientOptions...), nil
	case ProviderOllama:
		return ollama.NewClient(options.Model,
			ollama.WithHTTPClient(options.httpClient),
			ollama.WithBaseURL(options.URL),
			ollama.WithUsageListener(options.usageListener)), nil
	case ProviderVertexAI:
		client, err := vertexai.NewClient(ctx, options.ProjectID, options.Model,
			vertexai.WithHTTPClient(options.httpClient),
			vertexai.WithLocation(options.Location),
			vertexai.WithScopes(options.Scopes...),
			vertexai.WithProjectID(options.ProjectID),
			vertexai.WithUsageListener(options.usageListener))
		if err != nil {
			return nil, err
		}
		return client, nil
	default:
		return nil, fmt.Errorf("unsupported provider: %v", options.Provider)
	}
}

func (f *Factory) apiKey(ctx context.Context, APIKeyURL string) (string, error) {
	if APIKeyURL == "" {
		return "", nil
	}
	key, err := f.secrets.GeyKey(ctx, APIKeyURL)
	if err != nil {
		return "", err
	}
	return key.Secret, nil
}

func New() *Factory {
	return &Factory{secrets: secret.New()}
}

func (f *Factory) chatgptOAuthManager(options *ChatGPTOAuthOptions) (*chatgptauth.Manager, error) {
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
