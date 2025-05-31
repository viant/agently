package provider

import (
	"context"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestFactory_CreateEmbedder(t *testing.T) {
	factory := New()
	ctx := context.Background()

	testCases := []struct {
		name        string
		options     *Options
		shouldError bool
		skipIf      func() bool
	}{
		{
			name: "OpenAI",
			options: &Options{
				Provider:  ProviderOpenAI,
				Model:     "text-embedding-3-small",
				APIKeyURL: os.Getenv("OPENAI_API_KEY_URL"),
			},
			shouldError: false,
			skipIf: func() bool {
				return os.Getenv("OPENAI_API_KEY_URL") == "" && os.Getenv("OPENAI_API_KEY") == ""
			},
		},
		{
			name: "Ollama",
			options: &Options{
				Provider: ProviderOllama,
				Model:    "llama2",
				URL:      "http://localhost:11434",
			},
			shouldError: false,
			skipIf: func() bool {
				// Skip if Ollama is not running locally
				return true
			},
		},
		{
			name: "VertexAI",
			options: &Options{
				Provider:  ProviderVertexAI,
				Model:     "textembedding-gecko@latest",
				ProjectID: os.Getenv("GCP_PROJECT_ID"),
			},
			shouldError: false,
			skipIf: func() bool {
				return os.Getenv("GCP_PROJECT_ID") == ""
			},
		},
		{
			name: "Empty Provider",
			options: &Options{
				Provider: "",
			},
			shouldError: true,
		},
		{
			name: "Unsupported Provider",
			options: &Options{
				Provider: "unsupported",
			},
			shouldError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skipIf != nil && tc.skipIf() {
				t.Skip("Skipping test case")
			}

			embedder, err := factory.CreateEmbedder(ctx, tc.options)
			if tc.shouldError {
				assert.Error(t, err)
				assert.Nil(t, embedder)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, embedder)
			}
		})
	}
}