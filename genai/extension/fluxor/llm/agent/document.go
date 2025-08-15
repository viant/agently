package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/viant/afs/asset"
	"github.com/viant/afs/url"
	"github.com/viant/agently/genai/extension/fluxor/llm/augmenter"

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
		augmenterInput.Model = s.defaults.Embedder

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

func (s *Service) buildSystemEnrichment(documents []*asset.Resource) string {
	if len(documents) == 0 {
		return ""
	}

	var builder strings.Builder
	for i, doc := range documents {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(fmt.Sprintf("Document %d: %s\n", i+1, doc.Name))
		builder.WriteString("Result:\n")
		builder.Write(doc.Data)
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

// retrieveSystemRelevantDocuments gets relevant documents from the system knowledge base
func (s *Service) retrieveSystemRelevantDocuments(ctx context.Context, input *QueryInput) ([]*asset.Resource, error) {
	var allDocuments []*asset.Resource

	// Process each knowledge source
	for _, knowledge := range input.Agent.SystemKnowledge {

		docs, err := s.readFilesRecursive(ctx, knowledge.URL)
		if err != nil {
			return nil, fmt.Errorf("failed read system knowledge %s: %w", knowledge.URL, err)
		}

		allDocuments = append(allDocuments, docs...)

		// Stop if we've collected enough documents
		if input.MaxDocuments > 0 && len(allDocuments) >= input.MaxDocuments {
			allDocuments = allDocuments[:input.MaxDocuments]
			break
		}
	}

	return allDocuments, nil
}

// readFilesRecursive reads all files from the given directory recursively,
// returning them sorted alphabetically by full path.
func (s *Service) readFilesRecursive(ctx context.Context, root string) ([]*asset.Resource, error) {

	files := make([]*asset.Resource, 0, 0)

	err := s.fs.Walk(ctx, root, func(ctx context.Context, baseURL string, parent string, info os.FileInfo, reader io.Reader) (bool, error) {
		var name string
		if parent == "" {
			name = url.Join(baseURL, info.Name())
		} else {
			name = url.Join(baseURL, parent, info.Name())
		}

		if !info.IsDir() && reader != nil {
			if data, rErr := s.fs.DownloadWithURL(ctx, name); rErr == nil {
				files = append(files, asset.New(name, info.Mode(), info.IsDir(), "", data))
			} else {
				return false, rErr
			}
		}
		return true, nil
	})

	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	return files, nil
}
