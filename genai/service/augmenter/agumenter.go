package augmenter

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/tmc/langchaingo/embeddings"
	mcpfs "github.com/viant/agently/genai/service/augmenter/mcpfs"
	"github.com/viant/agently/internal/mcpuri"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/embedius/indexer"
	"github.com/viant/embedius/indexer/fs"
	"github.com/viant/embedius/indexer/fs/splitter"
	"github.com/viant/embedius/matching"
	"github.com/viant/embedius/matching/option"
	"github.com/viant/embedius/vectordb/mem"
)

type DocsAugmenter struct {
	embedder  string
	options   *option.Options
	fsIndexer *fs.Indexer
	memStore  *mem.Store
	service   *indexer.Service
}

type CodeAugmenter struct {
	memStore *mem.Store
	service  *indexer.Service
}

func Key(embedder string, options *option.Options) string {
	builder := strings.Builder{}
	builder.WriteString(embedder)
	builder.WriteString(":")
	if options != nil {
		if options.MaxFileSize > 0 {
			builder.WriteString(fmt.Sprintf("maxInclusionFileSize=%d", options.MaxFileSize))
		}
		if len(options.Inclusions) > 0 {
			builder.WriteString("incl:" + strings.Join(options.Inclusions, ","))
		}
		if len(options.Exclusions) > 0 {
			builder.WriteString("excl:" + strings.Join(options.Exclusions, ","))
		}
	}
	return builder.String()
}

func NewDocsAugmenter(embeddingsModel string, embedder embeddings.Embedder, options ...option.Option) *DocsAugmenter {
	baseURL := embeddingBaseURL()
	matcher := matching.New(options...)
	splitterFactory := splitter.NewFactory(4096)
	// Register a basic PDF splitter to extract printable text before chunking.
	splitterFactory.RegisterExtensionSplitter(".pdf", NewPDFSplitter(4096))
	ret := &DocsAugmenter{
		embedder:  embeddingsModel,
		fsIndexer: fs.New(baseURL, embeddingsModel, matcher, splitterFactory),
		memStore:  mem.NewStore(mem.WithBaseURL(baseURL)),
	}
	ret.service = indexer.NewService(baseURL, ret.memStore, embedder, ret.fsIndexer)
	return ret
}

func embeddingBaseURL() string { return path.Join(workspace.Root(), "index") }

func (s *Service) getDocAugmenter(ctx context.Context, input *AugmentDocsInput) (*DocsAugmenter, error) {
	key := Key(input.Model, input.Match)
	augmenter, ok := s.DocsAugmenters.Get(key)
	if !ok {
		model, err := s.finder.Find(ctx, input.Model)
		if err != nil {
			return nil, fmt.Errorf("failed to get embeddingModel: %v, %w", input.Model, err)
		}
		var matchOptions = []option.Option{}
		if input.Match != nil {
			matchOptions = input.Match.Options()
		}
		// Detect if any location targets MCP resources; if so, prefer a composite fs
		// that supports both MCP and regular AFS sources.
		useMCP := false
		for _, loc := range input.Locations {
			if mcpuri.Is(loc) {
				useMCP = true
				break
			}
		}
		if useMCP && s.mcpMgr != nil {
			baseURL := embeddingBaseURL()
			matcher := matching.New(matchOptions...)
			splitterFactory := splitter.NewFactory(4096)
			splitterFactory.RegisterExtensionSplitter(".pdf", NewPDFSplitter(4096))
			idx := fs.NewWithFS(baseURL, input.Model, matcher, splitterFactory, mcpfs.NewComposite(s.mcpMgr))
			ret := &DocsAugmenter{
				embedder:  input.Model,
				fsIndexer: idx,
				memStore:  mem.NewStore(mem.WithBaseURL(baseURL)),
			}
			ret.service = indexer.NewService(baseURL, ret.memStore, model, ret.fsIndexer)
			augmenter = ret
		} else {
			augmenter = NewDocsAugmenter(input.Model, model, matchOptions...)
		}
		s.DocsAugmenters.Set(key, augmenter)
	}
	return augmenter, nil
}
