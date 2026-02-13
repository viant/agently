package augmenter

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/viant/afs"
	"github.com/viant/agently/genai/embedder"
	mcpfs "github.com/viant/agently/genai/service/augmenter/mcpfs"
	svc "github.com/viant/agently/genai/tool/service"
	mcpcfg "github.com/viant/agently/internal/mcp/config"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
	mcpuri "github.com/viant/agently/internal/mcp/uri"
	"github.com/viant/agently/internal/shared"
	"github.com/viant/datly/view"
	embedius "github.com/viant/embedius"
	embindexer "github.com/viant/embedius/indexer"
	embSchema "github.com/viant/embedius/schema"
	"github.com/viant/embedius/vectordb"
	"github.com/viant/embedius/vectordb/sqlitevec"
	"github.com/viant/scy"
	"github.com/viant/scy/cred/secret"
)

const name = "llm/augmenter"

// Service extracts structured information from LLM responses
type Service struct {
	finder         embedder.Finder
	DocsAugmenters shared.Map[string, *DocsAugmenter]
	// Optional MCP client manager for resolving mcp: resources during indexing
	mcpMgr                      *mcpmgr.Manager
	mcpSnapshotResolver         mcpfs.SnapshotResolver
	mcpSnapshotManifestResolver mcpfs.SnapshotManifestResolver
	mcpSnapshotCacheRoot        string
	indexPathTemplate           string
	// Local (non-MCP) upstream sync configuration.
	localRoots     []localRoot
	localUpstreams map[string]LocalUpstream
	// sqlite-vec stores keyed by db path
	storeMu sync.Mutex
	stores  map[string]*sqlitevec.Store
	// Limits parallel upstream sync operations.
	upstreamSyncConcurrency int
	// Limits parallel matching operations across roots.
	matchConcurrency int
	indexAsync       bool
}

// New creates a new extractor service
func New(finder embedder.Finder, opts ...func(*Service)) *Service {
	s := &Service{
		finder:         finder,
		DocsAugmenters: shared.NewMap[string, *DocsAugmenter](),
		indexAsync:     true,
	}
	for _, o := range opts {
		if o != nil {
			o(s)
		}
	}
	return s
}

// WithMCPManager attaches an MCP manager so the augmenter can index mcp: resources.
func WithMCPManager(m *mcpmgr.Manager) func(*Service) { return func(s *Service) { s.mcpMgr = m } }

// WithMCPSnapshotResolver enables snapshot-based MCP reads when available.
func WithMCPSnapshotResolver(resolver mcpfs.SnapshotResolver) func(*Service) {
	return func(s *Service) { s.mcpSnapshotResolver = resolver }
}

// WithMCPSnapshotManifestResolver enables snapshot MD5 manifests when configured.
func WithMCPSnapshotManifestResolver(resolver mcpfs.SnapshotManifestResolver) func(*Service) {
	return func(s *Service) { s.mcpSnapshotManifestResolver = resolver }
}

// WithMCPSnapshotCacheRoot sets the MCP snapshot cache root template.
func WithMCPSnapshotCacheRoot(template string) func(*Service) {
	return func(s *Service) { s.mcpSnapshotCacheRoot = strings.TrimSpace(template) }
}

// WithIndexPathTemplate sets the base path template for Embedius indexes.
func WithIndexPathTemplate(template string) func(*Service) {
	return func(s *Service) { s.indexPathTemplate = strings.TrimSpace(template) }
}

// WithUpstreamSyncConcurrency sets the max number of concurrent upstream syncs.
func WithUpstreamSyncConcurrency(n int) func(*Service) {
	return func(s *Service) {
		if n < 0 {
			n = 0
		}
		s.upstreamSyncConcurrency = n
	}
}

// WithMatchConcurrency sets the max number of concurrent match operations.
func WithMatchConcurrency(n int) func(*Service) {
	return func(s *Service) {
		if n < 0 {
			n = 0
		}
		s.matchConcurrency = n
	}
}

// WithIndexAsync toggles background indexing.
func WithIndexAsync(enabled bool) func(*Service) {
	return func(s *Service) { s.indexAsync = enabled }
}

// LocalRoot binds a non-MCP resource root to an upstream definition.
type LocalRoot struct {
	ID          string
	URI         string
	UpstreamRef string
}

// LocalUpstream defines a database used to sync local/workspace resources.
type LocalUpstream struct {
	Name               string
	Driver             string
	DSN                string
	Shadow             string
	Batch              int
	Force              bool
	Enabled            *bool
	MinIntervalSeconds int
}

