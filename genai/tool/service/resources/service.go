package resources

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
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
	mcpcfg "github.com/viant/agently/internal/mcp/config"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
	mcpuri "github.com/viant/agently/internal/mcp/uri"
	"github.com/viant/agently/internal/workspace"
	mcprepo "github.com/viant/agently/internal/workspace/repository/mcp"
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

	descMu    sync.RWMutex
	descCache map[string]string
	mfsMu     sync.Mutex
	mfs       *mcpfs.Service
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

func (s *Service) mcpFS(ctx context.Context) (*mcpfs.Service, error) {
	if s.mcpMgr == nil {
		return nil, fmt.Errorf("mcp manager not configured (resources/mcpfs)")
	}
	s.mfsMu.Lock()
	defer s.mfsMu.Unlock()
	if s.mfs == nil {
		opts := []mcpfs.Option{}
		if strings.TrimSpace(s.defaults.SnapshotPath) != "" {
			opts = append(opts, mcpfs.WithSnapshotCacheRoot(s.defaults.SnapshotPath))
		}
		s.mfs = mcpfs.New(s.mcpMgr, opts...)
	}
	resolver := s.mcpSnapshotResolver(ctx)
	if resolver != nil {
		s.mfs.SetSnapshotResolver(resolver)
	}
	manifestResolver := s.mcpSnapshotManifestResolver(ctx)
	if manifestResolver != nil {
		s.mfs.SetSnapshotManifestResolver(manifestResolver)
	}
	return s.mfs, nil
}

// Name returns service name
func (s *Service) Name() string { return Name }

// ToolTimeout suggests a longer timeout for resources tools that may index large roots.
func (s *Service) ToolTimeout() time.Duration { return 15 * time.Minute }

