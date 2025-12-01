package resources

import (
	"context"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/viant/afs"
	"github.com/viant/afs/url"
	apiconv "github.com/viant/agently/client/conversation"
	agmodel "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/hygine"
	"github.com/viant/agently/genai/memory"
	aug "github.com/viant/agently/genai/service/augmenter"
	mcpfs "github.com/viant/agently/genai/service/augmenter/mcpfs"
	svc "github.com/viant/agently/genai/tool/service"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
	mcpuri "github.com/viant/agently/internal/mcp/uri"
	"github.com/viant/agently/internal/workspace"
	embopt "github.com/viant/embedius/matching/option"
	embSchema "github.com/viant/embedius/schema"
)

// Name identifies the resources tool service namespace
const Name = "resources"

// Service exposes resource roots, listing, reading and semantic match over filesystem and MCP
type Service struct {
	augmenter *aug.Service
	mcpMgr    *mcpmgr.Manager
	defaults  ResourcesDefaults
	conv      apiconv.Client
	aFinder   agmodel.Finder
	// defaultEmbedder is used when MatchInput.Embedder/Model is not provided.
	defaultEmbedder string
}

// New returns a resources service using a shared augmenter instance.
func New(augmenter *aug.Service, opts ...func(*Service)) *Service {
	s := &Service{augmenter: augmenter}
	for _, o := range opts {
		if o != nil {
			o(s)
		}
	}
	return s
}

// WithMCPManager attaches an MCP manager for listing/downloading MCP
func WithMCPManager(m *mcpmgr.Manager) func(*Service) { return func(s *Service) { s.mcpMgr = m } }

// WithConversationClient attaches a conversation client for context-aware filtering.
func WithConversationClient(c apiconv.Client) func(*Service) { return func(s *Service) { s.conv = c } }

// WithAgentFinder attaches an agent finder to resolve agent resources in context.
func WithAgentFinder(f agmodel.Finder) func(*Service) { return func(s *Service) { s.aFinder = f } }

// WithDefaultEmbedder specifies a default embedder ID to use when the caller
// does not provide one. This typically comes from executor config defaults.
func WithDefaultEmbedder(id string) func(*Service) {
	return func(s *Service) { s.defaultEmbedder = strings.TrimSpace(id) }
}

// Name returns service name
func (s *Service) Name() string { return Name }

// Methods declares available tool methods
func (s *Service) Methods() svc.Signatures {
	return []svc.Signature{
		{Name: "roots", Description: "Discover configured resource roots with optional descriptions", Input: reflect.TypeOf(&RootsInput{}), Output: reflect.TypeOf(&RootsOutput{})},
		{Name: "list", Description: "List resources under a root (file or MCP)", Input: reflect.TypeOf(&ListInput{}), Output: reflect.TypeOf(&ListOutput{})},
		{Name: "read", Description: "Read a single resource under a root", Input: reflect.TypeOf(&ReadInput{}), Output: reflect.TypeOf(&ReadOutput{})},
		{Name: "match", Description: "Semantic match search over one or more roots", Input: reflect.TypeOf(&MatchInput{}), Output: reflect.TypeOf(&MatchOutput{})},
		{Name: "grepFiles", Description: "Search text patterns in files under a root and return per-file snippets.", Input: reflect.TypeOf(&GrepInput{}), Output: reflect.TypeOf(&GrepOutput{})},
	}
}

// Method resolves an executable method by name
func (s *Service) Method(name string) (svc.Executable, error) {
	switch strings.ToLower(name) {
	case "roots":
		return s.roots, nil
	case "list":
		return s.list, nil
	case "read":
		return s.read, nil
	case "match":
		return s.match, nil
	case "grepfiles":
		return s.grepFiles, nil
	default:
		return nil, svc.NewMethodNotFoundError(name)
	}
}

// MatchInput defines parameters for semantic search across one or more roots.
type MatchInput struct {
	Query string `json:"query"`
	// RootURI contains root URIs selected for semantic search. Prefer using
	// RootIDs when possible; RootURI/Roots are retained for compatibility
	// but hidden from public schemas.
	RootURI []string `json:"rootUri,omitempty" internal:"true"`
	// Roots is a legacy alias for RootURI.
	Roots []string `json:"roots,omitempty" internal:"true"`
	// RootIDs contains stable identifiers corresponding to roots returned by
	// roots. When provided, they are resolved to URIs before
	// enforcement and search.
	RootIDs      []string        `json:"rootIds,omitempty" description:"resource root ids returned by roots"`
	Path         string          `json:"path,omitempty"`
	Model        string          `json:"model" internal:"true"`
	MaxDocuments int             `json:"maxDocuments,omitempty" `
	IncludeFile  bool            `json:"includeFile,omitempty" internal:"true"`
	Match        *embopt.Options `json:"match,omitempty"`
	// LimitBytes controls the maximum total bytes of matched content returned for the current cursor page.
	LimitBytes int `json:"limitBytes,omitempty" description:"Max total bytes per page of matched content. Default: 7000."`
	// Cursor selects the page (1..N) over the ranked documents, grouped by LimitBytes.
	Cursor int `json:"cursor,omitempty" description:"Result page selector (1..N). Default: 1."`
}

// MatchOutput mirrors augmenter.AugmentDocsOutput for convenience.
type MatchOutput struct {
	aug.AugmentDocsOutput
	// NextCursor points to the next page (cursor+1) when more content is available; 0 means no further pages.
	NextCursor int `json:"nextCursor,omitempty" description:"Next page cursor when available; 0 when none."`
	// Cursor echoes the selected page.
	Cursor int `json:"cursor,omitempty" description:"Selected page cursor (1..N)."`
	// LimitBytes echoes the applied byte limit per page.
	LimitBytes int `json:"limitBytes,omitempty" description:"Applied byte cap per page."`
}

