package vertexai

// AdaptRequest converts a slice of texts to a VertexAI-specific Request
func AdaptRequest(texts []string, model string) *Request {
	// Create instances from the texts
	instances := make([]Instance, len(texts))
	for i, text := range texts {
		instances[i] = Instance{
			Content: text,
		}
	}

	return &Request{
		Instances: instances,
	}
}

// AdaptResponse converts a VertexAI-specific Response to vectors and token count
func AdaptResponse(resp *Response, model string, embeddings *[][]float32, tokens *int) {
	// Extract embeddings from the response
	for _, prediction := range resp.Predictions {
		*embeddings = append(*embeddings, prediction.Embeddings.Values)

		// Update token count
		// VertexAI doesn't provide usage information in the same format as OpenAI,
		// but we can use the token count from the statistics
		*tokens += prediction.Embeddings.Statistics.TokenCount
	}
}
