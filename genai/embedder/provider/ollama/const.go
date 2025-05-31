package ollama

const (
	// ollamaEndpoint is the base URL for the Ollama API
	ollamaEndpoint = "http://localhost:11434/api"
	
	// embeddingsEndpoint is the endpoint for the embeddings API
	embeddingsEndpoint = "/embeddings"
	
	// defaultEmbeddingModel is the default model to use for embeddings
	defaultEmbeddingModel = "nomic-embed-text"
)