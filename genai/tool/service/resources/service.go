package resources

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
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
	"github.com/viant/agently/genai/textclip"
	svc "github.com/viant/agently/genai/tool/service"
	"github.com/viant/agently/genai/tool/service/shared/imageio"
	"github.com/viant/agently/internal/agent/systemdoc"
	authctx "github.com/viant/agently/internal/auth"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
	mcpuri "github.com/viant/agently/internal/mcp/uri"
	"github.com/viant/agently/internal/workspace"
	embopt "github.com/viant/embedius/matching/option"
	embSchema "github.com/viant/embedius/schema"
	"github.com/viant/mcp-protocol/extension"
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

	augmentDocsOverride func(ctx context.Context, input *aug.AugmentDocsInput, output *aug.AugmentDocsOutput) error
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
		{Name: "read", Description: "Read a single resource under a root. For large files, prefer byteRange and page in chunks (<= 8KB).", Input: reflect.TypeOf(&ReadInput{}), Output: reflect.TypeOf(&ReadOutput{})},
		{Name: "readImage", Description: "Read an image under a root and return a base64 payload suitable for attaching as a vision input. Defaults to resizing to fit 2048x768.", Input: reflect.TypeOf(&ReadImageInput{}), Output: reflect.TypeOf(&ReadImageOutput{})},
		{Name: "match", Description: "Semantic match search over one or more roots; use `match.exclusions` to block specific workspace paths.", Input: reflect.TypeOf(&MatchInput{}), Output: reflect.TypeOf(&MatchOutput{})},
		{Name: "matchDocuments", Description: "Rank semantic matches and return distinct URIs with score + root metadata for transcript promotion. Example: {\"rootIds\":[\"workspace://localhost/knowledge/bidder\"],\"query\":\"performance\"}. Output fields: documents[].uri, documents[].score, documents[].rootId.", Input: reflect.TypeOf(&MatchDocumentsInput{}), Output: reflect.TypeOf(&MatchDocumentsOutput{})},
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
	case "readimage":
		return s.readImage, nil
	case "match":
		return s.match, nil
	case "matchdocuments":
		return s.matchDocuments, nil
	case "grepfiles":
		return s.grepFiles, nil
	default:
		return nil, svc.NewMethodNotFoundError(name)
	}
}

func (s *Service) runAugmentDocs(ctx context.Context, input *aug.AugmentDocsInput, output *aug.AugmentDocsOutput) error {
	if s.augmentDocsOverride != nil {
		return s.augmentDocsOverride(ctx, input, output)
	}
	if s.augmenter == nil {
		return fmt.Errorf("augmenter service is not configured")
	}
	return s.augmenter.AugmentDocs(ctx, input, output)
}

// MatchInput defines parameters for semantic search across one or more roots.
type MatchInput struct {
	Query string `json:"query"`
	// RootURI/Roots are retained for backward compatibility but will default to all accessible roots when omitted.
	RootURI []string `json:"rootUri,omitempty" internal:"true"`
	Roots   []string `json:"roots,omitempty" internal:"true"`
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
	// SystemContent mirrors Content but only includes system-role documents so callers can surface
	// protected context as system messages.
	SystemContent string `json:"systemContent,omitempty" description:"Formatted content for system resources only."`
	// DocumentRoots maps document SourceURI values to their originating root IDs.
	DocumentRoots map[string]string `json:"documentRoots,omitempty" description:"Maps document source URIs to their root IDs."`
}

type augmentedDocuments struct {
	documents      []embSchema.Document
	trimPrefix     string
	systemPrefixes []string
}

type searchRootMeta struct {
	id     string
	wsRoot string
}

func (s *Service) selectSearchRoots(ctx context.Context, roots []Root, input *MatchInput) ([]Root, error) {
	if len(roots) == 0 {
		return nil, fmt.Errorf("no roots configured")
	}
	var selected []Root
	seen := map[string]struct{}{}
	add := func(candidates []Root) {
		for _, root := range candidates {
			key := strings.ToLower(strings.TrimSpace(root.ID))
			if key == "" {
				key = strings.ToLower(strings.TrimSpace(root.URI))
			}
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			if !root.AllowedSemanticSearch {
				continue
			}
			seen[key] = struct{}{}
			selected = append(selected, root)
		}
	}
	if len(input.RootIDs) > 0 {
		matchedByID := filterRootsByID(roots, input.RootIDs)
		add(matchedByID)
		missing := missingRootIDs(input.RootIDs, matchedByID)
		if len(missing) > 0 {
			matchedByURI := filterRootsByURI(roots, missing)
			add(matchedByURI)
			if remaining := missingRootIDs(input.RootIDs, append(matchedByID, matchedByURI...)); len(remaining) > 0 {
				return nil, fmt.Errorf("unknown rootId(s): %s", strings.Join(remaining, ", "))
			}
		}
	}
	if len(selected) == 0 {
		uris := append([]string(nil), input.RootURI...)
		uris = append(uris, input.Roots...)
		if len(uris) > 0 {
			add(filterRootsByURI(roots, uris))
		}
	}
	if len(selected) == 0 {
		add(roots)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no semantic-enabled roots available")
	}
	return selected, nil
}

