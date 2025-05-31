package openai

// Request represents the request structure for OpenAI embeddings API
type Request struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// Response represents the response structure from OpenAI embeddings API
type Response struct {
	Object string          `json:"object"`
	Data   []EmbeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  EmbeddingUsage  `json:"usage"`
}

// EmbeddingData represents a single embedding in the OpenAI embeddings API response
type EmbeddingData struct {
	Object    string    `json:"object"`
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

// EmbeddingUsage represents token usage information in the OpenAI embeddings API response
type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
