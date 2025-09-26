package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/viant/afs/url"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/service/augmenter"

	"github.com/tmc/langchaingo/schema"
)

// matchDocuments gets relevant documents from the knowledge base
func (s *Service) matchDocuments(ctx context.Context, input *QueryInput, knowledge []*agent.Knowledge) ([]schema.Document, error) {
	if input.EmbeddingModel == "" {
		return nil, fmt.Errorf("embedding model was not specified")
	}

	if len(knowledge) == 0 {
		return []schema.Document{}, nil
	}

	if knowledge[0] != nil && knowledge[0].InclusionMode == "full" {
		return s.fullKnowledge(ctx, knowledge)
	}

	return s.onlyNeededKnowledge(ctx, input, knowledge)
}

func (s *Service) fullKnowledge(ctx context.Context, knowledge []*agent.Knowledge) ([]schema.Document, error) {
	// Traverse each knowledge URL using afs.Service.Walk and collect file paths.
	files := make([]string, 0, 128)
	seen := make(map[string]struct{})

	addFile := func(p string) {
		if strings.TrimSpace(p) == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		files = append(files, p)
	}

	for _, kn := range knowledge {
		if kn == nil || strings.TrimSpace(kn.URL) == "" {
			continue
		}

		err := s.fs.Walk(ctx, kn.URL, func(ctx context.Context, baseURL string, parent string, info os.FileInfo, reader io.Reader) (bool, error) {
			if info == nil || info.IsDir() {
				return true, nil
			}

			p := ""
			if parent == "" {
				p = url.Join(baseURL, info.Name())
			} else {
				p = url.Join(baseURL, parent, info.Name())
			}

			addFile(p)
			return true, nil
		})

		if err != nil {
			return nil, fmt.Errorf("failed to walk knowledge URL %s: %w", kn.URL, err)
		}
	}

	// Sort deterministically by path
	sort.Strings(files)

	// Load file contents into schema.Document with metadata["path"] = file path
	out := make([]schema.Document, 0, len(files))
	for _, p := range files {
		data, err := s.fs.DownloadWithURL(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("failed to read knowledge file %s: %w", p, err)
		}
		out = append(out, schema.Document{Metadata: map[string]any{"path": p}, PageContent: string(data)})
	}
	return out, nil
}

func (s *Service) onlyNeededKnowledge(ctx context.Context, input *QueryInput, knowledge []*agent.Knowledge) ([]schema.Document, error) {
	var allDocuments []schema.Document

	// Initialize augmenter input
	augmenterInput := &augmenter.AugmentDocsInput{
		Model:        input.EmbeddingModel,
		Query:        input.Query,
		MaxDocuments: input.MaxDocuments,
	}

	for _, kn := range knowledge {
		if kn == nil {
			continue
		}
		augmenterInput.Locations = []string{kn.URL}
		augmenterInput.Match = kn.Match
		// Use augmenter to get relevant documents
		augmenterOutput := &augmenter.AugmentDocsOutput{}
		err := s.augmenter.AugmentDocs(ctx, augmenterInput, augmenterOutput)
		if err != nil {
			return nil, fmt.Errorf("failed to augment with knowledge %s: %w", kn.URL, err)
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