func filterRootsByID(roots []Root, ids []string) []Root {
	if len(ids) == 0 {
		return nil
	}
	idSet := map[string]struct{}{}
	for _, raw := range ids {
		if trimmed := normalizeRootID(raw); trimmed != "" {
			idSet[trimmed] = struct{}{}
		}
	}
	var out []Root
	for _, root := range roots {
		id := normalizeRootID(root.ID)
		if id == "" {
			id = normalizeRootID(root.URI)
		}
		if id == "" {
			continue
		}
		if _, ok := idSet[id]; ok {
			out = append(out, root)
		}
	}
	return out
}

func filterRootsByURI(roots []Root, uris []string) []Root {
	if len(uris) == 0 {
		return nil
	}
	uriSet := map[string]struct{}{}
	for _, raw := range uris {
		if trimmed := normalizeRootURI(raw); trimmed != "" {
			uriSet[trimmed] = struct{}{}
		}
	}
	var out []Root
	for _, root := range roots {
		uri := normalizeRootURI(root.URI)
		if uri == "" {
			continue
		}
		if _, ok := uriSet[uri]; ok {
			out = append(out, root)
		}
	}
	return out
}

func missingRootIDs(requested []string, matched []Root) []string {
	if len(requested) == 0 {
		return nil
	}
	matchedSet := map[string]struct{}{}
	for _, root := range matched {
		if key := normalizeRootID(root.ID); key != "" {
			matchedSet[key] = struct{}{}
		}
		if key := normalizeRootURI(root.URI); key != "" {
			matchedSet[key] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	var missing []string
	for _, raw := range requested {
		idKey := normalizeRootID(raw)
		uriKey := normalizeRootURI(raw)
		if idKey == "" && uriKey == "" {
			continue
		}
		if _, ok := matchedSet[idKey]; ok {
			continue
		}
		if _, ok := matchedSet[uriKey]; ok {
			continue
		}
		key := idKey
		if key == "" {
			key = uriKey
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		missing = append(missing, raw)
	}
	return missing
}

func normalizeRootID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeRootURI(value string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(value), "/"))
}

func assignRootMetadata(doc *embSchema.Document, roots []searchRootMeta) {
	if doc == nil || len(roots) == 0 {
		return
	}
	path := documentMetadataPath(doc.Metadata)
	if path == "" {
		return
	}
	normalized := strings.ToLower(strings.TrimRight(path, "/"))
	for _, entry := range roots {
		prefix := strings.ToLower(strings.TrimRight(entry.wsRoot, "/"))
		if prefix == "" {
			continue
		}
		if normalized == prefix || strings.HasPrefix(normalized, prefix+"/") {
			doc.Metadata["rootId"] = entry.id
			return
		}
	}
}

func normalizeSearchPath(p string, wsRoot string) string {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return ""
	}
	trimmed = toWorkspaceURI(trimmed)
	root := strings.TrimRight(strings.TrimSpace(wsRoot), "/")
	if root == "" {
		return trimmed
	}
	if strings.EqualFold(strings.TrimRight(trimmed, "/"), root) {
		return ""
	}
	if strings.HasPrefix(trimmed, root+"/") {
		return strings.TrimPrefix(trimmed[len(root):], "/")
	}
	return trimmed
}