func (s *Service) match(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*MatchInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*MatchOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	if s.augmenter == nil {
		return fmt.Errorf("augmenter service is not configured")
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return fmt.Errorf("query is required")
	}
	if len(input.RootURI) == 0 && len(input.Roots) == 0 && len(input.RootIDs) == 0 {
		return fmt.Errorf("roots or rootIds is required")
	}
	// Resolve embedder ID: prefer explicit model (internal), then service default.
	input.IncludeFile = true
	embedderID := strings.TrimSpace(input.Model)
	if embedderID == "" {
		embedderID = strings.TrimSpace(s.defaultEmbedder)
	}
	if embedderID == "" {
		return fmt.Errorf("embedder is required (set default embedder in config or provide internal model)")
	}
	// Enforce allowlist when agent context present and normalize roots.
	allowed := s.agentAllowed(ctx) // workspace://... or mcp:

	// Start with any explicit root URIs (including legacy Roots alias).
	sources := make([]string, 0, len(input.RootURI)+len(input.Roots)+len(input.RootIDs))
	sources = append(sources, input.RootURI...)
	sources = append(sources, input.Roots...)
	// Resolve any rootIds into URIs using the agent context.
	for _, id := range input.RootIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		uri, err := s.resolveRootID(ctx, id)
		if err != nil {
			return err
		}
		sources = append(sources, uri)
	}
	locations := make([]string, 0, len(sources))
	for _, root := range sources {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		wsRoot, _, err := s.normalizeUserRoot(ctx, root)
		if err != nil {
			return err
		}
		if len(allowed) > 0 && !isAllowedWorkspace(wsRoot, allowed) {
			return fmt.Errorf("root not allowed: %s", root)
		}
		// Enforce per-resource semantic capability when agent context is
		// available. When no agent or matching resource is found, default to
		// allowing semantic search.
		if !s.semanticAllowedForAgent(ctx, s.currentAgent(ctx), wsRoot) {
			return fmt.Errorf("semantic match not allowed for root: %s", root)
		}
		// Convert to file (for internal I/O) when workspace scheme
		base := wsRoot
		if strings.HasPrefix(wsRoot, "workspace://") {
			base = workspaceToFile(wsRoot)
		}
		if strings.TrimSpace(input.Path) != "" {
			p := strings.TrimSpace(input.Path)
			if isAbsLikePath(p) {
				// For absolute-like paths, ensure they remain under the configured
				// root when the root is file/workspace-backed.
				if !mcpuri.Is(wsRoot) {
					rootBase := base
					if strings.HasPrefix(rootBase, "file://") {
						rootBase = fileURLToPath(rootBase)
					}
					pathBase := p
					if strings.HasPrefix(pathBase, "file://") {
						pathBase = fileURLToPath(pathBase)
					}
					if !isUnderRootPath(pathBase, rootBase) {
						return fmt.Errorf("path %s is outside root %s", p, root)
					}
				}
				base = p
			} else {
				base = url.Join(base, strings.TrimPrefix(p, "/"))
			}
		}
		locations = append(locations, base)
	}
	if len(locations) == 0 {
		return fmt.Errorf("no valid roots provided")
	}
	trimPrefix := commonPrefix(locations)
	// Map to augmenter input and delegate
	augIn := &aug.AugmentDocsInput{
		Query:        query,
		Locations:    locations,
		Match:        input.Match,
		Model:        embedderID,
		MaxDocuments: input.MaxDocuments,
		IncludeFile:  input.IncludeFile,
		TrimPath:     trimPrefix,
	}
	var augOut aug.AugmentDocsOutput
	if err := s.augmenter.AugmentDocs(ctx, augIn, &augOut); err != nil {
		return err
	}

	// Order by match score descending
	sort.SliceStable(augOut.Documents, func(i, j int) bool { return augOut.Documents[i].Score > augOut.Documents[j].Score })

	for i := range augOut.Documents {
		doc := augOut.Documents[i]
		// Surface score in metadata for clients (in addition to struct field)
		if doc.Metadata == nil {
			doc.Metadata = map[string]any{}
		}
		doc.Metadata["score"] = doc.Score
		for _, key := range []string{"path", "docId", "fragmentId"} {
			if path, ok := doc.Metadata[key]; ok {
				doc.Metadata[key] = fileToWorkspace(path.(string))
			}
		}
	}

	// Apply byte-limited pagination for presentation
	limit := effectiveLimitBytes(input.LimitBytes)
	cursor := effectiveCursor(input.Cursor)
	pageDocs, hasNext := selectDocPage(augOut.Documents, limit, cursor, trimPrefix)

	// If the total formatted size of all documents does not exceed the limit,
	// there is no next page regardless of internal grouping.
	if total := totalFormattedBytes(augOut.Documents, trimPrefix); total <= limit {
		hasNext = false
	}

	// Rebuild Content for selected page using same format as augmenter
	var b strings.Builder
	for _, doc := range pageDocs {
		loc := strings.TrimPrefix(getStringFromMetadata(doc.Metadata, "path"), trimPrefix)
		if loc == "" {
			loc = getStringFromMetadata(doc.Metadata, "docId")
		}
		// Use local helper mirroring augmenter formatting
		_, _ = b.WriteString(formatDocument(loc, doc.PageContent))
	}

	// Populate output with paged content
	output.AugmentDocsOutput.Content = b.String()
	output.AugmentDocsOutput.Documents = pageDocs
	output.AugmentDocsOutput.DocumentsSize = augmenterDocumentsSize(pageDocs)
	output.Cursor = cursor
	output.LimitBytes = limit
	if hasNext {
		output.NextCursor = cursor + 1
	}
	return nil
}

// effectiveLimitBytes returns the per-page byte cap (default 7000, upper bound 200k)
func effectiveLimitBytes(limit int) int {
	if limit <= 0 {
		return 7000
	}
	if limit > 200000 {
		return 200000
	}
	return limit
}

// effectiveCursor normalizes the page cursor (1..N)
func effectiveCursor(cursor int) int {
	if cursor <= 0 {
		return 1
	}
	return cursor
}

// selectDocPage splits documents into pages of at most limitBytes (based on formatted content length) and returns the selected page.
func selectDocPage(docs []embSchema.Document, limitBytes int, cursor int, trimPrefix string) ([]embSchema.Document, bool) {
	if limitBytes <= 0 || len(docs) == 0 {
		return nil, false
	}
	// Build pages iteratively using formatted size to match presentation size
	pages := make([][]embSchema.Document, 0, 4)
	var cur []embSchema.Document
	used := 0
	for _, d := range docs {
		loc := strings.TrimPrefix(getStringFromMetadata(d.Metadata, "path"), trimPrefix)
		if loc == "" {
			loc = getStringFromMetadata(d.Metadata, "docId")
		}
		formatted := formatDocument(loc, d.PageContent)
		fragBytes := len(formatted)
		if fragBytes > limitBytes {
			if len(cur) > 0 {
				pages = append(pages, cur)
				cur = nil
				used = 0
			}
			pages = append(pages, []embSchema.Document{d})
			continue
		}
		if used+fragBytes > limitBytes {
			pages = append(pages, cur)
			cur = nil
			used = 0
		}
		cur = append(cur, d)
		used += fragBytes
	}
	if len(cur) > 0 {
		pages = append(pages, cur)
	}
	if len(pages) == 0 {
		return nil, false
	}
	if cursor < 1 {
		cursor = 1
	}
	if cursor > len(pages) {
		cursor = len(pages)
	}
	sel := pages[cursor-1]
	hasNext := cursor < len(pages)
	return sel, hasNext
}

// formatDocument mirrors augmenter.addDocumentContent formatting.
func formatDocument(loc string, content string) string {
	ext := strings.Trim(pathpkg.Ext(loc), ".")
	return fmt.Sprintf("file: %v\n```%v\n%v\n````\n\n", loc, ext, content)
}

// augmenterDocumentsSize computes combined size using augmenter.Document.Size()
func augmenterDocumentsSize(docs []embSchema.Document) int {
	total := 0
	for _, d := range docs {
		total += aug.Document(d).Size()
	}
	return total
}

// getStringFromMetadata safely extracts a string value from metadata map.
func getStringFromMetadata(metadata map[string]any, key string) string {
	if value, ok := metadata[key]; ok {
		if text, ok := value.(string); ok {
			return text
		}
	}
	return ""
}

// totalFormattedBytes sums the presentation bytes across all documents,
// matching the formatting used in Content output.
func totalFormattedBytes(docs []embSchema.Document, trimPrefix string) int {
	total := 0
	for _, d := range docs {
		loc := strings.TrimPrefix(getStringFromMetadata(d.Metadata, "path"), trimPrefix)
		if loc == "" {
			loc = getStringFromMetadata(d.Metadata, "docId")
		}
		total += len(formatDocument(loc, d.PageContent))
	}
	return total
}

// isAbsLikePath reports whether the provided path looks like an absolute
// location (filesystem path, file:// URL, or mcp: URI). In such cases callers
// should avoid blindly joining it onto an existing root URI.
func isAbsLikePath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	if strings.HasPrefix(p, "/") {
		return true
	}
	if strings.HasPrefix(p, "file://") {
		return true
	}
	if mcpuri.Is(p) {
		return true
	}
	return false
}

// fileURLToPath converts a file:// URL into a filesystem path. Non-file
// URLs are returned unchanged.
func fileURLToPath(u string) string {
	u = strings.TrimSpace(u)
	if !strings.HasPrefix(u, "file://") {
		return u
	}
	rest := strings.TrimPrefix(u, "file://")
	rest = strings.TrimPrefix(rest, "localhost")
	if !strings.HasPrefix(rest, "/") {
		rest = "/" + rest
	}
	return rest
}

// isUnderRootPath reports whether the given location lies under the provided
// root. It supports both filesystem paths and URL-like locations (file://,
// s3://, gs://, etc.) by normalizing to comparable path segments.
func isUnderRootPath(path, root string) bool {
	path = strings.TrimSpace(path)
	root = strings.TrimSpace(root)
	if path == "" || root == "" {
		return false
	}
	// URL-like (file://, s3://, gs://, etc.): compare normalized URL paths.
	if strings.Contains(root, "://") {
		rootPath := pathpkg.Clean("/" + strings.TrimLeft(url.Path(root), "/"))
		pathPath := pathpkg.Clean("/" + strings.TrimLeft(url.Path(path), "/"))
		if rootPath == "/" {
			return true
		}
		// Treat exact equality as under-root.
		if pathPath == rootPath {
			return true
		}
		if !strings.HasSuffix(rootPath, "/") {
			rootPath += "/"
		}
		return strings.HasPrefix(pathPath, rootPath)
	}
	// Filesystem paths: use OS-aware filepath semantics.
	pathFS := filepath.Clean(path)
	rootFS := filepath.Clean(root)
	if rootFS == string(os.PathSeparator) {
		return true
	}
	// Treat exact equality as under-root.
	if pathFS == rootFS {
		return true
	}
	if !strings.HasSuffix(rootFS, string(os.PathSeparator)) {
		rootFS += string(os.PathSeparator)
	}
	return strings.HasPrefix(pathFS, rootFS)
}

