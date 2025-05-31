package openai

import (
	"testing"
)

func TestAdaptRequest(t *testing.T) {
	testCases := []struct {
		name     string
		texts    []string
		model    string
		expected *Request
	}{
		{
			name:  "with texts",
			texts: []string{"test1", "test2"},
			model: "test-model",
			expected: &Request{
				Model: "test-model",
				Input: []string{"test1", "test2"},
			},
		},
		{
			name:  "empty texts",
			texts: []string{},
			model: "test-model",
			expected: &Request{
				Model: "test-model",
				Input: []string{},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := AdaptRequest(tc.texts, tc.model)

			// Check model
			if actual.Model != tc.expected.Model {
				t.Errorf("Expected model %s, got %s", tc.expected.Model, actual.Model)
			}

			// Check input length
			if len(actual.Input) != len(tc.expected.Input) {
				t.Errorf("Expected input length %d, got %d", len(tc.expected.Input), len(actual.Input))
			}

			// Check input values
			for i, v := range actual.Input {
				if v != tc.expected.Input[i] {
					t.Errorf("Expected input[%d] = %s, got %s", i, tc.expected.Input[i], v)
				}
			}
		})
	}
}

func TestAdaptResponse(t *testing.T) {
	testCases := []struct {
		name           string
		response       *Response
		model          string
		expectedVectors [][]float32
		expectedTokens int
	}{
		{
			name: "normal response",
			response: &Response{
				Object: "list",
				Data: []EmbeddingData{
					{
						Object:    "embedding",
						Embedding: []float32{0.1, 0.2, 0.3},
						Index:     0,
					},
					{
						Object:    "embedding",
						Embedding: []float32{0.4, 0.5, 0.6},
						Index:     1,
					},
				},
				Model: "test-model",
				Usage: EmbeddingUsage{
					PromptTokens: 10,
					TotalTokens:  10,
				},
			},
			model: "default-model",
			expectedVectors: [][]float32{
				{0.1, 0.2, 0.3},
				{0.4, 0.5, 0.6},
			},
			expectedTokens: 10,
		},
		{
			name: "empty response",
			response: &Response{
				Object: "list",
				Data:   []EmbeddingData{},
				Model:  "test-model",
				Usage: EmbeddingUsage{
					PromptTokens: 0,
					TotalTokens:  0,
				},
			},
			model:          "default-model",
			expectedVectors: [][]float32{},
			expectedTokens: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var actualVectors [][]float32
			var actualTokens int

			AdaptResponse(tc.response, tc.model, &actualVectors, &actualTokens)

			// Check vectors length
			if len(actualVectors) != len(tc.expectedVectors) {
				t.Errorf("Expected vectors length %d, got %d", len(tc.expectedVectors), len(actualVectors))
			}

			// Check vector values
			for i, embedding := range actualVectors {
				if len(embedding) != len(tc.expectedVectors[i]) {
					t.Errorf("Expected embedding length %d, got %d", len(tc.expectedVectors[i]), len(embedding))
				}

				for j, v := range embedding {
					if v != tc.expectedVectors[i][j] {
						t.Errorf("Expected vectors[%d][%d] = %f, got %f", i, j, tc.expectedVectors[i][j], v)
					}
				}
			}

			// Check tokens
			if actualTokens != tc.expectedTokens {
				t.Errorf("Expected tokens %d, got %d", tc.expectedTokens, actualTokens)
			}
		})
	}
}
