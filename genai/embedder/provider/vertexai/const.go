package vertexai

const (
	// vertexAIEndpoint is the base URL for the VertexAI API
	vertexAIEndpoint = "https://%s-aiplatform.googleapis.com/v1/projects/%v/locations/%s/publishers/google/models/%s:predict"

	projectMetaEndpoint = "https://cloudresourcemanager.googleapis.com/v3/projects/%s"

	// defaultLocation is the default location for the VertexAI API
	defaultLocation = "us-central1"

	// defaultEmbeddingModel is the default model to use for embeddings
	defaultEmbeddingModel = "textembedding-gecko"
)