// WithLocalUpstreams configures local resource upstreams and root bindings.
func WithLocalUpstreams(roots []LocalRoot, upstreams []LocalUpstream) func(*Service) {
	return func(s *Service) {
		s.localRoots = normalizeLocalRoots(roots)
		s.localUpstreams = normalizeLocalUpstreams(upstreams)
	}
}

// Name returns the service name
func (s *Service) Name() string {
	return name
}

func (s *Service) withIndexPath(ctx context.Context) context.Context {
	if s == nil || strings.TrimSpace(s.indexPathTemplate) == "" {
		return ctx
	}
	if strings.TrimSpace(indexPathTemplateFromContext(ctx)) != "" {
		return ctx
	}
	return WithIndexPathTemplateContext(ctx, s.indexPathTemplate)
}

const (
	augmentDocsMethod = "augmentDocs"
)

// Methods returns the service methods
func (s *Service) Methods() svc.Signatures {
	return []svc.Signature{
		{
			Name:     augmentDocsMethod,
			Internal: true,
			Input:    reflect.TypeOf(&AugmentDocsInput{}),
			Output:   reflect.TypeOf(&AugmentDocsOutput{}),
		},
	}
}

// Method returns the specified method
func (s *Service) Method(name string) (svc.Executable, error) {
	switch name {
	case augmentDocsMethod:
		return s.augmentDocs, nil
	default:
		return nil, svc.NewMethodNotFoundError(name)
	}
}

// augmentDocs processes LLM responses to augmentDocs with embedded context
func (s *Service) augmentDocs(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*AugmentDocsInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*AugmentDocsOutput)
	if !ok {
		return svc.NewInvalidOutputError(output)
	}

	return s.AugmentDocs(ctx, input, output)
}

func (s *Service) AugmentDocs(ctx context.Context, input *AugmentDocsInput, output *AugmentDocsOutput) error {
	input.Init(ctx)
	err := input.Validate(ctx)
	if err != nil {
		return fmt.Errorf("failed to init input: %w", err)
	}
	ctx = s.withIndexPath(ctx)
	augmenter, err := s.getDocAugmenter(ctx, input)
	if err != nil {
		return err
	}
	service := embedius.NewService(augmenter.service)
	var searchDocuments []embSchema.Document

	limit := s.matchConcurrency
	if limit <= 0 {
		limit = 1
	}
	type matchResult struct {
		docs []embSchema.Document
		err  error
	}
	results := make([]matchResult, len(input.Locations))
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, location := range input.Locations {
		i := i
		location := location
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			matchCtx := ctx
			if s.indexAsync {
				matchCtx = embindexer.WithAsyncIndex(matchCtx, true)
			}
			if cfg := s.upstreamSyncConfig(ctx, location, augmenter); cfg != nil {
				matchCtx = embindexer.WithUpstreamSyncConfig(matchCtx, cfg)
			}
			log.Printf("embedius: match start location=%q query=%q max=%d", location, input.Query, input.MaxDocuments)
			docs, err := service.Match(matchCtx, input.Query, input.MaxDocuments, location)
			if err != nil {
				log.Printf("embedius: match error location=%q err=%v", location, err)
				results[i] = matchResult{err: err}
				return
			}
			log.Printf("embedius: match done location=%q docs=%d", location, len(docs))
			results[i] = matchResult{docs: docs}
		}()
	}
	wg.Wait()
	var firstErr error
	successes := 0
	for _, res := range results {
		if res.err != nil {
			if firstErr == nil {
				firstErr = res.err
			}
			continue
		}
		successes++
		searchDocuments = append(searchDocuments, res.docs...)
	}
	if firstErr != nil && (!input.AllowPartial || successes == 0) {
		return fmt.Errorf("failed to augmentDocs documents: %w", firstErr)
	}
	output.Documents = searchDocuments
	output.DocumentsSize = Documents(searchDocuments).Size()

	// Ensure the set for the provided paths or kind
	responseContent := strings.Builder{}

	if input.IncludeFile {
		s.includeDocFileContent(ctx, searchDocuments, input, &responseContent)
	} else {
		s.includeDocuments(output, input, searchDocuments, &responseContent)
	}
	output.Content = responseContent.String()
	return nil
}

