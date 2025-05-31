package agent

import (
	"context"
	"fmt"
	"github.com/viant/agently/genai/extension/fluxor/llm/augmenter"
	"strings"

	"github.com/tmc/langchaingo/schema"
)

// retrieveRelevantDocuments gets relevant documents from the knowledge base
func (s *Service) retrieveRelevantDocuments(ctx context.Context, input *QueryInput) ([]schema.Document, error) {
	// Initialize augmenter input
	augmenterInput := &augmenter.AugmentDocsInput{
		Model:        input.EmbeddingModel,
		Query:        input.Query,
		MaxDocuments: input.MaxDocuments,
	}

	var allDocuments []schema.Document

	// Process each knowledge source
	for _, knowledge := range input.Agent.Knowledge {
		augmenterInput.Locations = []string{knowledge.URL}
		augmenterInput.Match = knowledge.Match

		// Use augmenter to get relevant documents
		augmenterOutput := &augmenter.AugmentDocsOutput{}
		err := s.augmenter.AugmentDocs(ctx, augmenterInput, augmenterOutput)
		if err != nil {
			return nil, fmt.Errorf("failed to augment with knowledge %s: %w", knowledge.URL, err)
		}

		// Add documents to the collection
		allDocuments = append(allDocuments, augmenterOutput.Documents...)

		// Stop if we've collected enough documents
		if input.MaxDocuments > 0 && len(allDocuments) >= input.MaxDocuments {
			allDocuments = allDocuments[:input.MaxDocuments]
			break
		}
	}

	return allDocuments, nil
}

// formatDocumentsForEnrichment formats documents for prompt enrichment
func (s *Service) formatDocumentsForEnrichment(documents []schema.Document, includeFile bool) string {
	if len(documents) == 0 {
		return ""
	}
	var builder strings.Builder
	for i, doc := range documents {
		if i > 0 {
			builder.WriteString("\n\n")
		}

		builder.WriteString(fmt.Sprintf("Document %d: %s\n", i+1, doc.Metadata["source"]))

		if includeFile {
			builder.WriteString("Result:\n")
			builder.WriteString(doc.PageContent)
		} else {
			// Extract a snippet for context
			content := doc.PageContent
			if len(content) > 10000 {
				content = content[:10000] + "..."
			}
			builder.WriteString("Excerpt:\n")
			builder.WriteString(content)
		}
	}
	return builder.String()
}

// calculateDocumentsSize calculates the total size of retrieved documents
func (s *Service) calculateDocumentsSize(documents []schema.Document) int {
	size := 0
	for _, doc := range documents {
		size += len(doc.PageContent)
	}
	return size
}