// Methods declares available tool methods
func (s *Service) Methods() svc.Signatures {
	return []svc.Signature{
		{Name: "roots", Description: "Discover configured resource roots with optional descriptions", Input: reflect.TypeOf(&RootsInput{}), Output: reflect.TypeOf(&RootsOutput{})},
		{Name: "list", Description: "List resources under a root (file or MCP)", Input: reflect.TypeOf(&ListInput{}), Output: reflect.TypeOf(&ListOutput{})},
		{Name: "read", Description: "Read a single resource under a root. For large files, prefer byteRange and page in chunks (<= 8KB).", Input: reflect.TypeOf(&ReadInput{}), Output: reflect.TypeOf(&ReadOutput{})},
		{Name: "readImage", Description: "Read an image under a root and return a base64 payload suitable for attaching as a vision input. Defaults to resizing to fit 2048x768.", Input: reflect.TypeOf(&ReadImageInput{}), Output: reflect.TypeOf(&ReadImageOutput{})},
		{Name: "match", Description: "Semantic match search over one or more roots; use `match.exclusions` to block specific  paths.", Input: reflect.TypeOf(&MatchInput{}), Output: reflect.TypeOf(&MatchOutput{})},
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
				var unresolved []string
				curAgent := s.currentAgent(ctx)
				for _, id := range remaining {
					uri, err := s.resolveRootID(ctx, id)
					if err != nil || strings.TrimSpace(uri) == "" {
						unresolved = append(unresolved, id)
						continue
					}
					if !s.semanticAllowedForAgent(ctx, curAgent, uri) {
						return nil, fmt.Errorf("rootId not semantic-enabled: %s", id)
					}
					add([]Root{{
						ID:                    strings.TrimSpace(id),
						URI:                   uri,
						AllowedSemanticSearch: true,
						Role:                  "system",
					}})
				}
				if len(unresolved) > 0 {
					return nil, fmt.Errorf("unknown rootId(s): %s", strings.Join(unresolved, ", "))
				}
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
	v := strings.TrimSpace(value)
	if mcpuri.Is(v) {
		v = mcpuri.NormalizeForCompare(v)
	}
	return strings.ToLower(strings.TrimSpace(v))
}

func normalizeRootURI(value string) string {
	v := strings.TrimSpace(value)
	if mcpuri.Is(v) {
		v = mcpuri.NormalizeForCompare(v)
	} else {
		v = strings.TrimRight(v, "/")
	}
	return strings.ToLower(strings.TrimSpace(v))
}

func assignRootMetadata(doc *embSchema.Document, roots []searchRootMeta) {
	if doc == nil || len(roots) == 0 {
		return
	}
	path := documentMetadataPath(doc.Metadata)
	if path == "" {
		return
	}
	normalized := normalizeWorkspaceKey(path)
	for _, entry := range roots {
		prefix := normalizeWorkspaceKey(entry.wsRoot)
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

func (s *Service) agentResources(ctx context.Context, ag *agmodel.Agent) []*agmodel.Resource {
	if ag == nil || len(ag.Resources) == 0 {
		return nil
	}
	var out []*agmodel.Resource
	seen := map[string]struct{}{}
	for _, r := range ag.Resources {
		if r == nil {
			continue
		}
		if strings.TrimSpace(r.URI) == "" && strings.TrimSpace(r.MCP) != "" {
			for _, expanded := range s.expandMCPResources(ctx, r) {
				if expanded == nil || strings.TrimSpace(expanded.URI) == "" {
					continue
				}
				key := normalizeRootURI(expanded.URI)
				if key == "" {
					continue
				}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				out = append(out, expanded)
			}
			continue
		}
		if strings.TrimSpace(r.URI) == "" {
			continue
		}
		key := normalizeRootURI(r.URI)
		if key != "" {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, r)
	}
	return out
}

func (s *Service) expandMCPResources(ctx context.Context, base *agmodel.Resource) []*agmodel.Resource {
	if s == nil || s.mcpMgr == nil || base == nil {
		return nil
	}
	server := strings.TrimSpace(base.MCP)
	if server == "" {
		return nil
	}
	opts, err := s.mcpMgr.Options(ctx, server)
	if err != nil || opts == nil {
		fmt.Printf("resources: mcp include server=%q err=%v\n", server, err)
		return nil
	}
	roots := mcpcfg.ResourceRoots(opts.Metadata)
	if len(roots) == 0 {
		fmt.Printf("resources: mcp include server=%q no roots\n", server)
		return nil
	}
	selectors := make([]string, 0, len(base.Roots))
	for _, r := range base.Roots {
		if v := strings.TrimSpace(r); v != "" {
			selectors = append(selectors, v)
		}
	}
	matchAll := len(selectors) == 0
	for _, sel := range selectors {
		if sel == "*" || strings.EqualFold(sel, "all") {
			matchAll = true
			break
		}
	}
	var out []*agmodel.Resource
	for _, root := range roots {
		uri := strings.TrimRight(strings.TrimSpace(root.URI), "/")
		if uri == "" {
			continue
		}
		if !matchAll && !matchesMCPSelector(selectors, root) {
			continue
		}
		if mcpuri.Is(uri) {
			uri = normalizeMCPURI(uri)
		}
		rootID := strings.TrimSpace(root.ID)
		if rootID == "" {
			rootID = uri
		}
		role := strings.TrimSpace(base.Role)
		if role == "" {
			role = "user"
		}
		res := &agmodel.Resource{
			ID:          rootID,
			URI:         uri,
			Role:        role,
			Binding:     base.Binding,
			MaxFiles:    base.MaxFiles,
			TrimPath:    base.TrimPath,
			Match:       mergeMatchOptions(base.Match, root.Include, root.Exclude, root.MaxSizeBytes),
			MinScore:    base.MinScore,
			Description: strings.TrimSpace(root.Description),
		}
		if strings.TrimSpace(base.Description) != "" {
			res.Description = strings.TrimSpace(base.Description)
		}
		if base.AllowSemanticMatch != nil {
			res.AllowSemanticMatch = base.AllowSemanticMatch
		} else {
			allowed := root.Vectorize && root.Snapshot
			res.AllowSemanticMatch = &allowed
		}
		if base.AllowGrep != nil {
			res.AllowGrep = base.AllowGrep
		} else {
			allowed := root.AllowGrep && root.Snapshot
			res.AllowGrep = &allowed
		}
		out = append(out, res)
	}
	return out
}

func mergeMatchOptions(base *embopt.Options, include, exclude []string, maxSizeBytes int64) *embopt.Options {
	if base == nil && len(include) == 0 && len(exclude) == 0 && maxSizeBytes <= 0 {
		return nil
	}
	out := &embopt.Options{}
	if base != nil {
		*out = *base
	}
	if len(out.Inclusions) == 0 && len(include) > 0 {
		out.Inclusions = append([]string(nil), include...)
	}
	if len(out.Exclusions) == 0 && len(exclude) > 0 {
		out.Exclusions = append([]string(nil), exclude...)
	}
	if out.MaxFileSize == 0 && maxSizeBytes > 0 {
		out.MaxFileSize = int(maxSizeBytes)
	}
	return out
}

func mergeEffectiveMatch(rootMatch, inputMatch *embopt.Options) *embopt.Options {
	if rootMatch == nil && inputMatch == nil {
		return nil
	}
	out := &embopt.Options{}
	if rootMatch != nil {
		*out = *rootMatch
	}
	if inputMatch != nil {
		if len(inputMatch.Inclusions) > 0 {
			out.Inclusions = append([]string(nil), inputMatch.Inclusions...)
		}
		if len(inputMatch.Exclusions) > 0 {
			out.Exclusions = append([]string(nil), inputMatch.Exclusions...)
		}
		if inputMatch.MaxFileSize > 0 {
			out.MaxFileSize = inputMatch.MaxFileSize
		}
	}
	if len(out.Inclusions) == 0 && len(out.Exclusions) == 0 && out.MaxFileSize == 0 {
		return nil
	}
	return out
}

func matchKey(rootMatch, inputMatch *embopt.Options) string {
	eff := mergeEffectiveMatch(rootMatch, inputMatch)
	if eff == nil {
		return ""
	}
	return fmt.Sprintf("max=%d|incl=%s|excl=%s", eff.MaxFileSize, strings.Join(eff.Inclusions, ","), strings.Join(eff.Exclusions, ","))
}

func matchesMCPSelector(selectors []string, root mcpcfg.ResourceRoot) bool {
	if len(selectors) == 0 {
		return true
	}
	rootURI := strings.TrimSpace(root.URI)
	rootID := strings.TrimSpace(root.ID)
	if rootID == "" {
		rootID = rootURI
	}
	for _, sel := range selectors {
		if sel == "" {
			continue
		}
		if normalizeRootID(sel) == normalizeRootID(rootID) {
			return true
		}
		if normalizeRootURI(sel) == normalizeRootURI(rootURI) {
			return true
		}
	}
	return false
}

func normalizeMCPURI(value string) string {
	if !mcpuri.Is(value) {
		return value
	}
	server, uri := mcpuri.Parse(value)
	if strings.TrimSpace(server) == "" {
		return value
	}
	normalized := mcpuri.Canonical(server, uri)
	if normalized == "" {
		return value
	}
	return normalized
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
	if mcpRoots := s.collectMCPRoots(ctx); len(mcpRoots) > 0 {
		byURI := map[string]bool{}
		for _, root := range availableRoots {
			key := strings.TrimRight(strings.TrimSpace(root.URI), "/")
			if key != "" {
				byURI[key] = true
			}
		}
		for _, root := range mcpRoots {
			key := strings.TrimRight(strings.TrimSpace(root.URI), "/")
			if key == "" || byURI[key] {
				continue
			}
			byURI[key] = true
			availableRoots = append(availableRoots, root)
		}
	}
	if len(availableRoots) == 0 {
		return nil, fmt.Errorf("no roots configured for semantic search")
	}
	selectedRoots, err := s.selectSearchRoots(ctx, availableRoots, input)
	if err != nil {
		return nil, err
	}
	var localRoots []aug.LocalRoot
	for _, root := range selectedRoots {
		if root.UpstreamRef == "" || mcpuri.Is(root.URI) {
			continue
		}
		localRoots = append(localRoots, aug.LocalRoot{
			ID:          root.ID,
			URI:         root.URI,
			UpstreamRef: root.UpstreamRef,
		})
	}
	if len(localRoots) > 0 {
		ctx = aug.WithLocalRoots(ctx, localRoots)
	}
	type locInfo struct {
		location string
		db       string
		match    *embopt.Options
	}
	locations := make([]string, 0, len(selectedRoots))
	locInfos := make([]locInfo, 0, len(selectedRoots))
	searchRoots := make([]searchRootMeta, 0, len(selectedRoots))
	for _, root := range selectedRoots {
		wsRoot := strings.TrimRight(strings.TrimSpace(root.URI), "/")
		if wsRoot == "" {
			continue
		}
		base := wsRoot
		if mcpuri.Is(wsRoot) {
			base = normalizeMCPURI(wsRoot)
		}
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
		locInfos = append(locInfos, locInfo{location: base, db: strings.TrimSpace(root.DB), match: root.Match})
		searchRoots = append(searchRoots, searchRootMeta{
			id:     root.ID,
			wsRoot: wsRoot,
		})
	}
	if len(locations) == 0 {
		return nil, fmt.Errorf("no valid roots provided")
	}
	trimPrefix := commonPrefix(locations)
	type matchGroup struct {
		db    string
		match *embopt.Options
		locs  []string
	}
	grouped := map[string]*matchGroup{}
	for _, li := range locInfos {
		key := li.db + "|" + matchKey(li.match, input.Match)
		g, ok := grouped[key]
		if !ok {
			g = &matchGroup{db: li.db, match: mergeEffectiveMatch(li.match, input.Match)}
			grouped[key] = g
		}
		g.locs = append(g.locs, li.location)
	}
	var allDocs []embSchema.Document
	for _, group := range grouped {
		augIn := &aug.AugmentDocsInput{
			Query:        query,
			Locations:    group.locs,
			Match:        group.match,
			Model:        embedderID,
			DB:           group.db,
			MaxDocuments: input.MaxDocuments,
			IncludeFile:  input.IncludeFile,
			TrimPath:     trimPrefix,
			AllowPartial: true,
		}
		var augOut aug.AugmentDocsOutput
		if err := s.runAugmentDocs(ctx, augIn, &augOut); err != nil {
			return nil, err
		}
		allDocs = append(allDocs, augOut.Documents...)
	}
	sort.SliceStable(allDocs, func(i, j int) bool { return allDocs[i].Score > allDocs[j].Score })

	curAgent := s.currentAgent(ctx)
	sysPrefixes := systemdoc.Prefixes(curAgent)
	for i := range allDocs {
		doc := &allDocs[i]
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
		documents:      allDocs,
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
	fmt.Printf("resources: match request query=%q roots=%v rootIds=%v path=%q\n", input.Query, input.Roots, input.RootIDs, input.Path)
	res, err := s.buildAugmentedDocuments(ctx, input)
	if err != nil {
		fmt.Printf("resources: match error query=%q err=%v\n", input.Query, err)
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
	fmt.Printf("resources: match response query=%q docs=%d cursor=%d next=%d\n", input.Query, len(pageDocs), output.Cursor, output.NextCursor)
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
	fmt.Printf("resources: matchDocuments request query=%q rootIds=%v path=%q\n", input.Query, input.RootIDs, input.Path)
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
		fmt.Printf("resources: matchDocuments error query=%q err=%v\n", input.Query, err)
		return err
	}
	docs := res.documents
	ranked := uniqueMatchedDocuments(docs, maxDocs)
	if len(ranked) == 0 {
		output.Documents = nil
		fmt.Printf("resources: matchDocuments response query=%q docs=0\n", input.Query)
		return nil
	}
	output.Documents = ranked
	fmt.Printf("resources: matchDocuments response query=%q docs=%d\n", input.Query, len(ranked))
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
	// pattern are returned. Globs use path-style matching rules and support
	// globstar ("**") to match any directory depth.
	Include []string `json:"include,omitempty" description:"optional file/path globs to include (relative to root+path); supports ** for any depth"`
	// Exclude defines optional file or path globs to exclude. When provided,
	// any item whose relative path or base name matches a pattern is
	// filtered out.
	Exclude []string `json:"exclude,omitempty" description:"optional file/path globs to exclude; supports ** for any depth"`
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
	// path.Match does not support "**" (globstar). Implement a minimal globstar
	// matcher where a full path segment equal to "**" can match zero or more
	// path segments.
	if strings.Contains(pattern, "**") {
		return listGlobStarMatch(pattern, value)
	}
	ok, err := pathpkg.Match(pattern, value)
	return err == nil && ok
}

func listGlobStarMatch(pattern, value string) bool {
	pattern = strings.TrimSpace(strings.ReplaceAll(pattern, "\\", "/"))
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if pattern == "" || value == "" {
		return false
	}

	pattern = strings.TrimPrefix(pattern, "./")
	pattern = strings.TrimPrefix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")

	value = strings.TrimPrefix(value, "./")
	value = strings.TrimPrefix(value, "/")
	value = strings.TrimSuffix(value, "/")

	pSegs := splitListGlob(pattern)
	vSegs := splitListGlob(value)

	type state struct{ i, j int }
	seen := make(map[state]bool, len(pSegs)*len(vSegs))
	memo := make(map[state]bool, len(pSegs)*len(vSegs))
	var match func(i, j int) bool
	match = func(i, j int) bool {
		s := state{i: i, j: j}
		if seen[s] {
			return memo[s]
		}
		seen[s] = true

		var ok bool
		switch {
		case i >= len(pSegs):
			ok = j >= len(vSegs)
		case pSegs[i] == "**":
			// "**" matches zero segments (advance pattern) or one segment (advance value).
			ok = match(i+1, j) || (j < len(vSegs) && match(i, j+1))
		case j >= len(vSegs):
			ok = false
		default:
			segOK, err := pathpkg.Match(pSegs[i], vSegs[j])
			ok = err == nil && segOK && match(i+1, j+1)
		}
		memo[s] = ok
		return ok
	}

	return match(0, 0)
}

func splitListGlob(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
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
		fmt.Printf("resources: list resolve error rootId=%q root=%q err=%v\n", input.RootID, input.RootURI, err)
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
		var err error
		mfs, err = s.mcpFS(ctx)
		if err != nil {
			return err
		}
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
	textclip.LineRange

	// MaxBytes caps the returned payload when neither byte nor line ranges are provided.
	// When zero, defaults are applied.
	MaxBytes int `json:"maxBytes,omitempty"`

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

	// IncludeData controls whether dataBase64 is returned in the tool response.
	// When false (default), the tool writes the encoded image to EncodedURI and
	// omits dataBase64 to keep tool output small.
	IncludeData bool `json:"includeData,omitempty"`
	// DestURL optionally specifies where to write the encoded image (file://...).
	DestURL string `json:"destURL,omitempty"`
}

type ReadImageOutput struct {
	URI      string `json:"uri"`
	Encoded  string `json:"encodedURI,omitempty"`
	Path     string `json:"path"`
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mimeType"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Bytes    int    `json:"bytes"`
	Base64   string `json:"dataBase64,omitempty"`
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
		fmt.Printf("resources: read resolve error rootId=%q root=%q uri=%q err=%v\n", input.RootID, input.RootURI, input.URI, err)
		return err
	}
	data, err := s.downloadResource(ctx, target.fullURI)
	if err != nil {
		fmt.Printf("resources: read download error uri=%q err=%v\n", target.fullURI, err)
		return err
	}
	selection, err := applyReadSelection(data, input)
	if err != nil {
		fmt.Printf("resources: read selection error uri=%q err=%v\n", target.fullURI, err)
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
	encodedURI, err := imageio.StoreEncodedImage(ctx, encoded, imageio.StoreOptions{DestURL: strings.TrimSpace(input.DestURL)})
	if err != nil {
		return err
	}
	output.Encoded = encodedURI
	if input.IncludeData {
		output.Base64 = base64.StdEncoding.EncodeToString(encoded.Bytes)
	}
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
	if input.MaxBytes > 0 || input.LineCount > 0 {
		return true
	}
	if input.BytesRange.OffsetBytes > 0 || input.BytesRange.LengthBytes > 0 {
		return true
	}
	if input.StartLine > 0 {
		return true
	}
	return false
}

func (s *Service) downloadResource(ctx context.Context, uri string) ([]byte, error) {
	if mcpuri.Is(uri) {
		mfs, err := s.mcpFS(ctx)
		if err != nil {
			return nil, fmt.Errorf("resources: download mcp uri=%q: %w", uri, err)
		}
		data, err := mfs.DownloadDirect(ctx, mcpfs.NewObjectFromURI(uri))
		if err == nil {
			return data, nil
		}
		fmt.Printf("resources: download direct failed uri=%q err=%v; falling back to snapshot\n", uri, err)
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
	} else if input.StartLine > 0 {
		// Line range selection
		lineRange := textclip.LineRange{StartLine: input.StartLine, LineCount: input.LineCount}
		if lineRange.LineCount < 0 {
			lineRange.LineCount = 0
		}
		clipped, start, _, err := textclip.ClipLinesByRange(data, lineRange)
		if err != nil {
			return nil, err
		}
		text = string(clipped)
		startLine = input.StartLine
		if lineRange.LineCount > 0 {
			endLine = startLine + lineRange.LineCount - 1
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
		maxLines := input.LineCount
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
	uKey := normalizeWorkspaceKey(loc)
	if uKey == "" {
		return false
	}
	// Compare canonical workspace:// or mcp: prefixes
	for _, a := range allowed {
		aKey := normalizeWorkspaceKey(a)
		if aKey == "" {
			continue
		}
		if strings.HasPrefix(uKey, aKey) {
			return true
		}
	}
	return false
}

func normalizeWorkspaceKey(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	if mcpuri.Is(v) {
		v = mcpuri.NormalizeForCompare(v)
	} else {
		v = strings.TrimRight(v, "/")
	}
	return strings.ToLower(strings.TrimSpace(v))
}

// -------- roots implementation --------

type ResourcesDefaults struct {
	Locations    []string
	TrimPath     string
	SummaryFiles []string
	DescribeMCP  bool
	SnapshotPath string
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
	// UpstreamRef is an internal-only reference used to resolve local upstream sync.
	UpstreamRef string `json:"-"`
	// DB is an optional embedius sqlite database path override for this root.
	DB string `json:"-"`
	// Match carries per-root match options (include/exclude/max file size).
	Match *embopt.Options `json:"match,omitempty"`
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
		desc := ""
		role := "user"
		rootID := wsRoot
		upstreamRef := ""
		rootDB := ""
		var rootMatch *embopt.Options
		if curAgent != nil {
			for _, r := range s.agentResources(ctx, curAgent) {
				if r == nil || strings.TrimSpace(r.URI) == "" {
					continue
				}
				normRes, _, err := s.normalizeUserRoot(ctx, r.URI)
				if err != nil || strings.TrimSpace(normRes) == "" {
					continue
				}
				if normalizeWorkspaceKey(normRes) == normalizeWorkspaceKey(wsRoot) {
					if strings.EqualFold(strings.TrimSpace(r.Role), "system") {
						role = "system"
					}
					if strings.TrimSpace(r.ID) != "" {
						rootID = strings.TrimSpace(r.ID)
					}
					if strings.TrimSpace(r.Description) != "" {
						desc = strings.TrimSpace(r.Description)
					}
					if strings.TrimSpace(r.UpstreamRef) != "" {
						upstreamRef = strings.TrimSpace(r.UpstreamRef)
					}
					if strings.TrimSpace(r.DB) != "" {
						rootDB = strings.TrimSpace(r.DB)
					}
					if r.Match != nil {
						rootMatch = r.Match
					}
					break
				}
			}
		}
		if desc == "" && (kind != "mcp" || s.defaults.DescribeMCP) {
			desc = s.describeCached(ctx, wsRoot, kind)
		}
		semAllowed := s.semanticAllowedForAgent(ctx, curAgent, wsRoot)
		grepAllowed := s.grepAllowedForAgent(ctx, curAgent, wsRoot)
		rootEntry := Root{
			ID:                    rootID,
			URI:                   wsRoot,
			Description:           desc,
			UpstreamRef:           upstreamRef,
			DB:                    rootDB,
			AllowedSemanticSearch: semAllowed,
			AllowedGrepSearch:     grepAllowed,
			Role:                  role,
		}
		if semAllowed && rootMatch != nil {
			rootEntry.Match = rootMatch
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
	mcpRoots := s.collectMCPRoots(ctx)
	var roots []Root
	seen := map[string]bool{}
	appendWithLimit := func(source []Root) {
		for _, r := range source {
			if max > 0 && len(roots) >= max {
				return
			}
			key := strings.TrimRight(strings.TrimSpace(r.URI), "/")
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			roots = append(roots, r)
		}
	}
	appendWithLimit(collected.user)
	appendWithLimit(collected.system)
	appendWithLimit(mcpRoots)
	output.Roots = roots
	return nil
}

func (s *Service) collectMCPRoots(ctx context.Context) []Root {
	if s == nil || s.mcpMgr == nil {
		return nil
	}
	repo := mcprepo.New(afs.New())
	names, err := repo.List(ctx)
	if err != nil || len(names) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var roots []Root
	for _, name := range names {
		opts, err := s.mcpMgr.Options(ctx, name)
		if err != nil || opts == nil {
			continue
		}
		for _, root := range mcpcfg.ResourceRoots(opts.Metadata) {
			uri := strings.TrimRight(strings.TrimSpace(root.URI), "/")
			if uri == "" || seen[uri] {
				continue
			}
			seen[uri] = true
			rootID := strings.TrimSpace(root.ID)
			if rootID == "" {
				rootID = uri
			}
			semAllowed := root.Vectorize && root.Snapshot
			match := mergeMatchOptions(nil, root.Include, root.Exclude, root.MaxSizeBytes)
			rootEntry := Root{
				ID:                    rootID,
				URI:                   uri,
				Description:           strings.TrimSpace(root.Description),
				AllowedSemanticSearch: root.Vectorize && root.Snapshot,
				AllowedGrepSearch:     root.AllowGrep && root.Snapshot,
				Role:                  "system",
			}
			if semAllowed && match != nil {
				rootEntry.Match = match
			}
			roots = append(roots, rootEntry)
		}
	}
	return roots
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
		for _, r := range s.agentResources(ctx, curAgent) {
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
	// Check MCP roots defined in MCP client metadata (allows simple IDs like "mediator").
	if s.mcpMgr != nil {
		for _, root := range s.collectMCPRoots(ctx) {
			if normalizeRootID(root.ID) != normalizeRootID(id) {
				continue
			}
			norm, _, err := s.normalizeUserRoot(ctx, root.URI)
			if err != nil {
				return "", err
			}
			if strings.TrimSpace(norm) == "" {
				break
			}
			return norm, nil
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
	// Treat github://... as shorthand for the github MCP server.
	if strings.HasPrefix(strings.ToLower(u), "github://") {
		return mcpuri.Canonical("github", u), "mcp", nil
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
		mfs, err := s.mcpFS(ctx)
		if err != nil {
			return ""
		}
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

func (s *Service) describeCached(ctx context.Context, uri, kind string) string {
	key := kind + "|" + normalizeWorkspaceKey(uri)
	if key == "" {
		return ""
	}
	s.descMu.RLock()
	if s.descCache != nil {
		if val, ok := s.descCache[key]; ok {
			s.descMu.RUnlock()
			return val
		}
	}
	s.descMu.RUnlock()

	desc := s.tryDescribe(ctx, uri, kind)
	s.descMu.Lock()
	if s.descCache == nil {
		s.descCache = map[string]string{}
	}
	s.descCache[key] = desc
	s.descMu.Unlock()
	return desc
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
	if ag == nil {
		return nil
	}
	expanded := s.agentResources(ctx, ag)
	if len(expanded) == 0 {
		return nil
	}
	out := make([]string, 0, len(expanded))
	for _, e := range expanded {
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
	if ws == "" || ag == nil {
		if mcpuri.Is(ws) {
			if meta, ok := s.mcpRootMeta(ctx, ws); ok && meta != nil {
				return meta.Vectorize && meta.Snapshot
			}
			return false
		}
		return true
	}
	for _, r := range s.agentResources(ctx, ag) {
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
	if mcpuri.Is(ws) {
		if meta, ok := s.mcpRootMeta(ctx, ws); ok && meta != nil {
			return meta.Vectorize && meta.Snapshot
		}
		return false
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
	if ag == nil {
		if isMCP {
			if meta, ok := s.mcpRootMeta(ctx, ws); ok && meta != nil {
				return meta.AllowGrep && meta.Snapshot
			}
			return false
		}
		return true
	}
	for _, r := range s.agentResources(ctx, ag) {
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
		if meta, ok := s.mcpRootMeta(ctx, ws); ok && meta != nil {
			return meta.AllowGrep && meta.Snapshot
		}
		return false
	}
	return true
}

// mcpRootMeta resolves MCP resource metadata for the provided MCP root.
func (s *Service) mcpRootMeta(ctx context.Context, location string) (*mcpcfg.ResourceRoot, bool) {
	if s == nil || s.mcpMgr == nil {
		return nil, false
	}
	server, _ := mcpuri.Parse(location)
	if strings.TrimSpace(server) == "" {
		return nil, false
	}
	opts, err := s.mcpMgr.Options(ctx, server)
	if err != nil || opts == nil {
		return nil, false
	}
	roots := mcpcfg.ResourceRoots(opts.Metadata)
	if len(roots) == 0 {
		return nil, false
	}
	normLoc := strings.TrimRight(strings.TrimSpace(location), "/")
	if mcpuri.Is(normLoc) {
		normLoc = normalizeMCPURI(normLoc)
	}
	for _, root := range roots {
		uri := strings.TrimRight(strings.TrimSpace(root.URI), "/")
		if uri == "" {
			continue
		}
		if mcpuri.Is(uri) {
			uri = normalizeMCPURI(uri)
		}
		if normLoc == uri || strings.HasPrefix(normLoc, uri+"/") {
			r := root
			return &r, true
		}
	}
	return nil, false
}

// mcpSnapshotResolver builds a snapshot resolver based on MCP metadata roots.
func (s *Service) mcpSnapshotResolver(ctx context.Context) mcpfs.SnapshotResolver {
	return func(location string) (snapshotURI, rootURI string, ok bool) {
		root, found := s.mcpRootMeta(ctx, location)
		if !found || root == nil || !root.Snapshot {
			return "", "", false
		}
		rootURI = strings.TrimRight(strings.TrimSpace(root.URI), "/")
		if rootURI == "" {
			return "", "", false
		}
		snapshotURI = strings.TrimSpace(root.SnapshotURI)
		if snapshotURI == "" {
			snapshotURI = rootURI + "/_snapshot.zip"
		}
		return snapshotURI, rootURI, true
	}
}

// mcpSnapshotManifestResolver reports whether snapshot MD5 manifests are enabled for a root.
func (s *Service) mcpSnapshotManifestResolver(ctx context.Context) mcpfs.SnapshotManifestResolver {
	return func(location string) bool {
		root, found := s.mcpRootMeta(ctx, location)
		if !found || root == nil || !root.Snapshot {
			return false
		}
		return root.SnapshotMD5
	}
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
	Include   []string `json:"include,omitempty" description:"optional file/path globs to include (matched against file path and base name); supports ** for any depth"`
	Exclude   []string `json:"exclude,omitempty" description:"optional file/path globs to exclude; supports ** for any depth"`

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

type grepSearchHashInput struct {
	Pattern         string   `json:"pattern,omitempty"`
	ExcludePattern  string   `json:"excludePattern,omitempty"`
	Root            string   `json:"root,omitempty"`
	RootID          string   `json:"rootId,omitempty"`
	Path            string   `json:"path,omitempty"`
	Recursive       bool     `json:"recursive,omitempty"`
	Include         []string `json:"include,omitempty"`
	Exclude         []string `json:"exclude,omitempty"`
	CaseInsensitive bool     `json:"caseInsensitive,omitempty"`
	Mode            string   `json:"mode,omitempty"`
	Bytes           int      `json:"bytes,omitempty"`
	Lines           int      `json:"lines,omitempty"`
	MaxFiles        int      `json:"maxFiles,omitempty"`
	MaxBlocks       int      `json:"maxBlocks,omitempty"`
	SkipBinary      bool     `json:"skipBinary,omitempty"`
	MaxSize         int      `json:"maxSize,omitempty"`
	Concurrency     int      `json:"concurrency,omitempty"`
}

func grepSearchHash(input *GrepInput, rootURI string) string {
	if input == nil {
		return ""
	}
	payload := grepSearchHashInput{
		Pattern:         strings.TrimSpace(input.Pattern),
		ExcludePattern:  strings.TrimSpace(input.ExcludePattern),
		Root:            strings.TrimSpace(rootURI),
		RootID:          strings.TrimSpace(input.RootID),
		Path:            strings.TrimSpace(input.Path),
		Recursive:       input.Recursive,
		Include:         append([]string(nil), input.Include...),
		Exclude:         append([]string(nil), input.Exclude...),
		CaseInsensitive: input.CaseInsensitive,
		Mode:            strings.TrimSpace(input.Mode),
		Bytes:           input.Bytes,
		Lines:           input.Lines,
		MaxFiles:        input.MaxFiles,
		MaxBlocks:       input.MaxBlocks,
		SkipBinary:      input.SkipBinary,
		MaxSize:         input.MaxSize,
		Concurrency:     input.Concurrency,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha1.Sum(raw)
	// Short but collision-resistant enough for UI row keys.
	return hex.EncodeToString(sum[:6])
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
	searchHash := grepSearchHash(input, rootURI)
	curAgent := s.currentAgent(ctx)
	allowed := s.agentAllowed(ctx)
	if mcpuri.Is(rootURI) {
		if wsRoot, _, err := s.normalizeUserRoot(ctx, rootURI); err == nil && strings.TrimSpace(wsRoot) != "" {
			if len(allowed) > 0 && !isAllowedWorkspace(wsRoot, allowed) {
				if rootMeta, ok := s.mcpRootMeta(ctx, wsRoot); ok && rootMeta != nil {
					allowed = append(allowed, wsRoot)
				}
			}
		}
	}
	rootCtx, err := s.newRootContext(ctx, rootURI, input.RootID, allowed)
	if err != nil {
		return err
	}
	wsRoot := rootCtx.Workspace()
	// Enforce per-resource grep capability when agent context is available.
	if !s.grepAllowedForAgent(ctx, curAgent, wsRoot) {
		return fmt.Errorf("grep not allowed for root: %s", rootURI)
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

	includes := normalizeListGlobs(input.Include)
	excludes := normalizeListGlobs(input.Exclude)

	stats := hygine.GrepStats{}
	var files []hygine.GrepFile
	totalBlocks := 0

	if mcpuri.Is(wsRoot) {
		return s.grepMCPFiles(ctx, input, output, rootCtx, searchHash, matchers, excludeMatchers, includes, excludes, mode, limitBytes, limitLines, maxFiles, maxBlocks, maxSize, skipBinary)
	}

	// Local/workspace handling
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

		scheme := url.Scheme(rootBase, "file")
		rootBaseNormalised := url.Normalize(rootBase, scheme)
		uriNormalised := url.Normalize(uri, scheme)

		rel := relativePath(rootBaseNormalised, uriNormalised)
		if rel == "" {
			baseNormalised := url.Normalize(base, scheme)
			rel = relativePath(baseNormalised, uriNormalised)
		}
		if rel == "" {
			rel = uri
		}
		name := pathpkg.Base(strings.TrimSuffix(rel, "/"))
		if !listMatchesFilters(rel, name, includes, excludes) {
			output.Stats = stats
			output.Files = nil
			return nil
		}

		stats.Scanned = 1
		stats.Matched = 1
		gf := hygine.GrepFile{Path: rel, URI: uri, Matches: len(matchLines)}
		gf.SearchHash = searchHash
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
			gf.RangeKey = fmt.Sprintf("%d-%d", 1, end)
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
			if gf.RangeKey == "" {
				gf.RangeKey = fmt.Sprintf("%d-%d", start+1, end)
			}
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
		scheme := url.Scheme(walkBaseURL, "file")
		baseNormalised := url.Normalize(base, scheme)
		rel := relativePath(baseNormalised, uri)
		if !listMatchesFilters(rel, info.Name(), includes, excludes) {
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
		gf.SearchHash = searchHash
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
			gf.RangeKey = fmt.Sprintf("%d-%d", 1, end)
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
			if gf.RangeKey == "" {
				gf.RangeKey = fmt.Sprintf("%d-%d", start+1, end)
			}
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

func (s *Service) grepMCPFiles(
	ctx context.Context,
	input *GrepInput,
	output *GrepOutput,
	rootCtx *rootContext,
	searchHash string,
	matchers []*regexp.Regexp,
	excludeMatchers []*regexp.Regexp,
	includes []string,
	excludes []string,
	mode string,
	limitBytes int,
	limitLines int,
	maxFiles int,
	maxBlocks int,
	maxSize int,
	skipBinary bool,
) error {
	if s == nil || s.mcpMgr == nil {
		return fmt.Errorf("mcp manager not configured")
	}
	wsRoot := rootCtx.Workspace()
	rootMeta, ok := s.mcpRootMeta(ctx, wsRoot)
	if !ok || rootMeta == nil || !rootMeta.Snapshot {
		return fmt.Errorf("grep requires snapshot support for root: %s", wsRoot)
	}
	if !rootMeta.AllowGrep {
		return fmt.Errorf("grep not allowed for root: %s", wsRoot)
	}
	resolver := s.mcpSnapshotResolver(ctx)
	snapURI, rootURI, ok := resolver(wsRoot)
	if !ok {
		return fmt.Errorf("grep requires snapshot support for root: %s", wsRoot)
	}
	mfs, err := s.mcpFS(ctx)
	if err != nil {
		return err
	}
	data, err := mfs.Download(ctx, mcpfs.NewObjectFromURI(snapURI))
	if err != nil {
		return err
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	stripPrefix := detectZipStripPrefix(reader)
	server, rootPath := mcpuri.Parse(rootURI)
	rootPath = strings.TrimRight(rootPath, "/")
	base := rootCtx.Base()
	baseServer, basePath := mcpuri.Parse(base)
	if strings.TrimSpace(baseServer) == "" {
		basePath = rootPath
	} else {
		basePath = strings.TrimRight(basePath, "/")
	}

	stats := hygine.GrepStats{}
	var files []hygine.GrepFile
	totalBlocks := 0

	for _, f := range reader.File {
		if f == nil || f.FileInfo().IsDir() {
			continue
		}
		rel := strings.TrimPrefix(f.Name, stripPrefix)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			continue
		}
		fullPath := mcpuri.JoinResourcePath(rootPath, rel)
		if basePath != "" && basePath != rootPath {
			if fullPath != basePath && !strings.HasPrefix(fullPath, basePath+"/") {
				continue
			}
		}
		uri := mcpuri.Canonical(server, fullPath)
		relPath := relativePath(base, uri)
		if relPath == "" {
			relPath = relativePath(wsRoot, uri)
		}
		if relPath == "" {
			relPath = rel
		}
		name := pathpkg.Base(strings.TrimSuffix(relPath, "/"))
		if !listMatchesFilters(relPath, name, includes, excludes) {
			continue
		}
		stats.Scanned++
		if stats.Matched >= maxFiles || totalBlocks >= maxBlocks {
			stats.Truncated = true
			break
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		fileData, err := readZipFile(rc, maxSize)
		_ = rc.Close()
		if err != nil {
			return err
		}
		if skipBinary && isBinary(fileData) {
			continue
		}
		text := string(fileData)
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
		gf := hygine.GrepFile{Path: relPath, URI: uri, Matches: len(matchLines)}
		gf.SearchHash = searchHash
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
			gf.RangeKey = fmt.Sprintf("%d-%d", 1, end)
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
			if gf.RangeKey == "" {
				gf.RangeKey = fmt.Sprintf("%d-%d", start+1, end)
			}
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
	output.Stats = stats
	output.Files = files
	return nil
}

func readZipFile(rc io.Reader, maxSize int) ([]byte, error) {
	if maxSize > 0 {
		rc = io.LimitReader(rc, int64(maxSize))
	}
	return io.ReadAll(rc)
}

func detectZipStripPrefix(reader *zip.Reader) string {
	if reader == nil {
		return ""
	}
	common := ""
	for _, f := range reader.File {
		if f == nil || f.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimPrefix(f.Name, "/")
		if name == "" {
			continue
		}
		parts := strings.SplitN(name, "/", 2)
		if len(parts) < 2 || parts[0] == "" {
			return ""
		}
		if common == "" {
			common = parts[0]
			continue
		}
		if common != parts[0] {
			return ""
		}
	}
	if common == "" {
		return ""
	}
	return common + "/"
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
			literal := regexp.QuoteMeta(strings.TrimSpace(p))
			if caseInsensitive {
				literal = "(?i)" + literal
			}
			re, err = regexp.Compile(literal)
			if err != nil {
				return nil, fmt.Errorf("invalid pattern %q: %w", p, err)
			}
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