// -------- list implementation --------

type ListInput struct {
	// RootURI is the normalized or user-provided root URI. Prefer using
	// RootID when possible; RootURI is retained for backward compatibility
	// but hidden from public schemas.
	RootURI string `json:"root,omitempty" internal:"true"`
	// RootID is a stable identifier corresponding to a root returned by
	// roots. When provided, it is resolved to the underlying
	// normalized URI before enforcement and listing.
	RootID    string `json:"rootId,omitempty"`
	Path      string `json:"path,omitempty"`
	Recursive bool   `json:"recursive,omitempty"`
	MaxItems  int    `json:"maxItems,omitempty"`
}

type ListItem struct {
	URI      string    `json:"uri"`
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

type ListOutput struct {
	Items []ListItem `json:"items"`
	Total int        `json:"total"`
}

func (s *Service) list(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*ListInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*ListOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	rootURI := strings.TrimSpace(input.RootURI)
	if rootURI == "" && strings.TrimSpace(input.RootID) != "" {
		var err error
		rootURI, err = s.resolveRootID(ctx, input.RootID)
		if err != nil {
			return err
		}
	}
	if rootURI == "" {
		return fmt.Errorf("root or rootId is required")
	}
	allowed := s.agentAllowed(ctx) // workspace://... or mcp:
	wsRoot, _, err := s.normalizeUserRoot(ctx, rootURI)
	if err != nil {
		return err
	}
	if len(allowed) > 0 && !isAllowedWorkspace(wsRoot, allowed) {
		return fmt.Errorf("root not allowed: %s", rootURI)
	}
	// Use file base for AFS when workspace scheme
	normRoot := wsRoot
	if strings.HasPrefix(wsRoot, "workspace://") {
		normRoot = workspaceToFile(wsRoot)
	}
	base := normRoot
	if strings.TrimSpace(input.Path) != "" {
		p := strings.TrimSpace(input.Path)
		if isAbsLikePath(p) {
			// For absolute-like paths/URIs, ensure they remain under the
			// configured root when the root is file/workspace-backed.
			if !mcpuri.Is(wsRoot) {
				rootBase := normRoot
				if strings.HasPrefix(rootBase, "file://") {
					rootBase = fileURLToPath(rootBase)
				}
				pathBase := p
				if strings.HasPrefix(pathBase, "file://") {
					pathBase = fileURLToPath(pathBase)
				}
				if !isUnderRootPath(pathBase, rootBase) {
					return fmt.Errorf("path %s is outside root %s", p, rootURI)
				}
			}
			// Use the path as-is instead of joining onto the normalized
			// root to avoid duplicated prefixes such as
			// /Users/.../Users/....
			base = p
		} else {
			base = url.Join(normRoot, strings.TrimPrefix(p, "/"))
		}
	}
	afsSvc := afs.New()
	mfs := (*mcpfs.Service)(nil)
	if s.mcpMgr != nil {
		mfs = mcpfs.New(s.mcpMgr)
	}
	max := input.MaxItems
	if max <= 0 {
		max = 0
	}
	var items []ListItem
	seen := map[string]bool{}
	if mcpuri.Is(base) {
		if mfs == nil {
			return fmt.Errorf("mcp manager not configured")
		}
		objs, err := mfs.List(ctx, base)
		if err != nil {
			return err
		}
		for _, o := range objs {
			if o == nil {
				continue
			}
			uri := o.URL()
			if seen[uri] {
				continue
			}
			seen[uri] = true
			items = append(items, ListItem{
				URI:      uri,
				Path:     relativePath(normRoot, uri),
				Name:     o.Name(),
				Size:     o.Size(),
				Modified: o.ModTime(),
			})
			if max > 0 && len(items) >= max {
				break
			}
		}
	} else {
		if input.Recursive {
			err := afsSvc.Walk(ctx, base, func(ctx context.Context, walkBaseURL, parent string, info os.FileInfo, reader io.Reader) (bool, error) {
				if info == nil || info.IsDir() {
					return true, nil
				}
				var uri string
				if parent == "" {
					uri = url.Join(walkBaseURL, info.Name())
				} else {
					uri = url.Join(walkBaseURL, parent, info.Name())
				}
				if seen[uri] {
					return true, nil
				}
				seen[uri] = true
				items = append(items, ListItem{
					URI:      uri,
					Path:     relativePath(normRoot, uri),
					Name:     info.Name(),
					Size:     info.Size(),
					Modified: info.ModTime(),
				})
				if max > 0 && len(items) >= max {
					return false, nil
				}
				return true, nil
			})
			if err != nil {
				return err
			}
		} else {
			objs, err := afsSvc.List(ctx, base)
			if err != nil {
				return err
			}
			for _, o := range objs {
				if o == nil || o.IsDir() {
					continue
				}
				uri := url.Join(base, o.Name())
				if seen[uri] {
					continue
				}
				seen[uri] = true
				items = append(items, ListItem{
					URI:      uri,
					Path:     relativePath(normRoot, uri),
					Name:     o.Name(),
					Size:     o.Size(),
					Modified: o.ModTime(),
				})
				if max > 0 && len(items) >= max {
					break
				}
			}
		}
	}
	output.Items = items
	output.Total = len(items)
	return nil
}

// ReadInput describes a request to read a single resource.
// Callers can either supply root+path (preferred for root-centric flows) or a
// fully qualified URI (as returned by list).
type ReadInput struct {
	// RootURI is the normalized or user-provided root URI. Prefer using
	// RootID when possible; RootURI is retained for backward compatibility
	// but hidden from public schemas.
	RootURI string `json:"root,omitempty" internal:"true"`
	// RootID is a stable identifier corresponding to a root returned by
	// roots. When provided (and URI is empty), it is resolved to
	// the underlying normalized URI before enforcement and reading.
	RootID   string `json:"rootId,omitempty"`
	Path     string `json:"path,omitempty"`
	URI      string `json:"uri,omitempty"`
	MaxBytes int    `json:"maxBytes,omitempty"`

	// OffsetBytes and LengthBytes describe a 0-based byte window within the
	// file. When StartLine is > 0, the line range takes precedence and the
	// byte range is ignored. When both byte and line ranges are unset, the
	// full (optionally MaxBytes-truncated) content is returned.
	OffsetBytes int64 `json:"offsetBytes,omitempty"` // 0-based byte offset; default 0
	LengthBytes int   `json:"lengthBytes,omitempty"` // bytes to read; 0 => use MaxBytes cap

	// StartLine and LineCount describe a 1-based line slice. When StartLine > 0
	// this mode is used in preference to the byte range.
	StartLine int `json:"startLine,omitempty"` // 1-based start line
	LineCount int `json:"lineCount,omitempty"` // number of lines; 0 => until EOF or MaxBytes

	// Mode controls how the line slice is interpreted when StartLine > 0.
	// Supported values:
	//   - "" or "slice"      : simple range [StartLine, StartLine+LineCount)
	//   - "indentation"      : indentation-aware code block around StartLine
	Mode string `json:"mode,omitempty" description:"read mode: 'slice' (default) for simple line ranges or 'indentation' for indentation-aware code blocks"`
}

// ReadOutput contains the resolved URI, relative path and optionally truncated content.
type ReadOutput struct {
	URI     string `json:"uri"`
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int    `json:"size"`
	// StartLine and EndLine are 1-based line numbers describing the selected
	// slice when Offset/Limit were provided. They are zero when the entire
	// (possibly MaxBytes-truncated) file content is returned.
	StartLine int `json:"startLine,omitempty"`
	EndLine   int `json:"endLine,omitempty"`
}

func (s *Service) read(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*ReadInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*ReadOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	rootURI := strings.TrimSpace(input.RootURI)
	pathPart := strings.TrimSpace(input.Path)
	uri := strings.TrimSpace(input.URI)
	allowed := s.agentAllowed(ctx)

	var (
		normRoot string
		fullURI  string
	)

	if uri != "" {
		// URI-only mode: enforce allowlist (when present) against the URI and
		// normalize it for reading.
		wsRoot, _, err := s.normalizeUserRoot(ctx, uri)
		if err != nil {
			return err
		}
		if len(allowed) > 0 && !isAllowedWorkspace(wsRoot, allowed) {
			return fmt.Errorf("resource not allowed: %s", uri)
		}
		fullURI = wsRoot
		if strings.HasPrefix(wsRoot, "workspace://") {
			fullURI = workspaceToFile(wsRoot)
		}
	} else {
		// Root+path mode
		if rootURI == "" && strings.TrimSpace(input.RootID) != "" {
			var err error
			rootURI, err = s.resolveRootID(ctx, input.RootID)
			if err != nil {
				return err
			}
		}
		if rootURI == "" {
			return fmt.Errorf("root or rootId or uri is required")
		}
		if pathPart == "" {
			return fmt.Errorf("path is required when uri is not provided")
		}
		wsRoot, _, err := s.normalizeUserRoot(ctx, rootURI)
		if err != nil {
			return err
		}
		if len(allowed) > 0 && !isAllowedWorkspace(wsRoot, allowed) {
			return fmt.Errorf("root not allowed: %s", rootURI)
		}
		normRoot = wsRoot
		if strings.HasPrefix(wsRoot, "workspace://") {
			normRoot = workspaceToFile(wsRoot)
		}
		p := pathPart
		if isAbsLikePath(p) {
			// For absolute filesystem paths, ensure they remain under the
			// configured root when the root is file/workspace-backed.
			if !mcpuri.Is(wsRoot) {
				rootBase := normRoot
				if strings.HasPrefix(rootBase, "file://") {
					rootBase = fileURLToPath(rootBase)
				}
				pathBase := p
				if strings.HasPrefix(pathBase, "file://") {
					pathBase = fileURLToPath(pathBase)
				}
				if !isUnderRootPath(pathBase, rootBase) {
					return fmt.Errorf("path %s is outside root %s", p, rootURI)
				}
			}
			// Convert to a file:// URL so that downstream readers operate on a
			// normalized URI instead of accidentally joining workspace and
			// filesystem prefixes.
			if strings.HasPrefix(p, "file://") || mcpuri.Is(p) {
				fullURI = p
			} else {
				fullURI = url.Join("file://localhost/", strings.TrimPrefix(p, "/"))
			}
		} else {
			fullURI = url.Join(normRoot, strings.TrimPrefix(p, "/"))
		}
	}
	var data []byte
	var err error
	if mcpuri.Is(fullURI) {
		if s.mcpMgr == nil {
			return fmt.Errorf("mcp manager not configured")
		}
		mfs := mcpfs.New(s.mcpMgr)
		data, err = mfs.Download(ctx, mcpfs.NewObjectFromURI(fullURI))
	} else {
		fs := afs.New()
		data, err = fs.DownloadWithURL(ctx, fullURI)
	}
	if err != nil {
		return err
	}
	originalSize := len(data)
	if input.MaxBytes > 0 && len(data) > input.MaxBytes {
		data = data[:input.MaxBytes]
	}
	text := string(data)
	startLine := 0
	endLine := 0

	// Prefer line-slice mode when StartLine is specified.
	if input.StartLine > 0 {
		mode := strings.ToLower(strings.TrimSpace(input.Mode))
		lines := strings.SplitAfter(text, "\n")
		total := len(lines)
		off := input.StartLine
		if off <= 0 {
			off = 1
		}
		if off > total {
			// No content in the requested range; return empty slice with size metadata.
			text = ""
			startLine = off
			endLine = off - 1
		} else if mode == "indentation" {
			startIdx, endIdx := indentationBlock(lines, off-1, input.LineCount)
			if startIdx < 0 || endIdx <= startIdx || startIdx >= total {
				text = ""
				startLine = off
				endLine = off - 1
			} else {
				if endIdx > total {
					endIdx = total
				}
				var b strings.Builder
				for _, ln := range lines[startIdx:endIdx] {
					_, _ = b.WriteString(ln)
				}
				text = b.String()
				startLine = startIdx + 1
				endLine = endIdx
			}
		} else {
			// default: simple slice mode
			limit := input.LineCount
			if limit <= 0 {
				limit = total - off + 1
			}
			startIdx := off - 1
			endIdx := startIdx + limit
			if endIdx > total {
				endIdx = total
			}
			var b strings.Builder
			for _, ln := range lines[startIdx:endIdx] {
				_, _ = b.WriteString(ln)
			}
			text = b.String()
			startLine = off
			endLine = startIdx + (endIdx - startIdx)
		}
	} else if input.OffsetBytes > 0 || input.LengthBytes > 0 {
		// Byte-range mode: apply offset/length over the MaxBytes-truncated buffer.
		off := input.OffsetBytes
		if off < 0 {
			off = 0
		}
		if off >= int64(len(data)) {
			text = ""
		} else {
			end := len(data)
			if input.LengthBytes > 0 {
				candidate := int(off) + input.LengthBytes
				if candidate < end {
					end = candidate
				}
			}
			if off < int64(end) {
				text = string(data[off:end])
			} else {
				text = ""
			}
		}
	}
	output.URI = fullURI
	if normRoot != "" {
		output.Path = relativePath(normRoot, fullURI)
	} else {
		output.Path = fullURI
	}
	output.Content = text
	output.Size = originalSize
	output.StartLine = startLine
	output.EndLine = endLine
	return nil
}

func relativePath(rootURI, fullURI string) string {
	root := strings.TrimSuffix(strings.TrimSpace(rootURI), "/")
	uri := strings.TrimSpace(fullURI)
	if root == "" || uri == "" {
		return ""
	}
	if !strings.HasPrefix(uri, root) {
		return uri
	}
	rel := strings.TrimPrefix(uri[len(root):], "/")
	return rel
}

// indentationBlock computes a best-effort indentation-aware block around the
// given anchor line index. It returns start and end indices (end exclusive)
// into the provided lines slice. When lineCount > 0, it is treated as a
// maximum number of lines for the block.
func indentationBlock(lines []string, anchorIdx int, lineCount int) (int, int) {
	total := len(lines)
	if total == 0 || anchorIdx < 0 || anchorIdx >= total {
		return -1, -1
	}
	// Move anchor down to first non-blank line when possible.
	for anchorIdx < total && isBlankLine(lines[anchorIdx]) {
		anchorIdx++
	}
	if anchorIdx >= total {
		return -1, -1
	}
	indentLevels := make([]int, total)
	for i, ln := range lines {
		indentLevels[i] = indentLevel(ln)
	}
	anchorIndent := indentLevels[anchorIdx]
	if anchorIndent < 0 {
		anchorIndent = 0
	}
	start := anchorIdx
	// Scan upwards to find the first line that belongs to this block. We stop
	// when we encounter a non-blank line with indentation strictly less than
	// the anchor indentation.
	for i := anchorIdx - 1; i >= 0; i-- {
		if isBlankLine(lines[i]) {
			continue
		}
		if indentLevels[i] < anchorIndent {
			break
		}
		start = i
	}
	end := anchorIdx + 1
	// Scan downwards until indentation drops below the anchor indentation or
	// we reach the end of the file.
	for i := anchorIdx + 1; i < total; i++ {
		if isBlankLine(lines[i]) {
			end = i + 1
			continue
		}
		if indentLevels[i] < anchorIndent {
			break
		}
		end = i + 1
	}
	// Apply optional lineCount cap when provided.
	if lineCount > 0 && end > start+lineCount {
		end = start + lineCount
	}
	return start, end
}

// indentLevel returns a simple indentation level (number of leading spaces
// and tabs, treating a tab as 4 spaces).
func indentLevel(line string) int {
	level := 0
	for _, r := range line {
		if r == ' ' {
			level++
			continue
		}
		if r == '\t' {
			level += 4
			continue
		}
		break
	}
	return level
}

func isBlankLine(line string) bool {
	return strings.TrimSpace(line) == ""
}

func commonPrefix(values []string) string {
	if len(values) == 0 {
		return ""
	}
	prefix := values[0]
	for _, v := range values[1:] {
		for !strings.HasPrefix(v, prefix) && prefix != "" {
			prefix = prefix[:len(prefix)-1]
		}
		if prefix == "" {
			break
		}
	}
	// Avoid cutting in the middle of a path segment when possible.
	if i := strings.LastIndex(prefix, "/"); i > 0 {
		return prefix[:i+1]
	}
	return prefix
}

func (s *Service) isAllowed(loc string, allowed []string) bool {
	return isAllowedWorkspace(loc, allowed)
}

func isAllowedWorkspace(loc string, allowed []string) bool {
	u := strings.TrimSpace(loc)
	if u == "" {
		return false
	}
	// Compare canonical workspace:// or mcp: prefixes
	for _, a := range allowed {
		if strings.HasPrefix(u, strings.TrimRight(a, "/")) {
			return true
		}
	}
	return false
}

// -------- roots implementation --------

type ResourcesDefaults struct {
	Locations    []string
	TrimPath     string
	SummaryFiles []string
}

// WithDefaults configures default roots and presentation hints.
func WithDefaults(d ResourcesDefaults) func(*Service) { return func(s *Service) { s.defaults = d } }

type RootsInput struct {
	MaxRoots int `json:"maxRoots,omitempty"`
}

type Root struct {
	// ID is a stable identifier for this root when available. When the
	// underlying agent resource entry defines an explicit id, it is surfaced
	// here. Otherwise, the normalized URI is used as a fallback id so callers
	// can still use rootId as an alias for the URI.
	ID string `json:"id"`

	URI         string `json:"uri"`
	Description string `json:"description,omitempty"`
	// AllowedSemanticSearch reports whether semantic match (match)
	// is permitted for this root in the current agent configuration.
	AllowedSemanticSearch bool `json:"allowedSemanticSearch"`
	// AllowedGrepSearch reports whether lexical grep (grepFiles)
	// is permitted for this root in the current agent configuration.
	AllowedGrepSearch bool `json:"allowedGrepSearch"`
}

type RootsOutput struct {
	Roots []Root `json:"roots"`
}

func (s *Service) roots(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*RootsInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*RootsOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	max := input.MaxRoots
	if max < 0 {
		max = 0
	}

	// Prefer agent-scoped resources when available; otherwise fall back to defaults
	locs := s.agentAllowed(ctx)
	if len(locs) == 0 {
		locs = append([]string(nil), s.defaults.Locations...)
	}
	if len(locs) == 0 {
		output.Roots = nil
		return nil
	}
	var roots []Root
	seen := map[string]bool{}
	curAgent := s.currentAgent(ctx)
	for _, loc := range locs {
		norm, kind, err := s.normalizeUserRoot(ctx, loc)
		if err != nil {
			continue
		}
		if norm == "" {
			continue
		}
		if seen[norm] {
			continue
		}
		seen[norm] = true
		desc := s.tryDescribe(ctx, norm, kind)
		rootID := ""
		skip := false
		// Prefer an explicit agent resource description when available, and
		// identify any system-scoped resources that should not be surfaced as
		// browseable roots (e.g. systemKnowledge backing files).
		if curAgent != nil && len(curAgent.Resources) > 0 {
			for _, r := range curAgent.Resources {
				if r == nil || strings.TrimSpace(r.URI) == "" {
					continue
				}
				normRes, _, err := s.normalizeUserRoot(ctx, r.URI)
				if err != nil || strings.TrimSpace(normRes) == "" {
					continue
				}
				if strings.TrimRight(strings.TrimSpace(normRes), "/") == strings.TrimRight(strings.TrimSpace(norm), "/") {
					// Hide system-role resources (systemKnowledge, etc.) from
					// roots so they are not offered as browseable
					// roots to coding agents.
					if strings.EqualFold(strings.TrimSpace(r.Role), "system") {
						skip = true
						break
					}
					if strings.TrimSpace(r.ID) != "" {
						rootID = strings.TrimSpace(r.ID)
					}
					if strings.TrimSpace(r.Description) != "" {
						desc = strings.TrimSpace(r.Description)
					}
					break
				}
			}
		}
		if skip {
			continue
		}
		if strings.TrimSpace(rootID) == "" {
			// Fallback: use normalized URI as an implicit id. This keeps
			// behaviour backward compatible while still allowing callers to
			// pass the id into rootId parameters.
			rootID = norm
		}
		semAllowed := s.semanticAllowedForAgent(ctx, curAgent, norm)
		grepAllowed := s.grepAllowedForAgent(ctx, curAgent, norm)
		roots = append(roots, Root{
			ID:                    rootID,
			URI:                   norm,
			Description:           desc,
			AllowedSemanticSearch: semAllowed,
			AllowedGrepSearch:     grepAllowed,
		})
		if max > 0 && len(roots) >= max {
			break
		}
	}
	output.Roots = roots
	return nil
}

func (s *Service) normalizeLocation(loc string) (string, string) {
	u := strings.TrimSpace(loc)
	if u == "" {
		return "", ""
	}
	if mcpuri.Is(u) {
		return u, "mcp"
	}
	// Resolve relative to workspace root, then to file:// URL
	base := workspace.Root()
	if strings.HasPrefix(u, "file://") {
		return u, "file"
	}
	// Pass-through other AFS-supported schemes like s3://, gs://, http(s)://, etc.
	if idx := strings.Index(u, "://"); idx > 0 {
		return u, "file"
	}
	if strings.HasPrefix(u, "/") {
		return "file://" + u, "file"
	}
	// treat as relative path
	abs := base
	if !strings.HasSuffix(abs, "/") {
		abs += "/"
	}
	return "file://" + abs + u, "file"
}

// resolveRootID maps a logical root id to its normalized root URI within the
// current agent context. It prefers explicit ids declared on agent resources
// and, for backward compatibility, falls back to interpreting the id as a
// URI only when it already looks like a URI (contains a scheme).
func (s *Service) resolveRootID(ctx context.Context, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("rootId is empty")
	}
	curAgent := s.currentAgent(ctx)
	if curAgent != nil {
		for _, r := range curAgent.Resources {
			if r == nil {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(r.ID), id) {
				norm, _, err := s.normalizeUserRoot(ctx, r.URI)
				if err != nil {
					return "", err
				}
				if strings.TrimSpace(norm) == "" {
					break
				}
				return norm, nil
			}
		}
	}
	// Fallback: only treat id as a URI when it already looks like one
	// (e.g., workspace://..., file://..., s3://..., mcp:...). This preserves
	// legacy configurations that used raw URIs as ids, while avoiding
	// accidentally mapping human-friendly ids like "local" into workspace
	// roots (e.g., workspace://localhost/local).
	if strings.Contains(id, "://") || mcpuri.Is(id) || strings.HasPrefix(id, "workspace://") || strings.HasPrefix(id, "file://") {
		norm, _, err := s.normalizeUserRoot(ctx, id)
		if err != nil {
			return "", fmt.Errorf("unknown rootId %s: %w", id, err)
		}
		if strings.TrimSpace(norm) == "" {
			return "", fmt.Errorf("unknown rootId: %s", id)
		}
		return norm, nil
	}
	return "", fmt.Errorf("unknown rootId: %s", id)
}

