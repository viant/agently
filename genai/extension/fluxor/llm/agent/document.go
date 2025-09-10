package agent

import (
	"context"
	"fmt"

	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/extension/fluxor/llm/augmenter"

	"github.com/tmc/langchaingo/schema"
)

// matchDocuments gets relevant documents from the knowledge base
func (s *Service) matchDocuments(ctx context.Context, input *QueryInput, knowledge []*agent.Knowledge) ([]schema.Document, error) {
	if input.EmbeddingModel == "" {
		return nil, fmt.Errorf("embedding model was empty")
	}

	// Initialize augmenter input
	augmenterInput := &augmenter.AugmentDocsInput{
		Model:        input.EmbeddingModel,
		Query:        input.Query,
		MaxDocuments: input.MaxDocuments,
	}

	var allDocuments []schema.Document

	// Process each knowledge source
	for _, knowledge := range knowledge {
		augmenterInput.Locations = []string{knowledge.URL}
		augmenterInput.Match = knowledge.Match
		// Use augmenter to get relevant documents
		augmenterOutput := &augmenter.AugmentDocsOutput{}
		err := s.augmenter.AugmentDocs(ctx, augmenterInput, augmenterOutput)
		if err != nil {
			return nil, fmt.Errorf("failed to augment with knowledge %s: %w", knowledge.URL, err)
		}
		allDocuments = augmenterOutput.LoadDocuments(ctx, s.fs)
		// Stop if we've collected enough documents
		if input.MaxDocuments > 0 && len(allDocuments) >= input.MaxDocuments {
			allDocuments = allDocuments[:input.MaxDocuments]
			break
		}
	}
	return allDocuments, nil
}

// calculateDocumentsSize calculates the total size of retrieved documents
func (s *Service) calculateDocumentsSize(documents []schema.Document) int {
	size := 0
	for _, doc := range documents {
		size += len(doc.PageContent)
	}
	return size
}