func (s *Service) upstreamSyncConfig(ctx context.Context, location string, augmenter *DocsAugmenter) *vectordb.UpstreamSyncConfig {
	if s == nil || augmenter == nil || augmenter.store == nil {
		return nil
	}
	if mcpuri.Is(location) {
		if s.mcpMgr == nil {
			return nil
		}
		root, upstream, ok := s.resolveUpstream(ctx, location)
		if !ok || root == nil || upstream == nil {
			return nil
		}
		if !upstream.Enabled {
			return &vectordb.UpstreamSyncConfig{Enabled: false}
		}
		datasetID := strings.TrimSpace(root.ID)
		if datasetID == "" {
			if augmenter.fsIndexer != nil {
				if ns, err := augmenter.fsIndexer.Namespace(ctx, location); err == nil {
					datasetID = ns
				}
			}
		}
		if strings.TrimSpace(upstream.Driver) == "" || strings.TrimSpace(upstream.DSN) == "" {
			return nil
		}
		if datasetID == "" {
			return nil
		}
		up, err := s.upstreamDB(ctx, upstream)
		if err != nil {
			log.Printf("embedius upstream open failed for %q: %v", root.URI, err)
			return nil
		}
		minInterval := time.Hour
		if upstream.MinIntervalSeconds > 0 {
			minInterval = time.Duration(upstream.MinIntervalSeconds) * time.Second
		}
		return &vectordb.UpstreamSyncConfig{
			Enabled:     true,
			DatasetID:   datasetID,
			UpstreamDB:  up,
			Shadow:      upstream.Shadow,
			BatchSize:   upstream.Batch,
			Force:       upstream.Force,
			Background:  true,
			MinInterval: minInterval,
			LocalShadow: "_vec_emb_docs",
			AssetTable:  "emb_asset",
			Logf:        func(format string, args ...any) { log.Printf(format, args...) },
		}
	}
	root, upstream, ok := s.resolveLocalUpstream(ctx, location)
	if !ok || root == nil || upstream == nil {
		return nil
	}
	if !isLocalUpstreamEnabled(upstream) {
		return &vectordb.UpstreamSyncConfig{Enabled: false}
	}
	datasetID := strings.TrimSpace(root.ID)
	if datasetID == "" {
		if augmenter.fsIndexer != nil {
			if ns, err := augmenter.fsIndexer.Namespace(ctx, location); err == nil {
				datasetID = ns
			}
		}
	}
	if strings.TrimSpace(upstream.Driver) == "" || strings.TrimSpace(upstream.DSN) == "" {
		return nil
	}
	if datasetID == "" {
		return nil
	}
	up, err := s.upstreamDBLocal(ctx, upstream)
	if err != nil {
		log.Printf("embedius upstream open failed for %q: %v", root.URI, err)
		return nil
	}
	minInterval := time.Hour
	if upstream.MinIntervalSeconds > 0 {
		minInterval = time.Duration(upstream.MinIntervalSeconds) * time.Second
	}
	return &vectordb.UpstreamSyncConfig{
		Enabled:     true,
		DatasetID:   datasetID,
		UpstreamDB:  up,
		Shadow:      upstream.Shadow,
		BatchSize:   upstream.Batch,
		Force:       upstream.Force,
		Background:  true,
		MinInterval: minInterval,
		LocalShadow: "_vec_emb_docs",
		AssetTable:  "emb_asset",
		Logf:        func(format string, args ...any) { log.Printf(format, args...) },
	}
}

func (s *Service) upstreamDB(ctx context.Context, upstream *mcpcfg.Upstream) (*sql.DB, error) {
	dsn := upstream.DSN
	if upstream.Secret != nil {
		secSvc := secret.New()
		ref := formatSecretRef(upstream.Secret)
		sec, err := secSvc.Lookup(ctx, secret.Resource(ref))
		if err != nil {
			return nil, fmt.Errorf("upstream %q secret load failed (%s): %w", upstream.Name, ref, err)
		}
		dsn = sec.Expand(dsn)
	}
	conn := view.NewConnector("embedius_upstream", upstream.Driver, dsn)
	db, err := conn.DB()
	if err != nil {
		return nil, err
	}
	if err := s.pingDB(ctx, db); err != nil {
		return nil, err
	}
	return db, nil
}

func formatSecretRef(res *scy.Resource) string {
	if res == nil {
		return ""
	}
	if strings.TrimSpace(res.Key) == "" {
		return strings.TrimSpace(res.URL)
	}
	return strings.TrimSpace(res.URL) + "|" + strings.TrimSpace(res.Key)
}

func (s *Service) pingDB(ctx context.Context, db *sql.DB) error {
	const (
		attempts     = 3
		waitDuration = 2 * time.Second
		pingTimeout  = 5 * time.Second
	)
	var err error
	for i := 0; i < attempts; i++ {
		pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
		err = db.PingContext(pingCtx)
		cancel()
		if err == nil {
			return nil
		}
		if i+1 < attempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitDuration):
			}
		}
	}
	return err
}

