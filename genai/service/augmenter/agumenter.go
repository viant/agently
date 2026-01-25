package augmenter

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	adaptembed "github.com/viant/agently/genai/embedder/adapter"
	baseembed "github.com/viant/agently/genai/embedder/provider/base"
	mcpfs "github.com/viant/agently/genai/service/augmenter/mcpfs"
	authctx "github.com/viant/agently/internal/auth"
	mcpuri "github.com/viant/agently/internal/mcp/uri"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/embedius/indexer"
	"github.com/viant/embedius/indexer/fs"
	"github.com/viant/embedius/indexer/fs/splitter"
	"github.com/viant/embedius/matching"
	"github.com/viant/embedius/matching/option"
	"github.com/viant/embedius/vectordb/sqlitevec"
)

type DocsAugmenter struct {
	embedder  string
	options   *option.Options
	fsIndexer *fs.Indexer
	store     *sqlitevec.Store
	service   *indexer.Service
}

type CodeAugmenter struct {
	store   *sqlitevec.Store
	service *indexer.Service
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

func NewDocsAugmenter(ctx context.Context, embeddingsModel string, embedder baseembed.Embedder, options ...option.Option) *DocsAugmenter {
	baseURL := embeddingBaseURL(ctx)
	_ = os.MkdirAll(baseURL, 0755)
	matcher := matching.New(options...)
	splitterFactory := splitter.NewFactory(4096)
	// Register a basic PDF splitter to extract printable text before chunking.
	splitterFactory.RegisterExtensionSplitter(".pdf", NewPDFSplitter(4096))
	store, err := newSQLiteStore(baseURL)
	if err != nil {
		panic(fmt.Sprintf("failed to create sqlitevec store: %v", err))
	}
	ret := &DocsAugmenter{
		embedder:  embeddingsModel,
		fsIndexer: fs.New(baseURL, embeddingsModel, matcher, splitterFactory),
		store:     store,
	}
	ret.service = indexer.NewService(baseURL, ret.store, adaptembed.LangchainEmbedderAdapter{Inner: embedder}, ret.fsIndexer)

	return ret
}

// NewDocsAugmenterWithStore constructs a DocsAugmenter that reuses the provided sqlitevec store.
func NewDocsAugmenterWithStore(ctx context.Context, embeddingsModel string, embedder baseembed.Embedder, store *sqlitevec.Store, options ...option.Option) *DocsAugmenter {
	baseURL := embeddingBaseURL(ctx)
	_ = os.MkdirAll(baseURL, 0755)
	matcher := matching.New(options...)
	splitterFactory := splitter.NewFactory(4096)
	splitterFactory.RegisterExtensionSplitter(".pdf", NewPDFSplitter(4096))
	ret := &DocsAugmenter{
		embedder:  embeddingsModel,
		fsIndexer: fs.New(baseURL, embeddingsModel, matcher, splitterFactory),
		store:     store,
	}
	ret.service = indexer.NewService(baseURL, ret.store, adaptembed.LangchainEmbedderAdapter{Inner: embedder}, ret.fsIndexer)
	return ret
}

func embeddingBaseURL(ctx context.Context) string {
	user := strings.TrimSpace(authctx.EffectiveUserID(ctx))
	if user == "" {
		user = "default"
	}
	base := path.Join(workspace.Root(), "index", user)

	return base
}

func (s *Service) getDocAugmenter(ctx context.Context, input *AugmentDocsInput) (*DocsAugmenter, error) {
	// Detect if any location targets MCP resources; if so, prefer a composite fs
	// that supports both MCP and regular AFS sources.
	useMCP := false
	for _, loc := range input.Locations {
		if mcpuri.Is(loc) {
			useMCP = true
			break
		}
	}
	if useMCP && s.mcpMgr == nil {
		return nil, fmt.Errorf("mcp manager not configured for MCP locations")
	}
	// Use a single augmenter per model+options(+mcp) and a shared sqlite store.
	key := Key(input.Model, input.Match)
	if useMCP {
		key += ":mcp"
	}
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
		store := s.ensureStore(ctx)
		if useMCP && s.mcpMgr != nil {
			baseURL := embeddingBaseURL(ctx)
			_ = os.MkdirAll(baseURL, 0755)
			matcher := matching.New(matchOptions...)
			splitterFactory := splitter.NewFactory(4096)
			splitterFactory.RegisterExtensionSplitter(".pdf", NewPDFSplitter(4096))
			idx := fs.NewWithFS(
				baseURL,
				input.Model,
				matcher,
				splitterFactory,
				mcpfs.NewComposite(
					s.mcpMgr,
					mcpfs.WithSnapshotResolver(s.mcpSnapshotResolver),
					mcpfs.WithSnapshotManifestResolver(s.mcpSnapshotManifestResolver),
				),
			)
			ret := &DocsAugmenter{
				embedder:  input.Model,
				fsIndexer: idx,
				store:     store,
			}
			ret.service = indexer.NewService(baseURL, ret.store, adaptembed.LangchainEmbedderAdapter{Inner: model}, ret.fsIndexer)
			augmenter = ret
		} else {
			augmenter = NewDocsAugmenterWithStore(ctx, input.Model, model, store, matchOptions...)
		}
		s.DocsAugmenters.Set(key, augmenter)
	}
	return augmenter, nil
}

// ensureStore initializes a shared sqlite-vec store once per process.
func (s *Service) ensureStore(ctx context.Context) *sqlitevec.Store {
	s.storeOnce.Do(func() {
		baseURL := embeddingBaseURL(ctx)
		_ = os.MkdirAll(baseURL, 0755)
		store, err := newSQLiteStore(baseURL)
		if err != nil {
			panic(fmt.Sprintf("failed to create sqlitevec store: %v", err))
		}
		s.store = store
	})
	return s.store
}

// debugf prints Embedius-related debug information when AGENTLY_DEBUG_EMBEDIUS=1
