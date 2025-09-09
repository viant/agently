package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/extension/fluxor/llm/augmenter"

	"github.com/tmc/langchaingo/schema"
)

// matchDocuments gets relevant documents from the knowledge base
func (s *Service) matchDocuments(ctx context.Context, input *QueryInput, knowledge []*agent.Knowledge) ([]schema.Document, error) {
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
		augmenterInput.Model = s.defaults.Embedder
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

// formatDocumentsForEnrichment formats documents for prompt enrichment
func (s *Service) formatDocumentsForEnrichment(ctx context.Context, documents []schema.Document, includeFile bool) string {
	if len(documents) == 0 {
		return ""
	}

	var included = map[string]bool{}

	// If includeFile is true, we will include the file content in the output
	var builder strings.Builder
	var includeCnt = 0
	for i, doc := range documents {
		path, ok := doc.Metadata["path"]
		if !ok {
			path = doc.Metadata["docId"]
		}
		location, ok := path.(string)
		if included[location] {
			continue
		}

		if includeFile {
			if i > 0 {
				builder.WriteString("\n\n")
			}
			includeCnt++
			builder.WriteString(fmt.Sprintf("Document %d: %s\n", includeCnt, path))
			if ok {
				data, err := s.fs.DownloadWithURL(ctx, location)
				if err == nil {
					included[location] = true
					builder.WriteString("Result:\n")
					builder.WriteString(string(data))
					continue
				}
			}
			builder.WriteString("Result:\n")
			builder.WriteString(doc.PageContent)
		} else {
			if i > 0 {
				builder.WriteString("\n\n")
			}
			builder.WriteString(fmt.Sprintf("Document %d: %s\n", i+1, path))
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
