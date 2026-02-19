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
	"github.com/viant/agently/internal/workspace"
	embSchema "github.com/viant/embedius/schema"
)

// matchDocuments gets relevant documents from the knowledge base
func (s *Service) matchDocuments(ctx context.Context, input *QueryInput, knowledge []*agent.Knowledge) ([]embSchema.Document, error) {
	if input.EmbeddingModel == "" {
		return nil, fmt.Errorf("embedding model was not specified")
	}

	if len(knowledge) == 0 {
		return []embSchema.Document{}, nil
	}

	// Decide mode: explicit full, explicit match, or auto (default)
	if !s.shouldUseMatch(ctx, knowledge) {
		return s.fullKnowledge(ctx, knowledge)
	}

	return s.onlyNeededKnowledge(ctx, input, knowledge)
}

func (s *Service) fullKnowledge(ctx context.Context, knowledge []*agent.Knowledge) ([]embSchema.Document, error) {
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
	out := make([]embSchema.Document, 0, len(files))
	for _, p := range files {
		data, err := s.fs.DownloadWithURL(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("failed to read knowledge file %s: %w", p, err)
		}
		out = append(out, embSchema.Document{Metadata: map[string]any{"path": p}, PageContent: string(data)})
	}
	return out, nil
}

func (s *Service) onlyNeededKnowledge(ctx context.Context, input *QueryInput, knowledge []*agent.Knowledge) ([]embSchema.Document, error) {
	var allDocuments []embSchema.Document

	// Initialize augmenter input
	augmenterInput := &augmenter.AugmentDocsInput{
		Model:        input.EmbeddingModel,
		Query:        input.Query,
		MaxDocuments: input.MaxDocuments,
		TrimPath:     workspace.Root(),
	}

	for _, kn := range knowledge {
		if kn == nil {
			continue
		}
		augmenterInput.Locations = []string{kn.URL}
		augmenterInput.Match = kn.Filter
		// Use augmenter to get relevant documents
		augmenterOutput := &augmenter.AugmentDocsOutput{}
		err := s.augmenter.AugmentDocs(ctx, augmenterInput, augmenterOutput)
		if err != nil {
			return nil, fmt.Errorf("failed to augment with knowledge %s: %w", kn.URL, err)
		}
		// Optional minScore filter
		if kn.MinScore != nil {
			filtered := make([]embSchema.Document, 0, len(augmenterOutput.Documents))
			for _, d := range augmenterOutput.Documents {
				if d.Score >= float32(*kn.MinScore) {
					filtered = append(filtered, d)
				}
			}
			augmenterOutput.Documents = filtered
		}
		// Stable order by normalized source URI to maximize cache reuse
		sort.SliceStable(augmenterOutput.Documents, func(i, j int) bool {
			si := strings.ToLower(strings.TrimSpace(extractSourceDoc(augmenterOutput.Documents[i].Metadata)))
			sj := strings.ToLower(strings.TrimSpace(extractSourceDoc(augmenterOutput.Documents[j].Metadata)))
			return si < sj
		})
		// Dedupe by normalized source URI (keep first occurrence)
		{
			seen := map[string]bool{}
			uniq := make([]embSchema.Document, 0, len(augmenterOutput.Documents))
			for _, d := range augmenterOutput.Documents {
				key := strings.ToLower(strings.TrimSpace(extractSourceDoc(d.Metadata)))
				if key == "" || seen[key] {
					continue
				}
				seen[key] = true
				uniq = append(uniq, d)
			}
			augmenterOutput.Documents = uniq
		}
		// Limit to the top N matched assets per knowledge (default 5 or defaults.match.maxFiles)
		limit := kn.EffectiveMaxFiles()
		if kn.MaxFiles <= 0 && s.defaults != nil && s.defaults.Match.MaxFiles > 0 {
			limit = s.defaults.Match.MaxFiles
		}
		if limit > 0 && len(augmenterOutput.Documents) > limit {
			augmenterOutput.Documents = augmenterOutput.Documents[:limit]
		}
		loaded := augmenterOutput.LoadDocuments(ctx, s.fs)
		// Trim trailing whitespace to stabilize content tokens
		for i := range loaded {
			loaded[i].PageContent = strings.TrimSpace(loaded[i].PageContent)
		}
		allDocuments = loaded
	}
	return allDocuments, nil
}

// matchResources removed from binding path; binding continues to use knowledge.

// shouldUseMatch determines whether to use match mode.
//   - inclusionMode=="match" => true
//   - inclusionMode=="full" => false
//   - default/auto: count files in each location; if any count exceeds
//     EffectiveMaxFiles (default 5), return true (use match). Otherwise false.
func (s *Service) shouldUseMatch(ctx context.Context, knowledge []*agent.Knowledge) bool {
	if len(knowledge) == 0 || knowledge[0] == nil {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(knowledge[0].InclusionMode))
	switch mode {
	case "match":
		return true
	case "full":
		return false
	}
	// Auto/default
	for _, kn := range knowledge {
		if kn == nil || strings.TrimSpace(kn.URL) == "" {
			continue
		}
		limit := kn.EffectiveMaxFiles()
		if kn.MaxFiles <= 0 && s.defaults != nil && s.defaults.Match.MaxFiles > 0 {
			limit = s.defaults.Match.MaxFiles
		}
		count := 0
		_ = s.fs.Walk(ctx, kn.URL, func(ctx context.Context, baseURL string, parent string, info os.FileInfo, _ io.Reader) (bool, error) {
			if info == nil {
				return true, nil
			}
			if info.IsDir() {
				return true, nil
			}
			count++
			if count > limit {
				return false, nil // early stop
			}
			return true, nil
		})
		if count > limit {
			return true // prefer match when too many resources
		}
	}
	return false // small sets -> full mode OK
}

// calculateDocumentsSize calculates the total size of retrieved documents
func (s *Service) calculateDocumentsSize(documents []embSchema.Document) int {
	size := 0
	for _, doc := range documents {
		size += len(doc.PageContent)
	}
	return size
}

// extractSource mirrors adapter.extractSource to avoid import cycles.
func extractSourceDoc(meta map[string]any) string { // local copy used only in this file
	if meta == nil {
		return ""
	}
	if v, ok := meta["path"]; ok {
		if s, _ := v.(string); strings.TrimSpace(s) != "" {
			return s
		}
	}
	if v, ok := meta["docId"]; ok {
		if s, _ := v.(string); strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