// normalizeUserRoot enforces workspace:// or mcp: for resources tools.
// - workspace kinds (e.g., agents/...) => workspace://localhost/<input>
// - relative => agents/<agentId>/<input>, else <workspace>/<input>
// - mcp: passthrough
// - file:// absolute under workspace => mapped to workspace://
// - others => error
func (s *Service) normalizeUserRoot(ctx context.Context, in string) (string, string, error) {
	u := strings.TrimSpace(in)
	if u == "" {
		return "", "", nil
	}
	if mcpuri.Is(u) {
		return u, "mcp", nil
	}
	if strings.HasPrefix(u, "workspace://") {
		// Normalize agent segment casing when path starts with agents/
		rel := strings.TrimPrefix(u, "workspace://")
		rel = strings.TrimPrefix(rel, "localhost/")
		low := strings.ToLower(rel)
		if strings.HasPrefix(low, workspace.KindAgent+"/") {
			// Extract agent id segment and remainder
			seg := rel[len(workspace.KindAgent)+1:]
			agentID := seg
			rest := ""
			if i := strings.Index(seg, "/"); i != -1 {
				agentID = seg[:i]
				rest = seg[i+1:]
			}
			agentID = strings.ToLower(strings.TrimSpace(agentID))
			if rest != "" {
				return url.Join("workspace://localhost/", workspace.KindAgent, agentID, rest), "workspace", nil
			}
			return url.Join("workspace://localhost/", workspace.KindAgent, agentID), "workspace", nil
		}
		return u, "workspace", nil
	}
	if strings.HasPrefix(u, "file://") {
		// For file:// URIs, accept the value as-is and treat it as a file
		// root. When the URI happens to be under the current workspace root,
		// other helpers may still map it to workspace:// for internal use, but
		// we no longer reject file:// URIs that live outside the workspace.
		return u, "file", nil
	}
	// known workspace kinds
	lower := strings.ToLower(u)
	kinds := []string{
		workspace.KindAgent + "/",
		workspace.KindModel + "/",
		workspace.KindEmbedder + "/",
		workspace.KindMCP + "/",
		workspace.KindWorkflow + "/",
		workspace.KindTool + "/",
		workspace.KindOAuth + "/",
		workspace.KindFeeds + "/",
		workspace.KindA2A + "/",
	}
	for _, pfx := range kinds {
		if strings.HasPrefix(lower, pfx) {
			// Normalize prefix to canonical lowercase kind and, for agents, normalize the agent id segment
			rel := u
			if len(u) >= len(pfx) {
				rel = u[len(pfx):]
			}
			if pfx == workspace.KindAgent+"/" {
				// Ensure agent folder matches the canonical (lowercase) agent id to align with workspace layout
				agentSeg := rel
				rest := ""
				if i := strings.Index(agentSeg, "/"); i != -1 {
					rest = agentSeg[i+1:]
					agentSeg = agentSeg[:i]
				}
				agentSeg = strings.ToLower(strings.TrimSpace(agentSeg))
				if rest != "" {
					return url.Join("workspace://localhost/", workspace.KindAgent, agentSeg, rest), "workspace", nil
				}
				return url.Join("workspace://localhost/", workspace.KindAgent, agentSeg), "workspace", nil
			}
			// Other kinds: keep remainder as-is, normalize kind to canonical lowercase
			return url.Join("workspace://localhost/", pfx+rel), "workspace", nil
		}
	}
	// relative: resolve under the current workspace root
	return url.Join("workspace://localhost/", u), "workspace", nil
}