func (s *Service) resolveUpstream(ctx context.Context, location string) (*mcpcfg.ResourceRoot, *mcpcfg.Upstream, bool) {
	if s == nil || s.mcpMgr == nil {
		return nil, nil, false
	}
	locServer, locPaths := mcpuri.CompareParts(location)
	if strings.TrimSpace(locServer) == "" || len(locPaths) == 0 {
		return nil, nil, false
	}
	opts, err := s.mcpMgr.Options(ctx, locServer)
	if err != nil || opts == nil {
		log.Printf("embedius upstream resolve failed server=%q err=%v", locServer, err)
		return nil, nil, false
	}
	roots := mcpcfg.ResourceRoots(opts.Metadata)
	if len(roots) == 0 {
		log.Printf("embedius upstream resolve: no roots for server=%q", locServer)
		return nil, nil, false
	}
	upstreams := mcpcfg.Upstreams(opts.Metadata)
	if len(upstreams) == 0 {
		log.Printf("embedius upstream resolve: no upstreams for server=%q", locServer)
		return nil, nil, false
	}
	for _, root := range roots {
		rootServer, rootPaths := mcpuri.CompareParts(root.URI)
		if strings.TrimSpace(rootServer) == "" || len(rootPaths) == 0 {
			continue
		}
		if rootServer != locServer {
			continue
		}
		matched := false
		for _, lp := range locPaths {
			for _, rp := range rootPaths {
				if lp == rp || strings.HasPrefix(lp, rp+"/") {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			if strings.TrimSpace(root.UpstreamRef) == "" {
				return &root, nil, false
			}
			up, ok := upstreams[strings.TrimSpace(root.UpstreamRef)]
			if !ok {
				return &root, nil, false
			}
			return &root, &up, true
		}
	}
	return nil, nil, false
}

func (s *Service) includeDocuments(output *AugmentDocsOutput, input *AugmentDocsInput, searchDocuments []embSchema.Document, responseContent *strings.Builder) {
	documentSize := output.DocumentsSize
	if documentSize < input.MaxResponseSize {
		for _, doc := range searchDocuments {
			loc := input.Location(getStringFromMetadata(doc.Metadata, "path"))
			_, _ = s.addDocumentContent(responseContent, loc, doc.PageContent)
		}

		return
	}

	sizeSoFar := 0
	for _, doc := range searchDocuments {
		if sizeSoFar+Document(doc).Size() >= input.MaxResponseSize {
			break
		}
		sizeSoFar += Document(doc).Size()
		loc := input.Location(getStringFromMetadata(doc.Metadata, "path"))
		_, _ = s.addDocumentContent(responseContent, loc, doc.PageContent)
	}
}

func (s *Service) includeDocFileContent(ctx context.Context, searchResults []embSchema.Document, input *AugmentDocsInput, responseContent *strings.Builder) {
	fs := afs.New()
	documentSize := Documents(searchResults).Size()
	var unique = make(map[string]bool)

	sizeSoFar := 0
	if documentSize < input.MaxResponseSize {
		for _, doc := range searchResults {
			loc := input.Location(getStringFromMetadata(doc.Metadata, "path"))
			if loc != "" {
				if unique[loc] {
					continue
				}
				unique[loc] = true
			}
			var data []byte
			var err error
			if mcpuri.Is(loc) && s.mcpMgr != nil {
				// Read via MCP
				mfs := mcpfs.New(
					s.mcpMgr,
					mcpfs.WithSnapshotResolver(s.mcpSnapshotResolver),
					mcpfs.WithSnapshotManifestResolver(s.mcpSnapshotManifestResolver),
				)
				data, err = mfs.Download(ctx, mcpfs.NewObjectFromURI(loc))
			} else {
				data, err = fs.DownloadWithURL(ctx, loc)
			}
			if err != nil {
				continue
			}
			//TODO add template based output
			if sizeSoFar+len(data) <= input.MaxResponseSize {
				_, _ = s.addDocumentContent(responseContent, loc, string(data))
			} else if sizeSoFar+Document(doc).Size() <= input.MaxResponseSize {
				_, _ = s.addDocumentContent(responseContent, loc, doc.PageContent)
			}

		}
	}
}

func (s *Service) addDocumentContent(response *strings.Builder, loc string, content string) (int, error) {
	return response.WriteString(fmt.Sprintf("file: %v\n```%v\n%v\n````\n\n", loc, strings.Trim(path.Ext(loc), "."), content))
}

// Helper function to safely extract a string from metadata
func getStringFromMetadata(metadata map[string]any, key string) string {
	if value, ok := metadata[key]; ok {
		text, _ := value.(string)
		return text
	}
	return ""
}
