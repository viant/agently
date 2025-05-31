package ollama

// Request represents the request structure for Ollama embeddings API
type Request struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// Response represents the response structure from Ollama embeddings API
type Response struct {
	Embedding []float32 `json:"embedding"`
	CreatedAt string    `json:"created_at"`
	Model     string    `json:"model"`
	EvalCount int       `json:"eval_count"`
}
