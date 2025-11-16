package resources

import (
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
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
	Roots        []string        `json:"roots"`
	Path         string          `json:"path,omitempty"`
	Model        string          `json:"model"`
	MaxDocuments int             `json:"maxDocuments,omitempty"`
	IncludeFile  bool            `json:"includeFile,omitempty"`
	Match        *embopt.Options `json:"match,omitempty"`
}

// MatchOutput mirrors augmenter.AugmentDocsOutput for convenience.
type MatchOutput struct {
	aug.AugmentDocsOutput
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
	if len(input.Roots) == 0 {
		return fmt.Errorf("roots is required")
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		return fmt.Errorf("model is required")
	}
	// Enforce allowlist when agent context present and normalize roots.
	allowed := s.agentAllowed(ctx)
	locations := make([]string, 0, len(input.Roots))
	for _, root := range input.Roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		if len(allowed) > 0 && !s.isAllowed(root, allowed) {
			return fmt.Errorf("root not allowed: %s", root)
		}
		norm, _ := s.normalizeLocation(root)
		if norm == "" {
			continue
		}
		base := norm
		if strings.TrimSpace(input.Path) != "" {
			base = url.Join(norm, strings.TrimPrefix(strings.TrimSpace(input.Path), "/"))
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
		Model:        model,
		MaxDocuments: input.MaxDocuments,
		IncludeFile:  input.IncludeFile,
		TrimPath:     trimPrefix,
	}
	var augOut aug.AugmentDocsOutput
	if err := s.augmenter.AugmentDocs(ctx, augIn, &augOut); err != nil {
		return err
	}
	output.AugmentDocsOutput = augOut
	return nil
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
	allowed := s.agentAllowed(ctx)
	if len(allowed) > 0 && !s.isAllowed(rootURI, allowed) {
		return fmt.Errorf("root not allowed: %s", rootURI)
	}
	normRoot, _ := s.normalizeLocation(rootURI)
	if normRoot == "" {
		return fmt.Errorf("invalid root: %s", rootURI)
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
		if len(allowed) > 0 && !s.isAllowed(uri, allowed) {
			return fmt.Errorf("resource not allowed: %s", uri)
		}
		norm, _ := s.normalizeLocation(uri)
		if norm == "" {
			return fmt.Errorf("invalid uri: %s", uri)
		}
		fullURI = norm
	} else {
		// Root+path mode
		if rootURI == "" {
			return fmt.Errorf("root or uri is required")
		}
		if pathPart == "" {
			return fmt.Errorf("path is required when uri is not provided")
		}
		if len(allowed) > 0 && !s.isAllowed(rootURI, allowed) {
			return fmt.Errorf("root not allowed: %s", rootURI)
		}
		normRoot, _ = s.normalizeLocation(rootURI)
		if normRoot == "" {
			return fmt.Errorf("invalid root: %s", rootURI)
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
	u := strings.TrimSpace(loc)
	if u == "" {
		return false
	}
	// Normalize to best-effort canonical form
	norm, _ := s.normalizeLocation(u)
	if norm == "" {
		norm = u
	}
	for _, a := range allowed {
		if strings.HasPrefix(norm, a) {
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
	Label       string `json:"label"`
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
		norm, kind := s.normalizeLocation(loc)
		if norm == "" {
			continue
		}
		if seen[norm] {
			continue
		}
		seen[norm] = true
		label := s.defaultLabel(norm, kind)
		desc := s.tryDescribe(ctx, norm, kind)
		roots = append(roots, Root{URI: norm, Label: label, Description: desc, Kind: kind, Source: source})
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
	// file
	fs := afs.New()
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
		return nil
	}
	ag, err := s.aFinder.Find(ctx, agentID)
	if err != nil || ag == nil || len(ag.Resources) == 0 {
		return nil
	}
	out := make([]string, 0, len(ag.Resources))
	for _, e := range ag.Resources {
		if e == nil {
			continue
		}
		if u := strings.TrimSpace(e.URI); u != "" {
			norm, _ := s.normalizeLocation(u)
			if norm == "" {
				norm = u
			}
			out = append(out, norm)
		}
	}
	return out
}