// (duplicate removed above)

func (s *Service) defaultLabel(uri, kind string) string {
	if kind == "mcp" {
		// mcp:server:/prefix -> server: prefix
		parts := strings.SplitN(strings.TrimPrefix(uri, "mcp:"), ":", 2)
		if len(parts) == 2 {
			return parts[0] + ": " + strings.TrimPrefix(parts[1], "/")
		}
		return uri
	}
	// file://... -> base folder name
	base := uri
	if i := strings.LastIndex(uri, "/"); i != -1 && i+1 < len(uri) {
		base = uri[i+1:]
	}
	return strings.TrimSuffix(base, "/")
}

func (s *Service) tryDescribe(ctx context.Context, uri, kind string) string {
	order := s.defaults.SummaryFiles
	if len(order) == 0 {
		order = []string{".summary", ".summary.md", "README.md"}
	}
	if kind == "mcp" {
		if s.mcpMgr == nil {
			return ""
		}
		mfs := mcpfs.New(s.mcpMgr)
		for _, name := range order {
			p := url.Join(uri, name)
			data, err := mfs.Download(ctx, mcpfs.NewObjectFromURI(p))
			if err == nil && len(data) > 0 {
				return summarizeText(string(boundBytes(data, 4096)))
			}
		}
		return ""
	}
	// file or workspace:// → map to file for reading
	fs := afs.New()
	if strings.HasPrefix(uri, "workspace://") {
		uri = workspaceToFile(uri)
	}
	for _, name := range order {
		p := url.Join(uri, name)
		data, err := fs.DownloadWithURL(ctx, p)
		if err == nil && len(data) > 0 {
			return summarizeText(string(boundBytes(data, 4096)))
		}
	}
	return ""
}

