package openai

// AdaptRequest converts a slice of texts to an OpenAI-specific Request
func AdaptRequest(texts []string, model string) *Request {
	return &Request{
		Model: model,
		Input: texts,
	}
}

// AdaptResponse converts an OpenAI-specific Response to vectors and token count
func AdaptResponse(resp *Response, model string, embeddings *[][]float32, tokens *int) {
	// Use the model from the response if available, otherwise use the provided model
	responseModel := resp.Model
	if responseModel == "" {
		responseModel = model
	}

	// Extract embeddings from the response
	for _, data := range resp.Data {
		*embeddings = append(*embeddings, data.Embedding)
	}

	// Update token count
	*tokens += resp.Usage.TotalTokens
}