func (s *Service) buildAugmentedDocuments(ctx context.Context, input *MatchInput) (*augmentedDocuments, error) {
	if s == nil {
		return nil, fmt.Errorf("service not configured")
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	embedderID := strings.TrimSpace(input.Model)
	if embedderID == "" {
		embedderID = strings.TrimSpace(s.defaultEmbedder)
	}
	if embedderID == "" {
		return nil, fmt.Errorf("embedder is required (set default embedder in config or provide internal model)")
	}
	input.IncludeFile = true

	collectedRoots, err := s.collectRoots(ctx)
	if err != nil {
		return nil, err
	}
	availableRoots := collectedRoots.all()
	if len(availableRoots) == 0 {
		return nil, fmt.Errorf("no roots configured for semantic search")
	}
	selectedRoots, err := s.selectSearchRoots(ctx, availableRoots, input)
	if err != nil {
		return nil, err
	}
	locations := make([]string, 0, len(selectedRoots))
	searchRoots := make([]searchRootMeta, 0, len(selectedRoots))
	for _, root := range selectedRoots {
		wsRoot := strings.TrimRight(strings.TrimSpace(root.URI), "/")
		if wsRoot == "" {
			continue
		}
		base := wsRoot
		if strings.HasPrefix(wsRoot, "workspace://") {
			base = workspaceToFile(wsRoot)
		}
		if trimmed := normalizeSearchPath(input.Path, wsRoot); trimmed != "" {
			base, err = joinBaseWithPath(wsRoot, base, trimmed, root.URI)
			if err != nil {
				return nil, err
			}
		}
		locations = append(locations, base)
		searchRoots = append(searchRoots, searchRootMeta{
			id:     root.ID,
			wsRoot: wsRoot,
		})
	}
	if len(locations) == 0 {
		return nil, fmt.Errorf("no valid roots provided")
	}
	trimPrefix := commonPrefix(locations)
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
	if err := s.runAugmentDocs(ctx, augIn, &augOut); err != nil {
		return nil, err
	}
	sort.SliceStable(augOut.Documents, func(i, j int) bool { return augOut.Documents[i].Score > augOut.Documents[j].Score })

	curAgent := s.currentAgent(ctx)
	sysPrefixes := systemdoc.Prefixes(curAgent)
	for i := range augOut.Documents {
		doc := &augOut.Documents[i]
		if doc.Metadata == nil {
			doc.Metadata = map[string]any{}
		}
		doc.Metadata["score"] = doc.Score
		for _, key := range []string{"path", "docId", "fragmentId"} {
			if p, ok := doc.Metadata[key]; ok {
				if s, _ := p.(string); s != "" {
					doc.Metadata[key] = toWorkspaceURI(s)
				}
			}
		}
		assignRootMetadata(doc, searchRoots)
	}
	return &augmentedDocuments{
		documents:      augOut.Documents,
		trimPrefix:     trimPrefix,
		systemPrefixes: sysPrefixes,
	}, nil
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
	res, err := s.buildAugmentedDocuments(ctx, input)
	if err != nil {
		return err
	}

	// Apply byte-limited pagination for presentation
	limit := effectiveLimitBytes(input.LimitBytes)
	cursor := effectiveCursor(input.Cursor)
	pageDocs, hasNext := selectDocPage(res.documents, limit, cursor, res.trimPrefix)

	// If the total formatted size of all documents does not exceed the limit,
	// there is no next page regardless of internal grouping.
	if total := totalFormattedBytes(res.documents, res.trimPrefix); total <= limit {
		hasNext = false
	}

	// Rebuild Content for selected page using same format as augmenter
	content := buildDocumentContent(pageDocs, res.trimPrefix)
	output.AugmentDocsOutput.Content = content
	output.AugmentDocsOutput.Documents = pageDocs
	output.AugmentDocsOutput.DocumentsSize = augmenterDocumentsSize(pageDocs)
	output.DocumentRoots = buildDocumentRootsMap(pageDocs)
	output.Cursor = cursor
	output.LimitBytes = limit
	if sys := buildDocumentContent(filterSystemDocuments(pageDocs, res.systemPrefixes), res.trimPrefix); strings.TrimSpace(sys) != "" {
		output.SystemContent = sys
	}
	if hasNext {
		output.NextCursor = cursor + 1
	}
	return nil
}

type MatchDocumentsInput struct {
	Query        string          `json:"query" description:"semantic search query" required:"true"`
	RootIDs      []string        `json:"rootIds,omitempty" description:"resource root ids returned by roots"`
	Path         string          `json:"path,omitempty" description:"optional subpath relative to selected roots"`
	Model        string          `json:"model,omitempty" internal:"true"`
	MaxDocuments int             `json:"maxDocuments,omitempty" description:"maximum number of matched documents (distinct URIs); defaults to 5"`
	Match        *embopt.Options `json:"match,omitempty" internal:"true"`
}

type MatchedDocument struct {
	URI    string  `json:"uri"`
	RootID string  `json:"rootId,omitempty"`
	Score  float32 `json:"score"`
}

type MatchDocumentsOutput struct {
	Documents []MatchedDocument `json:"documents"`
}

func (s *Service) matchDocuments(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*MatchDocumentsInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	if strings.TrimSpace(input.Query) == "" {
		return fmt.Errorf("query is required")
	}
	output, ok := out.(*MatchDocumentsOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	maxDocs := input.MaxDocuments
	if maxDocs <= 0 {
		maxDocs = 5
	}
	matchInput := &MatchInput{
		Query:        input.Query,
		RootIDs:      append([]string(nil), input.RootIDs...),
		Path:         input.Path,
		Model:        input.Model,
		MaxDocuments: maxDocs,
		Match:        input.Match,
	}
	res, err := s.buildAugmentedDocuments(ctx, matchInput)
	if err != nil {
		return err
	}
	docs := res.documents
	ranked := uniqueMatchedDocuments(docs, maxDocs)
	if len(ranked) == 0 {
		output.Documents = nil
		return nil
	}
	output.Documents = ranked
	return nil
}

func uniqueMatchedDocuments(docs []embSchema.Document, max int) []MatchedDocument {
	if len(docs) == 0 {
		return nil
	}
	type entry struct {
		doc MatchedDocument
		idx int
	}
	seen := map[string]entry{}
	order := make([]string, 0, len(docs))
	for idx, doc := range docs {
		uri := documentMetadataPath(doc.Metadata)
		if uri == "" {
			continue
		}
		current := MatchedDocument{
			URI:    uri,
			RootID: documentRootID(doc.Metadata),
			Score:  doc.Score,
		}
		if existing, ok := seen[uri]; ok {
			if current.Score > existing.doc.Score {
				seen[uri] = entry{doc: current, idx: existing.idx}
			}
			continue
		}
		seen[uri] = entry{doc: current, idx: idx}
		order = append(order, uri)
	}
	if len(seen) == 0 {
		return nil
	}
	result := make([]MatchedDocument, 0, len(seen))
	for _, uri := range order {
		if rec, ok := seen[uri]; ok {
			result = append(result, rec.doc)
			if max > 0 && len(result) >= max {
				break
			}
		}
	}
	return result
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
		loc := documentLocation(d, trimPrefix)
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
		loc := documentLocation(d, trimPrefix)
		total += len(formatDocument(loc, d.PageContent))
	}
	return total
}

func buildDocumentContent(docs []embSchema.Document, trimPrefix string) string {
	if len(docs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, doc := range docs {
		loc := documentLocation(doc, trimPrefix)
		_, _ = b.WriteString(formatDocument(loc, doc.PageContent))
	}
	return b.String()
}

func documentLocation(doc embSchema.Document, trimPrefix string) string {
	loc := strings.TrimPrefix(getStringFromMetadata(doc.Metadata, "path"), trimPrefix)
	if loc == "" {
		loc = getStringFromMetadata(doc.Metadata, "docId")
	}
	return loc
}

func filterSystemDocuments(docs []embSchema.Document, prefixes []string) []embSchema.Document {
	if len(prefixes) == 0 || len(docs) == 0 {
		return nil
	}
	out := make([]embSchema.Document, 0, len(docs))
	for _, doc := range docs {
		if systemdoc.Matches(prefixes, documentMetadataPath(doc.Metadata)) {
			out = append(out, doc)
		}
	}
	return out
}

func documentMetadataPath(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	for _, key := range []string{"path", "docId", "fragmentId"} {
		if v, ok := metadata[key]; ok {
			if s, _ := v.(string); strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

func documentRootID(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	if v, ok := metadata["rootId"]; ok {
		if s, _ := v.(string); strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func buildDocumentRootsMap(docs []embSchema.Document) map[string]string {
	if len(docs) == 0 {
		return nil
	}
	roots := make(map[string]string)
	for _, doc := range docs {
		path := documentMetadataPath(doc.Metadata)
		rootID := documentRootID(doc.Metadata)
		if path == "" || rootID == "" {
			continue
		}
		roots[path] = rootID
	}
	if len(roots) == 0 {
		return nil
	}
	return roots
}

// -------- list implementation --------

type ListInput struct {
	// RootURI is the normalized or user-provided root URI. Prefer using
	// RootID when possible; RootURI is retained for backward compatibility
	// but hidden from public schemas.
	RootURI string `json:"root,omitempty" internal:"true" description:"normalized or user-provided root URI; prefer rootId when available"`
	// RootID is a stable identifier corresponding to a root returned by
	// roots. When provided, it is resolved to the underlying
	// normalized URI before enforcement and listing.
	RootID string `json:"rootId,omitempty" description:"resource root id returned by roots"`
	// Path is an optional subpath under the selected root. When empty, the
	// root itself is listed. Paths may be relative to the root or
	// absolute-like; absolute-like paths must remain under the root.
	Path string `json:"path,omitempty" description:"optional subpath under the root to list"`
	// Recursive controls whether the listing should walk the subtree under
	// Path. When false, only immediate children are returned.
	Recursive bool `json:"recursive,omitempty" description:"when true, walk recursively under path"`
	// Include defines optional file or path globs to include. When provided,
	// only items whose relative path or base name matches at least one
	// pattern are returned. Globs use path-style matching rules.
	Include []string `json:"include,omitempty" description:"optional file/path globs to include (relative to root+path)"`
	// Exclude defines optional file or path globs to exclude. When provided,
	// any item whose relative path or base name matches a pattern is
	// filtered out.
	Exclude []string `json:"exclude,omitempty" description:"optional file/path globs to exclude"`
	// MaxItems caps the number of items returned. When zero or negative, no
	// explicit limit is applied.
	MaxItems int `json:"maxItems,omitempty" description:"maximum number of items to return; 0 means no limit"`
}

type ListItem struct {
	URI      string    `json:"uri"`
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	RootID   string    `json:"rootId,omitempty"`
}

type ListOutput struct {
	Items []ListItem `json:"items"`
	Total int        `json:"total"`
}

// normalizeListGlobs trims whitespace and removes empty patterns.
func normalizeListGlobs(patterns []string) []string {
	out := make([]string, 0, len(patterns))
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// listGlobMatch performs a best-effort path-style glob match. It uses
// path.Match and treats any pattern error as a non-match.
func listGlobMatch(pattern, value string) bool {
	if pattern == "" || value == "" {
		return false
	}
	ok, err := pathpkg.Match(pattern, value)
	if err != nil {
		return false
	}
	return ok
}

// listMatchesFilters reports whether a candidate with the given relative
// path and base name passes include/exclude filters. When includes are
// non-empty, at least one must match; excludes always take precedence.
func listMatchesFilters(relPath, name string, includes, excludes []string) bool {
	// Exclude has priority.
	for _, pat := range excludes {
		if listGlobMatch(pat, relPath) || listGlobMatch(pat, name) {
			return false
		}
	}
	if len(includes) == 0 {
		return true
	}
	for _, pat := range includes {
		if listGlobMatch(pat, relPath) || listGlobMatch(pat, name) {
			return true
		}
	}
	return false
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
	rootCtx, err := s.newRootContext(ctx, input.RootURI, input.RootID, s.agentAllowed(ctx))
	if err != nil {
		return err
	}
	rootBase := rootCtx.Base()
	base := rootBase
	if trimmed := strings.TrimSpace(input.Path); trimmed != "" {
		base, err = rootCtx.ResolvePath(trimmed)
		if err != nil {
			return err
		}
	}
	includes := normalizeListGlobs(input.Include)
	excludes := normalizeListGlobs(input.Exclude)
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
			rel := relativePath(rootBase, uri)
			if !listMatchesFilters(rel, o.Name(), includes, excludes) {
				continue
			}
			seen[uri] = true
			items = append(items, ListItem{
				URI:      uri,
				Path:     rel,
				Name:     o.Name(),
				Size:     o.Size(),
				Modified: o.ModTime(),
				RootID:   rootCtx.ID(),
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

				scheme := url.SchemeExtensionURL(walkBaseURL)
				rootBaseNormalised := url.Normalize(rootBase, scheme)
				rel := relativePath(rootBaseNormalised, uri)

				if !listMatchesFilters(rel, info.Name(), includes, excludes) {
					return true, nil
				}
				seen[uri] = true
				items = append(items, ListItem{
					URI:      uri,
					Path:     rel,
					Name:     info.Name(),
					Size:     info.Size(),
					Modified: info.ModTime(),
					RootID:   rootCtx.ID(),
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

			baseNormalised := base
			if len(objs) > 0 {
				scheme := url.SchemeExtensionURL(objs[0].URL())
				baseNormalised = url.Normalize(base, scheme)
			}

			for _, o := range objs {
				if o == nil {
					continue
				}

				if baseNormalised == o.URL() {
					continue
				}

				uri := url.Join(base, o.Name()) // we don't enforce normalised base here
				if seen[uri] {
					continue
				}
				rel := relativePath(rootBase, uri)
				if !listMatchesFilters(rel, o.Name(), includes, excludes) {
					continue
				}
				seen[uri] = true
				items = append(items, ListItem{
					URI:      uri,
					Path:     rel,
					Name:     o.Name(),
					Size:     o.Size(),
					Modified: o.ModTime(),
					RootID:   rootCtx.ID(),
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
	RootID string `json:"rootId,omitempty"`
	Path   string `json:"path,omitempty"`
	URI    string `json:"uri,omitempty"`

	// Range selectors; nested objects accepted by JSON schema
	BytesRange textclip.BytesRange `json:"bytesRange,omitempty"`
	LineRange  textclip.LineRange  `json:"lineRange,omitempty"`

	// MaxBytes and MaxLines cap the returned payload when neither byte nor
	// line ranges are provided. When zero, defaults are applied.
	MaxBytes int `json:"maxBytes,omitempty"`
	MaxLines int `json:"maxLines,omitempty"`

	// Mode provides lightweight previews without full reads:
	// head (default), tail, signatures.
	Mode string `json:"mode,omitempty"`
}

// ReadOutput contains the resolved URI, relative path and optionally truncated content.
type ReadOutput struct {
	URI     string `json:"uri"`
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int    `json:"size"`
	// Returned and Remaining describe how much of the original payload was
	// returned after applying caps/ranges.
	Returned  int `json:"returned,omitempty"`
	Remaining int `json:"remaining,omitempty"`
	// StartLine and EndLine are 1-based line numbers describing the selected
	// slice when Offset/Limit were provided. They are zero when the entire
	// (possibly MaxBytes-truncated) file content is returned.
	StartLine int `json:"startLine,omitempty"`
	EndLine   int `json:"endLine,omitempty"`
	// Binary is true when the content was detected as binary and not fully returned.
	Binary bool `json:"binary,omitempty"`
	// ModeApplied echoes the preview mode applied.
	ModeApplied string `json:"modeApplied,omitempty"`
	// Continuation carries paging/truncation hints when content was clipped.
	Continuation *extension.Continuation `json:"continuation,omitempty"`
}

type ReadImageInput struct {
	// URI is an absolute URI; when provided, RootURI/RootID/Path are ignored.
	URI string `json:"uri,omitempty"`
	// RootURI/RootID + Path select an image under a root.
	RootURI string `json:"root,omitempty"`
	RootID  string `json:"rootId,omitempty"`
	Path    string `json:"path,omitempty"`

	// MaxWidth/MaxHeight define a resize-to-fit box; default 2048x768.
	MaxWidth  int `json:"maxWidth,omitempty"`
	MaxHeight int `json:"maxHeight,omitempty"`
	// MaxBytes caps the encoded output bytes; default 4MB.
	MaxBytes int `json:"maxBytes,omitempty"`

	// Format optionally forces output encoding: "png" or "jpeg".
	Format string `json:"format,omitempty"`
}

type ReadImageOutput struct {
	URI      string `json:"uri"`
	Path     string `json:"path"`
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mimeType"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Bytes    int    `json:"bytes"`
	Base64   string `json:"dataBase64"`
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
	target, err := s.resolveReadTarget(ctx, input, s.agentAllowed(ctx))
	if err != nil {
		return err
	}
	data, err := s.downloadResource(ctx, target.fullURI)
	if err != nil {
		return err
	}
	selection, err := applyReadSelection(data, input)
	if err != nil {
		return err
	}
	limitRequested := readLimitRequested(input)
	populateReadOutput(output, target, selection.Text, len(data), selection.Returned, selection.Remaining, selection.StartLine, selection.EndLine, selection.ModeApplied, limitRequested, selection.Binary, selection.OffsetBytes)
	return nil
}

func (s *Service) readImage(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*ReadImageInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*ReadImageOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	readTarget, err := s.resolveReadTarget(ctx, &ReadInput{
		URI:     input.URI,
		RootURI: input.RootURI,
		RootID:  input.RootID,
		Path:    input.Path,
	}, s.agentAllowed(ctx))
	if err != nil {
		return err
	}
	raw, err := s.downloadResource(ctx, readTarget.fullURI)
	if err != nil {
		return err
	}
	options := imageio.NormalizeOptions(imageio.Options{
		MaxWidth:  input.MaxWidth,
		MaxHeight: input.MaxHeight,
		MaxBytes:  input.MaxBytes,
		Format:    strings.TrimSpace(input.Format),
	})
	encoded, err := imageio.EncodeToFit(raw, options)
	if err != nil {
		return err
	}
	output.URI = readTarget.fullURI
	output.Path = strings.TrimSpace(input.Path)
	if output.Path == "" {
		output.Path = strings.TrimSpace(readTarget.fullURI)
	}
	output.Name = pathpkg.Base(output.Path)
	output.MimeType = encoded.MimeType
	output.Width = encoded.Width
	output.Height = encoded.Height
	output.Bytes = len(encoded.Bytes)
	output.Base64 = base64.StdEncoding.EncodeToString(encoded.Bytes)
	return nil
}

type readTarget struct {
	fullURI  string
	normRoot string
}

func (s *Service) resolveReadTarget(ctx context.Context, input *ReadInput, allowed []string) (*readTarget, error) {
	uri := strings.TrimSpace(input.URI)
	if uri != "" {
		fullURI, err := s.normalizeFullURI(ctx, uri, allowed)
		if err != nil {
			return nil, err
		}
		return &readTarget{fullURI: fullURI}, nil
	}
	rootCtx, err := s.newRootContext(ctx, input.RootURI, input.RootID, allowed)
	if err != nil {
		return nil, err
	}
	pathPart := strings.TrimSpace(input.Path)
	if pathPart == "" {
		return nil, fmt.Errorf("path is required when uri is not provided")
	}
	fullURI, err := rootCtx.ResolvePath(pathPart)
	if err != nil {
		return nil, err
	}
	return &readTarget{fullURI: fullURI, normRoot: rootCtx.Base()}, nil
}

func readLimitRequested(input *ReadInput) bool {
	if input == nil {
		return false
	}
	if strings.TrimSpace(input.Mode) != "" {
		return true
	}
	if input.MaxBytes > 0 || input.MaxLines > 0 {
		return true
	}
	if input.BytesRange.OffsetBytes > 0 || input.BytesRange.LengthBytes > 0 {
		return true
	}
	if input.LineRange.StartLine > 0 || input.LineRange.LineCount > 0 {
		return true
	}
	return false
}

func (s *Service) downloadResource(ctx context.Context, uri string) ([]byte, error) {
	if mcpuri.Is(uri) {
		if s.mcpMgr == nil {
			return nil, fmt.Errorf("mcp manager not configured")
		}
		mfs := mcpfs.New(s.mcpMgr)
		return mfs.Download(ctx, mcpfs.NewObjectFromURI(uri))
	}
	fs := afs.New()
	return fs.DownloadWithURL(ctx, uri)
}

type readSelection struct {
	Text        string
	StartLine   int
	EndLine     int
	ModeApplied string
	Returned    int
	Remaining   int
	Binary      bool
	// OffsetBytes records the starting byte offset used for this selection (0 when head/tail without explicit range).
	OffsetBytes int
}

func applyReadSelection(data []byte, input *ReadInput) (*readSelection, error) {
	const defaultMaxBytes = 8192
	appliedMode := strings.TrimSpace(strings.ToLower(input.Mode))
	if appliedMode == "" {
		appliedMode = "head"
	}
	startLine := 0
	endLine := 0

	// Binary guard: avoid returning raw binary payloads.
	if isBinaryContent(data) {
		return &readSelection{
			Text:        "[binary content omitted]",
			StartLine:   0,
			EndLine:     0,
			ModeApplied: appliedMode,
			Returned:    0,
			Remaining:   len(data),
			Binary:      true,
			OffsetBytes: 0,
		}, nil
	}

	// Compute selection into these variables, then return once at the end.
	var text string
	var returned, remaining, offsetBytes int

	// Default text is the whole file; branches below will override.
	text = string(data)

	// Byte range selection
	if input.BytesRange.OffsetBytes > 0 || input.BytesRange.LengthBytes > 0 {
		clipped, start, _, err := textclip.ClipBytesByRange(data, input.BytesRange)
		if err != nil {
			return nil, err
		}
		text = string(clipped)
		offsetBytes = start
		returned = len(text)
		remaining = len(data) - (start + returned)
		if remaining < 0 {
			remaining = 0
		}
	} else if input.LineRange.StartLine > 0 || input.LineRange.LineCount > 0 {
		// Line range selection
		clipped, start, _, err := textclip.ClipLinesByRange(data, input.LineRange)
		if err != nil {
			return nil, err
		}
		text = string(clipped)
		if input.LineRange.StartLine > 0 {
			startLine = input.LineRange.StartLine
			if input.LineRange.LineCount > 0 {
				endLine = startLine + input.LineRange.LineCount - 1
			}
		}
		offsetBytes = start
		returned = len(text)
		remaining = len(data) - (start + returned)
		if remaining < 0 {
			remaining = 0
		}
	} else {
		// Head/Tail/Signatures modes
		maxBytes := input.MaxBytes
		if maxBytes <= 0 {
			maxBytes = defaultMaxBytes
		}
		maxLines := input.MaxLines
		if maxLines < 0 {
			maxLines = 0
		}
		text, returned, remaining = applyMode(text, len(data), appliedMode, maxBytes, maxLines)
		offsetBytes = 0
	}

	rs := &readSelection{
		Text:        text,
		StartLine:   startLine,
		EndLine:     endLine,
		ModeApplied: appliedMode,
		Returned:    returned,
		Remaining:   remaining,
		Binary:      false,
		OffsetBytes: offsetBytes,
	}
	return rs, nil
}

func applyMode(text string, totalSize int, mode string, maxBytes, maxLines int) (string, int, int) {
	switch mode {
	case "tail":
		return textclip.ClipTail(text, totalSize, maxBytes, maxLines)
	case "signatures":
		if sig := textclip.ExtractSignatures(text, maxBytes); sig != "" {
			return sig, len(sig), clipRemaining(totalSize, len(sig))
		}
		// fallback to head if no signatures found
	}
	return textclip.ClipHead(text, totalSize, maxBytes, maxLines)
}

func clipRemaining(totalSize, returned int) int {
	if totalSize <= returned {
		return 0
	}
	return totalSize - returned
}

func isBinaryContent(data []byte) bool {
	if !utf8.Valid(data) {
		return true
	}
	const maxInspect = 1024
	limit := len(data)
	if limit > maxInspect {
		limit = maxInspect
	}
	control := 0
	for _, b := range data[:limit] {
		if b == 0 {
			return true
		}
		if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			control++
		}
	}
	return control > limit/10
}

func populateReadOutput(out *ReadOutput, target *readTarget, content string, size, returned, remaining, startLine, endLine int, mode string, limitRequested bool, binary bool, byteOffset int) {
	out.URI = target.fullURI
	if target.normRoot != "" {
		out.Path = relativePath(target.normRoot, target.fullURI)
	} else {
		out.Path = target.fullURI
	}
	out.Content = content
	out.Size = size
	out.Returned = returned
	out.Remaining = remaining
	out.StartLine = startLine
	out.EndLine = endLine
	out.ModeApplied = mode
	out.Binary = binary
	truncated := returned > 0 && size > returned
	if !limitRequested {
		truncated = false
	}
	if remaining <= 0 && truncated {
		remaining = size - (byteOffset + returned)
		if remaining < 0 {
			remaining = 0
		}
		out.Remaining = remaining
	}
	if limitRequested && (remaining > 0 || truncated) {
		out.Continuation = &extension.Continuation{
			HasMore:   true,
			Remaining: remaining,
			Returned:  returned,
			Mode:      mode,
			Binary:    binary,
		}
		if out.Continuation.Remaining < 0 {
			out.Continuation.Remaining = 0
		}
		if out.Continuation.Returned < 0 {
			out.Continuation.Returned = 0
		}
		// Compute next byte range based on current offset and returned size.
		nextOffset := byteOffset + returned
		nextLength := returned
		if remaining > 0 && nextLength > remaining {
			nextLength = remaining
		}
		if remaining <= 0 {
			// No continuation when nothing remains.
			out.Continuation = nil
		} else {
			out.Continuation.NextRange = &extension.RangeHint{
				Bytes: &extension.ByteRange{
					Offset: nextOffset,
					Length: nextLength,
				},
			}
			// Optionally include line hints when present
			if endLine > 0 && startLine > 0 {
				count := endLine - startLine + 1
				if count < 0 {
					count = 0
				}
				out.Continuation.NextRange.Lines = &extension.LineRange{Start: endLine + 1, Count: count}
			}
		}
	}
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
	AllowedGrepSearch bool   `json:"allowedGrepSearch"`
	Role              string `json:"role,omitempty"`
}

type RootsOutput struct {
	Roots []Root `json:"roots"`
}

type rootCollection struct {
	user   []Root
	system []Root
}

func (c *rootCollection) all() []Root {
	if c == nil {
		return nil
	}
	out := make([]Root, 0, len(c.user)+len(c.system))
	out = append(out, c.user...)
	out = append(out, c.system...)
	return out
}

func (s *Service) collectRoots(ctx context.Context) (*rootCollection, error) {
	locs := s.agentAllowed(ctx)
	if len(locs) == 0 {
		locs = append([]string(nil), s.defaults.Locations...)
	}
	if len(locs) == 0 {
		return &rootCollection{}, nil
	}
	curAgent := s.currentAgent(ctx)
	seen := map[string]bool{}
	var userRoots []Root
	var systemRoots []Root
	for _, loc := range locs {
		root := strings.TrimSpace(loc)
		if root == "" {
			continue
		}
		wsRoot, kind, err := s.normalizeUserRoot(ctx, root)
		if err != nil || wsRoot == "" {
			continue
		}
		if seen[wsRoot] {
			continue
		}
		seen[wsRoot] = true
		desc := s.tryDescribe(ctx, wsRoot, kind)
		role := "user"
		rootID := wsRoot
		if curAgent != nil && len(curAgent.Resources) > 0 {
			for _, r := range curAgent.Resources {
				if r == nil || strings.TrimSpace(r.URI) == "" {
					continue
				}
				normRes, _, err := s.normalizeUserRoot(ctx, r.URI)
				if err != nil || strings.TrimSpace(normRes) == "" {
					continue
				}
				if strings.TrimRight(strings.TrimSpace(normRes), "/") == strings.TrimRight(strings.TrimSpace(wsRoot), "/") {
					if strings.EqualFold(strings.TrimSpace(r.Role), "system") {
						role = "system"
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
		semAllowed := s.semanticAllowedForAgent(ctx, curAgent, wsRoot)
		grepAllowed := s.grepAllowedForAgent(ctx, curAgent, wsRoot)
		rootEntry := Root{
			ID:                    rootID,
			URI:                   wsRoot,
			Description:           desc,
			AllowedSemanticSearch: semAllowed,
			AllowedGrepSearch:     grepAllowed,
			Role:                  role,
		}
		if role == "system" {
			systemRoots = append(systemRoots, rootEntry)
			continue
		}
		userRoots = append(userRoots, rootEntry)
	}
	return &rootCollection{user: userRoots, system: systemRoots}, nil
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

	collected, err := s.collectRoots(ctx)
	if err != nil {
		return err
	}
	var roots []Root
	appendWithLimit := func(source []Root) {
		for _, r := range source {
			if max > 0 && len(roots) >= max {
				return
			}
			roots = append(roots, r)
		}
	}
	appendWithLimit(collected.user)
	appendWithLimit(collected.system)
	output.Roots = roots
	return nil
}

// normalizeLocation was unused; removed to reduce file size and duplication.

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

	// If the context was canceled/deadlined and we couldn't resolve the agent,
	// retry once with a background context that preserves identity and conversation id.
	if curAgent == nil && (errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded)) {
		userID := strings.TrimSpace(authctx.EffectiveUserID(ctx))
		bg := context.Background()
		if userID != "" {
			bg = authctx.WithUserInfo(bg, &authctx.UserInfo{Subject: userID})
		}
		if convID := strings.TrimSpace(memory.ConversationIDFromContext(ctx)); convID != "" {
			bg = memory.WithConversationID(bg, convID)
		}
		curAgent = s.currentAgent(bg)
	}

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
	curAgent := s.currentAgent(ctx)
	allowed := s.agentAllowed(ctx)
	rootCtx, err := s.newRootContext(ctx, rootURI, input.RootID, allowed)
	if err != nil {
		return err
	}
	wsRoot := rootCtx.Workspace()
	// Enforce per-resource grep capability when agent context is available.
	if !s.grepAllowedForAgent(ctx, curAgent, wsRoot) {
		return fmt.Errorf("grep not allowed for root: %s", rootURI)
	}
	// Currently grepFiles is implemented for local/workspace-backed roots only.
	if mcpuri.Is(wsRoot) {
		return fmt.Errorf("grepFiles is not supported for mcp resources")
	}
	rootBase := rootCtx.Base()
	base := rootBase
	if trimmed := strings.TrimSpace(input.Path); trimmed != "" {
		base, err = rootCtx.ResolvePath(trimmed)
		if err != nil {
			return err
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

	// Local/workspace handling (MCP roots are rejected earlier in this method)
	fs := afs.New()

	// When input.Path resolves to a file, avoid Walk (which assumes directories for file:// roots).
	if obj, err := fs.Object(ctx, base); err == nil && obj != nil && !obj.IsDir() {
		uri := base
		data, err := fs.DownloadWithURL(ctx, uri)
		if err != nil {
			return err
		}
		if maxSize > 0 && len(data) > maxSize {
			data = data[:maxSize]
		}
		if skipBinary && isBinary(data) {
			output.Stats = stats
			output.Files = nil
			return nil
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
			output.Stats = stats
			output.Files = nil
			return nil
		}

		rel := relativePath(rootBase, uri)
		if rel == "" {
			rel = relativePath(base, uri)
		}
		if rel == "" {
			rel = uri
		}
		if !globAllowed(rel, input.Include, input.Exclude) {
			output.Stats = stats
			output.Files = nil
			return nil
		}

		stats.Scanned = 1
		stats.Matched = 1
		gf := hygine.GrepFile{Path: rel, URI: uri, Matches: len(matchLines)}
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
			output.Stats = stats
			output.Files = files
			return nil
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
		}
		files = append(files, gf)
		output.Stats = stats
		output.Files = files
		return nil
	}

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