func summarizeText(s string) string {
	txt := strings.TrimSpace(s)
	if txt == "" {
		return ""
	}
	if i := strings.Index(txt, "\n\n"); i != -1 {
		txt = txt[:i]
	}
	if len(txt) > 512 {
		txt = txt[:512]
	}
	return strings.TrimSpace(txt)
}

func boundBytes(b []byte, n int) []byte {
	if n <= 0 || len(b) <= n {
		return b
	}
	return b[:n]
}

// agentAllowed gathers agent.resources URIs based on the current conversation context.
func (s *Service) agentAllowed(ctx context.Context) []string {
	ag := s.currentAgent(ctx)
	if ag == nil || len(ag.Resources) == 0 {
		return nil
	}
	out := make([]string, 0, len(ag.Resources))
	for _, e := range ag.Resources {
		if e == nil {
			continue
		}
		if u := strings.TrimSpace(e.URI); u != "" {
			ws, _, err := s.normalizeUserRoot(ctx, u)
			if err != nil || ws == "" {
				continue
			}
			out = append(out, ws)
		}
	}
	return out
}

// semanticAllowedForAgent reports whether semantic match is permitted for the
// given normalized workspace root under the provided agent configuration. When
// no matching resource is found, the effective value defaults to true.
func (s *Service) semanticAllowedForAgent(ctx context.Context, ag *agmodel.Agent, wsRoot string) bool {
	ws := strings.TrimRight(strings.TrimSpace(wsRoot), "/")
	if ws == "" || ag == nil || len(ag.Resources) == 0 {
		return true
	}
	for _, r := range ag.Resources {
		if r == nil || strings.TrimSpace(r.URI) == "" {
			continue
		}
		norm, _, err := s.normalizeUserRoot(ctx, r.URI)
		if err != nil || strings.TrimSpace(norm) == "" {
			continue
		}
		if strings.TrimRight(strings.TrimSpace(norm), "/") == ws {
			return r.SemanticAllowed()
		}
	}
	return true
}

// grepAllowedForAgent reports whether grepFiles is permitted for the given
// normalized workspace root under the provided agent configuration. When no
// matching resource is found, the effective value defaults to true.
func (s *Service) grepAllowedForAgent(ctx context.Context, ag *agmodel.Agent, wsRoot string) bool {
	ws := strings.TrimRight(strings.TrimSpace(wsRoot), "/")
	if ws == "" {
		return true
	}
	isMCP := mcpuri.Is(ws)
	// When no agent or resources are present, default to allowing grep on
	// local/workspace roots but require an explicit resource with allowGrep
	// for MCP roots.
	if ag == nil || len(ag.Resources) == 0 {
		if isMCP {
			return false
		}
		return true
	}
	for _, r := range ag.Resources {
		if r == nil || strings.TrimSpace(r.URI) == "" {
			continue
		}
		norm, _, err := s.normalizeUserRoot(ctx, r.URI)
		if err != nil || strings.TrimSpace(norm) == "" {
			continue
		}
		if strings.TrimRight(strings.TrimSpace(norm), "/") == ws {
			return r.GrepAllowed()
		}
	}
	// No matching resource: allow grep by default for local/workspace roots,
	// but require explicit opt-in for MCP roots.
	if isMCP {
		return false
	}
	return true
}

