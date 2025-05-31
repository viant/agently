package augmenter

import (
	"context"
	"fmt"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/viant/embedius/indexer"
	"github.com/viant/embedius/indexer/codebase"
	"github.com/viant/embedius/indexer/fs"
	"github.com/viant/embedius/indexer/fs/splitter"
	"github.com/viant/embedius/matching"
	"github.com/viant/embedius/matching/option"
	"github.com/viant/embedius/vectordb/mem"
	"os"
	"path"
	"strings"
)

type DocsAugmenter struct {
	embedder  string
	options   *option.Options
	fsIndexer *fs.Indexer
	memStore  *mem.Store
	service   *indexer.Service
}

type CodeAugmenter struct {
	codebaseIndexer *codebase.Indexer
	memStore        *mem.Store
	service         *indexer.Service
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
	ret := &DocsAugmenter{
		embedder:  embeddingsModel,
		fsIndexer: fs.New(baseURL, embeddingsModel, matcher, splitterFactory),
		memStore:  mem.NewStore(mem.WithBaseURL(baseURL)),
	}
	ret.service = indexer.NewService(baseURL, ret.memStore, embedder, ret.fsIndexer)
	return ret
}

func NewCodeAugmenter(embeddingsModel string, embedder embeddings.Embedder, options ...option.Option) *CodeAugmenter {
	baseURL := embeddingBaseURL()
	ret := &CodeAugmenter{
		codebaseIndexer: codebase.New(embeddingsModel),
		memStore:        mem.NewStore(),
	}
	ret.service = indexer.NewService(baseURL, ret.memStore, embedder, ret.codebaseIndexer)
	return ret
}

func embeddingBaseURL() string {
	baseURL := path.Join(os.Getenv("HOME"), ".emb")
	return baseURL
}

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
		augmenter = NewDocsAugmenter(input.Model, model, matchOptions...)
		s.DocsAugmenters.Set(key, augmenter)
	}
	return augmenter, nil
}

func (s *Service) getCodeAugmenter(ctx context.Context, input *AugmentCodeInput) (*CodeAugmenter, error) {
	key := Key(input.Model, nil)
	augmenter, ok := s.CodeAugmenters.Get(key)
	if !ok {
		model, err := s.finder.Find(ctx, input.Model)
		if err != nil {
			return nil, fmt.Errorf("failed to get model: %v, %w", input.Model, err)
		}
		augmenter = NewCodeAugmenter(input.Model, model)
		s.CodeAugmenters.Set(key, augmenter)
	}
	return augmenter, nil
}
