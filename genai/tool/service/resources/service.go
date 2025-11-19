package resources

import (
	"context"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/viant/afs"
	"github.com/viant/afs/url"
	apiconv "github.com/viant/agently/client/conversation"
	agmodel "github.com/viant/agently/genai/agent"
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

// Service exposes resource roots, listing, reading and semantic match over filesystem and MCP resources.
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

// WithMCPManager attaches an MCP manager for listing/downloading MCP resources.
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
	default:
		return nil, svc.NewMethodNotFoundError(name)
	}
}

// MatchInput defines parameters for semantic search across one or more roots.
type MatchInput struct {
	Query        string          `json:"query"`
	RootURI      []string        `json:"rootUri" description:"use valid resources:root uri"`
	Roots        []string        `json:"roots,omitempty" description:"alias for rootUri; deprecated"`
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
	if len(input.RootURI) == 0 && len(input.Roots) == 0 {
		return fmt.Errorf("roots is required")
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
	sources := input.RootURI
	if len(sources) == 0 && len(input.Roots) > 0 {
		sources = input.Roots
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
		// Convert to file (for internal I/O) when workspace scheme
		base := wsRoot
		if strings.HasPrefix(wsRoot, "workspace://") {
			base = workspaceToFile(wsRoot)
		}
		if strings.TrimSpace(input.Path) != "" {
			base = url.Join(base, strings.TrimPrefix(strings.TrimSpace(input.Path), "/"))
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

// -------- list implementation --------

type ListInput struct {
	RootURI   string `json:"root"`
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
	if rootURI == "" {
		return fmt.Errorf("root is required")
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
		base = url.Join(normRoot, strings.TrimPrefix(strings.TrimSpace(input.Path), "/"))
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
	RootURI  string `json:"root,omitempty"`
	Path     string `json:"path,omitempty"`
	URI      string `json:"uri,omitempty"`
	MaxBytes int    `json:"maxBytes,omitempty"`
}

// ReadOutput contains the resolved URI, relative path and optionally truncated content.
type ReadOutput struct {
	URI     string `json:"uri"`
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int    `json:"size"`
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
		if rootURI == "" {
			return fmt.Errorf("root or uri is required")
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
		fullURI = url.Join(normRoot, strings.TrimPrefix(pathPart, "/"))
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
	if input.MaxBytes > 0 && len(data) > input.MaxBytes {
		data = data[:input.MaxBytes]
	}
	output.URI = fullURI
	if normRoot != "" {
		output.Path = relativePath(normRoot, fullURI)
	} else {
		output.Path = fullURI
	}
	output.Content = string(data)
	output.Size = len(data)
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
	URI         string `json:"uri"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind"`   // file|mcp
	Source      string `json:"source"` // default
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
	source := "agent"
	if len(locs) == 0 {
		locs = append([]string(nil), s.defaults.Locations...)
		source = "default"
	}
	if len(locs) == 0 {
		output.Roots = nil
		return nil
	}
	var roots []Root
	seen := map[string]bool{}
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
		roots = append(roots, Root{URI: norm, Description: desc, Kind: kind, Source: source})
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
		// Map to workspace if under workspace root
		wsBase := url.Normalize(workspace.Root(), "file")
		if strings.HasPrefix(u, strings.TrimRight(wsBase, "/")+"/") {
			rel := strings.TrimPrefix(u, strings.TrimRight(wsBase, "/")+"/")
			return url.Join("workspace://localhost/", rel), "file", nil
		}
		return "", "", fmt.Errorf("file uri not allowed: outside workspace")
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