// currentAgent returns the active agent from conversation context, if available.
func (s *Service) currentAgent(ctx context.Context) *agmodel.Agent {
	if s.conv == nil || s.aFinder == nil {
		return nil
	}
	convID := memory.ConversationIDFromContext(ctx)
	if strings.TrimSpace(convID) == "" {
		return nil
	}
	resp, err := apiconv.NewService(s.conv).Get(ctx, apiconv.GetRequest{Id: convID})
	if err != nil || resp == nil || resp.Conversation == nil {
		return nil
	}
	tr := resp.Conversation.GetTranscript()
	var agentID string
	if len(tr) > 0 {
		t := tr[len(tr)-1]
		if t != nil && t.AgentIdUsed != nil && strings.TrimSpace(*t.AgentIdUsed) != "" {
			agentID = strings.TrimSpace(*t.AgentIdUsed)
		}
	}
	if strings.TrimSpace(agentID) == "" {
		// Fallback: use conversation.AgentId when present
		if resp.Conversation.AgentId != nil && strings.TrimSpace(*resp.Conversation.AgentId) != "" {
			agentID = strings.TrimSpace(*resp.Conversation.AgentId)
		} else {
			return nil
		}
	}
	ag, err := s.aFinder.Find(ctx, agentID)
	if err != nil {
		return nil
	}
	return ag
}

// workspaceToFile maps workspace://localhost/<rel> to file://<workspaceRoot>/<rel>
func workspaceToFile(ws string) string {
	base := url.Normalize(workspace.Root(), "file")
	rel := strings.TrimPrefix(ws, "workspace://")
	rel = strings.TrimPrefix(rel, "localhost/")
	return url.Join(strings.TrimRight(base, "/")+"/", rel)
}

// workspaceToFile maps workspace://localhost/<rel> to file://<workspaceRoot>/<rel>
func fileToWorkspace(file string) string {
	base := workspace.Root()
	file = strings.Replace(file, base, "", 1)
	return "workspace://localhost" + url.Path(file)
}

// cleanFileURL removes any "/../" segments from file URLs to produce a stable canonical form.
func cleanFileURL(u string) string {
	if !strings.HasPrefix(u, "file://") {
		return u
	}
	rest := strings.TrimPrefix(u, "file://localhost")
	rest = strings.TrimPrefix(rest, "file://")
	cleaned := "/" + strings.TrimLeft(rest, "/")
	cleaned = pathpkg.Clean(cleaned)
	return "file://localhost" + cleaned
}

// -------- grepFiles implementation --------

// GrepInput describes a lexical, snippet-aware search over files under a
// single root. It is intentionally similar in spirit to the GitHub
// SearchRepoContentInput, but scoped to local/workspace
type GrepInput struct {
	// Pattern is a required search expression. Internally it may be split on
	// simple OR separators ("|" or " or ") into multiple patterns which are
	// combined using logical OR.
	Pattern string `json:"pattern" description:"search pattern; supports OR via '|' or textual 'or' (case-insensitive)"`
	// ExcludePattern optionally defines patterns that, when matched, cause a
	// file or snippet to be excluded. It follows the same splitting rules as
	// Pattern.
	ExcludePattern string `json:"excludePattern,omitempty" description:"exclude pattern; same OR semantics as pattern"`

	// RootURI is the normalized or user-provided root URI. Prefer using
	// RootID when possible; RootURI is retained for backward compatibility.
	RootURI string `json:"root,omitempty"`
	// RootID is a stable identifier corresponding to a root returned by
	// roots. When provided, it is resolved to the underlying
	// normalized URI before enforcement and grep operations.
	RootID    string   `json:"rootId,omitempty"`
	Path      string   `json:"path"`
	Recursive bool     `json:"recursive,omitempty"`
	Include   []string `json:"include,omitempty"`
	Exclude   []string `json:"exclude,omitempty"`

	CaseInsensitive bool `json:"caseInsensitive,omitempty"`

	Mode      string `json:"mode,omitempty"  description:"snippet mode: 'head' shows the first lines of each matching file; 'match' shows lines around matches"  choice:"head" choice:"match"`
	Bytes     int    `json:"bytes,omitempty"`
	Lines     int    `json:"lines,omitempty"`
	MaxFiles  int    `json:"maxFiles,omitempty"`
	MaxBlocks int    `json:"maxBlocks,omitempty"`

	SkipBinary  bool `json:"skipBinary,omitempty"`
	MaxSize     int  `json:"maxSize,omitempty"`
	Concurrency int  `json:"concurrency,omitempty"`
}

type GrepOutput struct {
	Stats hygine.GrepStats  `json:"stats"`
	Files []hygine.GrepFile `json:"files,omitempty"`
}

