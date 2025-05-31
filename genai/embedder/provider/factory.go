package provider

import (
	"context"
	"fmt"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/viant/agently/genai/embedder/provider/ollama"
	"github.com/viant/agently/genai/embedder/provider/openai"
	"github.com/viant/agently/genai/embedder/provider/vertexai"
	"github.com/viant/scy/cred/secret"
)

type Factory struct {
	secrets *secret.Service
}

func (f *Factory) CreateEmbedder(ctx context.Context, options *Options) (embeddings.EmbedderClient, error) {

	if options.Provider == "" {
		return nil, fmt.Errorf("provider was empty")
	}
	switch options.Provider {
	case ProviderOpenAI:
		apiKey, err := f.apiKey(ctx, options.APIKeyURL)
		if err != nil {
			return nil, err
		}
		return openai.NewClient(apiKey, options.Model,
			openai.WithHTTPClient(options.httpClient),
			openai.WithUsageListener(options.usageListener)), nil
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
