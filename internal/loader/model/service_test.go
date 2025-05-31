package model

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/viant/afs"
	_ "github.com/viant/afs/embed"
	"github.com/viant/agently/genai/llm/provider"
	"github.com/viant/agently/internal/loader/fs"
	"github.com/viant/fluxor/service/meta"
	"testing"
)

// testFS holds our test YAML files
//
//go:embed testdata/*
var testFS embed.FS

// TestService_Load tests the model loading functionality
func TestService_Load(t *testing.T) {
	// Set up memory file system
	ctx := context.Background()

	// Test cases
	testCases := []struct {
		name         string
		url          string
		expectedJSON string
		expectedErr  bool
	}{
		{
			name:         "Valid OpenAI model",
			url:          "openai.yaml",
			expectedJSON: `{"id":"gpt-4","description":"GPT-4 model from OpenAI","options":{"model":"gpt-4","provider":"openai","apiKeyURL":"secrets/openai","temperature":0.7,"maxTokens":4096}}`,
		},
		{
			name:         "Valid GoogleAI model",
			url:          "googleai.yaml",
			expectedJSON: `{"id":"gemini-pro","description":"Gemini Pro model from Google","options":{"model":"gemini-pro","provider":"googleai","apiKeyURL":"secrets/googleai","credentialsURL":"secrets/gcp-credentials","temperature":0.8,"maxTokens":2048}}`,
		},
		{
			name:        "Invalid URL",
			url:         "nonexistent.yaml",
			expectedErr: true,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service := New(fs.WithMetaService[provider.Config](meta.New(afs.New(), "embed:///testdata", &testFS)))
			loadedModel, err := service.Load(ctx, tc.url)

			if tc.expectedErr {
				assert.NotNil(t, err)
				return
			}

			assert.Nil(t, err)
			expected := &provider.Config{}
			err = json.Unmarshal([]byte(tc.expectedJSON), expected)
			assert.Nil(t, err)

			if !assert.EqualValues(t, expected, loadedModel) {
				actualJSON, _ := json.Marshal(loadedModel)
				fmt.Println(string(actualJSON))
			}
		})
	}
}