func (s *Service) grepFiles(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*GrepInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*GrepOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	pattern := strings.TrimSpace(input.Pattern)
	if pattern == "" {
		return fmt.Errorf("pattern must not be empty")
	}
	rootURI := strings.TrimSpace(input.RootURI)
	if rootURI == "" && strings.TrimSpace(input.RootID) != "" {
		var err error
		rootURI, err = s.resolveRootID(ctx, input.RootID)
		if err != nil {
			return err
		}
	}
	if rootURI == "" {
		return fmt.Errorf("root or rootId is required")
	}
	allowed := s.agentAllowed(ctx)
	wsRoot, _, err := s.normalizeUserRoot(ctx, rootURI)
	if err != nil {
		return err
	}
	if len(allowed) > 0 && !isAllowedWorkspace(wsRoot, allowed) {
		return fmt.Errorf("root not allowed: %s", rootURI)
	}
	// Enforce per-resource grep capability when agent context is available.
	if !s.grepAllowedForAgent(ctx, s.currentAgent(ctx), wsRoot) {
		return fmt.Errorf("grep not allowed for root: %s", rootURI)
	}
	// Currently grepFiles is implemented for local/workspace-backed roots only.
	if mcpuri.Is(wsRoot) {
		return fmt.Errorf("grepFiles is not supported for mcp resources")
	}
	base := wsRoot
	if strings.HasPrefix(wsRoot, "workspace://") {
		base = workspaceToFile(wsRoot)
	}
	if strings.TrimSpace(input.Path) != "" {
		p := strings.TrimSpace(input.Path)
		if isAbsLikePath(p) {
			// For absolute-like paths, ensure they remain under the configured
			// root when the root is file/workspace-backed, and avoid joining to
			// prevent duplicated prefixes.
			if !mcpuri.Is(wsRoot) {
				rootBase := base
				if strings.HasPrefix(rootBase, "file://") {
					rootBase = fileURLToPath(rootBase)
				}
				pathBase := p
				if strings.HasPrefix(pathBase, "file://") {
					pathBase = fileURLToPath(pathBase)
				}
				if !isUnderRootPath(pathBase, rootBase) {
					return fmt.Errorf("path %s is outside root %s", p, rootURI)
				}
			}
			base = p
		} else {
			base = url.Join(base, strings.TrimPrefix(p, "/"))
		}
	}
	// Normalise defaults
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	if mode == "" {
		mode = "match"
	}
	limitBytes := input.Bytes
	if limitBytes <= 0 {
		limitBytes = 512
	}
	limitLines := input.Lines
	if limitLines <= 0 {
		limitLines = 32
	}
	maxFiles := input.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 20
	}
	maxBlocks := input.MaxBlocks
	if maxBlocks <= 0 {
		maxBlocks = 200
	}
	maxSize := input.MaxSize
	if maxSize <= 0 {
		maxSize = 1024 * 1024 // 1MB
	}
	skipBinary := input.SkipBinary
	if !input.SkipBinary {
		// default behaviour: skip binary files unless explicitly disabled
		skipBinary = true
	}

	patList := splitPatterns(pattern)
	exclList := splitPatterns(strings.TrimSpace(input.ExcludePattern))
	if len(patList) == 0 {
		return fmt.Errorf("pattern must not be empty")
	}
	matchers, err := compilePatterns(patList, input.CaseInsensitive)
	if err != nil {
		return err
	}
	excludeMatchers, err := compilePatterns(exclList, input.CaseInsensitive)
	if err != nil {
		return err
	}

	stats := hygine.GrepStats{}
	var files []hygine.GrepFile
	totalBlocks := 0

	// Local/workspace vs MCP handling
	if mcpuri.Is(wsRoot) {
		if s.mcpMgr == nil {
			return fmt.Errorf("mcp manager not configured")
		}
		mfs := mcpfs.New(s.mcpMgr)
		objs, err := mfs.List(ctx, base)
		if err != nil {
			return err
		}
		for _, o := range objs {
			if o == nil || o.IsDir() {
				continue
			}
			uri := o.URL()
			rel := relativePath(base, uri)
			if !globAllowed(rel, input.Include, input.Exclude) {
				continue
			}
			stats.Scanned++
			if stats.Matched >= maxFiles || totalBlocks >= maxBlocks {
				stats.Truncated = true
				break
			}
			data, err := mfs.Download(ctx, o)
			if err != nil {
				return err
			}
			if maxSize > 0 && len(data) > maxSize {
				data = data[:maxSize]
			}
			if skipBinary && isBinary(data) {
				continue
			}
			text := string(data)
			lines := strings.Split(text, "\n")
			var matchLines []int
			for i, line := range lines {
				if lineMatches(line, matchers, excludeMatchers) {
					matchLines = append(matchLines, i)
				}
			}
			if len(matchLines) == 0 {
				continue
			}
			stats.Matched++
			gf := hygine.GrepFile{Path: rel, URI: uri}
			gf.Matches = len(matchLines)
			if mode == "head" {
				end := limitLines
				if end > len(lines) {
					end = len(lines)
				}
				snippetText := joinLines(lines[:end])
				if len(snippetText) > limitBytes {
					snippetText = snippetText[:limitBytes]
				}
				gf.Snippets = append(gf.Snippets, hygine.Snippet{StartLine: 1, EndLine: end, Text: snippetText})
				files = append(files, gf)
				if stats.Matched >= maxFiles || totalBlocks >= maxBlocks {
					stats.Truncated = true
					break
				}
				continue
			}
			for _, idx := range matchLines {
				if totalBlocks >= maxBlocks {
					stats.Truncated = true
					break
				}
				start := idx - limitLines/2
				if start < 0 {
					start = 0
				}
				end := start + limitLines
				if end > len(lines) {
					end = len(lines)
				}
				snippetText := joinLines(lines[start:end])
				cut := false
				if len(snippetText) > limitBytes {
					snippetText = snippetText[:limitBytes]
					cut = true
				}
				gf.Snippets = append(gf.Snippets, hygine.Snippet{
					StartLine:   start + 1,
					EndLine:     end,
					Text:        snippetText,
					OffsetBytes: 0,
					LengthBytes: len(snippetText),
					Cut:         cut,
				})
				totalBlocks++
				if totalBlocks >= maxBlocks {
					stats.Truncated = true
					break
				}
			}
			files = append(files, gf)
			if stats.Matched >= maxFiles || totalBlocks >= maxBlocks {
				stats.Truncated = true
				break
			}
		}
	} else {
		fs := afs.New()
		err = fs.Walk(ctx, base, func(ctx context.Context, walkBaseURL, parent string, info os.FileInfo, reader io.Reader) (bool, error) {
			if info == nil || info.IsDir() {
				return true, nil
			}
			// Enforce non-recursive mode: only consider direct children when Recursive=false.
			if !input.Recursive && parent != "" {
				return true, nil
			}
			// Build full URI
			var uri string
			if parent == "" {
				uri = url.Join(walkBaseURL, info.Name())
			} else {
				uri = url.Join(walkBaseURL, parent, info.Name())
			}
			// Apply include/exclude globs on the relative path
			rel := relativePath(base, uri)
			if !globAllowed(rel, input.Include, input.Exclude) {
				return true, nil
			}

			stats.Scanned++
			if stats.Matched >= maxFiles || totalBlocks >= maxBlocks {
				stats.Truncated = true
				return false, nil
			}
			data, err := fs.DownloadWithURL(ctx, uri)
			if err != nil {
				return false, err
			}
			if maxSize > 0 && len(data) > maxSize {
				data = data[:maxSize]
			}
			if skipBinary && isBinary(data) {
				return true, nil
			}
			text := string(data)
			lines := strings.Split(text, "\n")
			var matchLines []int
			for i, line := range lines {
				if lineMatches(line, matchers, excludeMatchers) {
					matchLines = append(matchLines, i)
				}
			}
			if len(matchLines) == 0 {
				return true, nil
			}
			stats.Matched++
			gf := hygine.GrepFile{Path: rel, URI: uri}
			gf.Matches = len(matchLines)
			// Build snippets depending on mode
			if mode == "head" {
				// Single snippet from the top of the file
				end := limitLines
				if end > len(lines) {
					end = len(lines)
				}
				snippetText := joinLines(lines[:end])
				if len(snippetText) > limitBytes {
					snippetText = snippetText[:limitBytes]
				}
				gf.Snippets = append(gf.Snippets, hygine.Snippet{StartLine: 1, EndLine: end, Text: snippetText})
				files = append(files, gf)
				return stats.Matched < maxFiles && totalBlocks < maxBlocks, nil
			}
			// match mode: build a snippet around each match line
			for _, idx := range matchLines {
				if totalBlocks >= maxBlocks {
					stats.Truncated = true
					return false, nil
				}
				start := idx - limitLines/2
				if start < 0 {
					start = 0
				}
				end := start + limitLines
				if end > len(lines) {
					end = len(lines)
				}
				snippetText := joinLines(lines[start:end])
				cut := false
				if len(snippetText) > limitBytes {
					snippetText = snippetText[:limitBytes]
					cut = true
				}
				gf.Snippets = append(gf.Snippets, hygine.Snippet{
					StartLine:   start + 1,
					EndLine:     end,
					Text:        snippetText,
					OffsetBytes: 0,
					LengthBytes: len(snippetText),
					Cut:         cut,
				})
				totalBlocks++
				if totalBlocks >= maxBlocks {
					stats.Truncated = true
					break
				}
			}
			files = append(files, gf)
			if stats.Matched >= maxFiles || totalBlocks >= maxBlocks {
				stats.Truncated = true
				return false, nil
			}
			return true, nil
		})
		if err != nil {
			return err
		}
	}
	output.Stats = stats
	output.Files = files
	return nil
}

// splitPatterns splits a logical OR expression into individual patterns.
// It supports simple separators like "|" and textual "or" (case-insensitive)
// surrounded by spaces, e.g. "Auth or Token" or "AUTH OR TOKEN".
func splitPatterns(expr string) []string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	lower := strings.ToLower(expr)
	sep := " or "
	var parts []string
	start := 0
	for {
		idx := strings.Index(lower[start:], sep)
		if idx == -1 {
			break
		}
		// idx is relative to start; convert to absolute index into expr
		abs := start + idx
		parts = append(parts, expr[start:abs])
		start = abs + len(sep)
	}
	if start == 0 {
		// no textual "or" found; treat whole expr as a single part
		parts = []string{expr}
	} else {
		parts = append(parts, expr[start:])
	}
	var out []string
	for _, p := range parts {
		for _, sub := range strings.Split(p, "|") {
			if v := strings.TrimSpace(sub); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

// helper used by grepFiles to compile patterns.
func compilePatterns(patterns []string, caseInsensitive bool) ([]*regexp.Regexp, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		pat := strings.TrimSpace(p)
		if pat == "" {
			continue
		}
		if caseInsensitive {
			pat = "(?i)" + pat
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", p, err)
		}
		out = append(out, re)
	}
	return out, nil
}

// lineMatches reports whether a line matches at least one include pattern and
// none of the exclude patterns.
func lineMatches(line string, includes, excludes []*regexp.Regexp) bool {
	matched := false
	if len(includes) == 0 {
		matched = true
	} else {
		for _, re := range includes {
			if re.FindStringIndex(line) != nil {
				matched = true
				break
			}
		}
	}
	if !matched {
		return false
	}
	for _, re := range excludes {
		if re.FindStringIndex(line) != nil {
			return false
		}
	}
	return true
}

// globAllowed applies simple include/exclude globs to a path. When include is
// empty, all paths are considered included.
func globAllowed(path string, include, exclude []string) bool {
	if path == "" {
		return false
	}
	allowed := true
	if len(include) > 0 {
		allowed = false
		for _, g := range include {
			g = strings.TrimSpace(g)
			if g == "" {
				continue
			}
			if ok, _ := pathpkg.Match(g, path); ok {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		return false
	}
	for _, g := range exclude {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if ok, _ := pathpkg.Match(g, path); ok {
			return false
		}
	}
	return true
}

// isBinary provides a simple heuristic for binary files: presence of NUL
// bytes or invalid UTF-8.
func isBinary(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	if !utf8.Valid(b) {
		return true
	}
	return false
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	for i, ln := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(ln)
	}
	return b.String()
}
