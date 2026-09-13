// Package windowpreview hosts native Forge windows with shared MCP datasources.
package windowpreview

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/viant/afs"
	uistyle "github.com/viant/agently-core/service/ui/style"
	"github.com/viant/agently-core/service/ui/tablepreference"
	"github.com/viant/agently/preview/datasource"
	"github.com/viant/agently/preview/mcp/mock"
	"github.com/viant/forge/backend/service/meta"
	"github.com/viant/forge/backend/types"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Root, Assets, Addr, MetadataRoot string
	WorkspaceRoot                    string
	MCPURL                           string
	PlatformMCPURL                   string
	StewardMCPURL                    string
	DisableAuthorization             bool
	ReadOnly                         bool
}
type Workspace struct {
	Title         string                         `yaml:"title" json:"title"`
	DefaultWindow string                         `yaml:"defaultWindow" json:"defaultWindow"`
	Windows       map[string]Window              `yaml:"windows" json:"windows"`
	Endpoints     map[string]datasource.Endpoint `yaml:"endpoints" json:"endpoints"`
	DataSources   map[string]Source              `yaml:"dataSources" json:"-"`
}
type Window struct {
	Title                 string                 `yaml:"title" json:"title"`
	File                  string                 `yaml:"file" json:"-"`
	DataSources           []string               `yaml:"dataSources" json:"-"`
	AuthorizationSnapshot map[string]interface{} `yaml:"authorizationSnapshot" json:"-"`
	WindowFormDefaults    map[string]string      `yaml:"windowFormDefaults" json:"-"`
}

// Backend follows agently-core's mcp_tool datasource declaration.
type Backend struct {
	Kind    string         `yaml:"kind"`
	Service string         `yaml:"service"`
	Method  string         `yaml:"method"`
	Pinned  map[string]any `yaml:"pinned"`
}
type Source struct {
	ID               string `yaml:"id,omitempty"`
	types.DataSource `yaml:",inline"`
	Backend          Backend           `yaml:"backend"`
	Query            mock.Query        `yaml:"query"`
	FilterFields     map[string]string `yaml:"filterFields"`
}
type App struct {
	styles    *uistyle.Service
	config    Config
	workspace Workspace
	root      string
	mock      *mock.Server
	mu        sync.Mutex
	fetches   map[string]int
	routes    map[string]string
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func New(config Config) (*App, error) {
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	a := &App{config: config, root: root, fetches: map[string]int{}, routes: map[string]string{}}
	styleRoot := root
	if strings.TrimSpace(config.WorkspaceRoot) != "" {
		styleRoot, err = filepath.Abs(config.WorkspaceRoot)
		if err != nil {
			return nil, err
		}
		styleRoot, err = filepath.EvalSymlinks(styleRoot)
		if err != nil {
			return nil, err
		}
	}
	a.styles = uistyle.New(func() string { return styleRoot })
	a.config.MetadataRoot = root
	if strings.TrimSpace(config.MetadataRoot) != "" {
		metadataRoot, e := filepath.Abs(config.MetadataRoot)
		if e != nil {
			return nil, e
		}
		metadataRoot, e = filepath.EvalSymlinks(metadataRoot)
		if e != nil {
			return nil, e
		}
		a.config.MetadataRoot = metadataRoot
	}
	b, err := a.read("preview.yaml")
	if err != nil {
		return nil, err
	}
	if err = yaml.Unmarshal(b, &a.workspace); err != nil {
		return nil, err
	}
	if err = a.configureMCPEndpoints(); err != nil {
		return nil, err
	}
	if err = a.hydrateWorkspaceDataSources(); err != nil {
		return nil, err
	}
	if len(a.workspace.Windows) == 0 {
		return nil, fmt.Errorf("preview.yaml requires a windows catalog")
	}
	if _, ok := a.workspace.Windows[a.workspace.DefaultWindow]; !ok {
		return nil, fmt.Errorf("defaultWindow must identify a catalog window")
	}
	for id := range a.workspace.Windows {
		if !idPattern.MatchString(id) {
			return nil, fmt.Errorf("unsafe window ID %q", id)
		}
		if _, err = a.LoadWindow(context.Background(), id); err != nil {
			return nil, err
		}
	}
	for id, s := range a.workspace.DataSources {
		if !idPattern.MatchString(id) || s.Backend.Kind != "mcp_tool" {
			return nil, fmt.Errorf("datasource %s requires an mcp_tool backend", id)
		}
		if _, _, err = datasource.Resolve(&types.Service{Endpoint: s.Backend.Service, URI: s.Backend.Method}, a.workspace.Endpoints, "http://127.0.0.1"); err != nil {
			return nil, err
		}
	}
	a.mock, err = mock.New(mock.Config{Root: root})
	if err != nil {
		return nil, err
	}
	return a, nil
}

// hydrateWorkspaceDataSources keeps the authored Forge datasource contract
// (parameters, selectors, cache policy, and autoFetch) while routing every
// request to the stable synthetic MCP tool named after its datasource ID.
func (a *App) hydrateWorkspaceDataSources() error {
	if filepath.Clean(a.config.MetadataRoot) == filepath.Clean(a.root) {
		return nil
	}
	definitions := map[string]Source{}
	root := filepath.Join(a.config.MetadataRoot, "datasources")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(strings.TrimSpace(string(data)), "$import(") {
			return nil
		}
		var source Source
		if err = yaml.Unmarshal(data, &source); err != nil {
			return fmt.Errorf("parse datasource %s: %w", path, err)
		}
		if source.ID != "" {
			definitions[source.ID] = source
		}
		return nil
	})
	if err != nil {
		return err
	}
	for id := range a.workspace.DataSources {
		override := a.workspace.DataSources[id]
		source, ok := definitions[id]
		if !ok {
			return fmt.Errorf("preview datasource %s is absent from workspace metadata", id)
		}
		service := strings.ToLower(strings.TrimSpace(source.Backend.Service))
		if service != "platform" && service != "steward" {
			// Draft-only sources have no live MCP owner. In preview they use the
			// synthetic Steward endpoint and are never allowed to reach production.
			service = "steward"
		}
		source.Backend = Backend{Kind: "mcp_tool", Service: service, Method: id}
		if override.AutoFetch != nil {
			source.AutoFetch = override.AutoFetch
		}
		// Preview fixtures may explicitly declare client-side filtering/paging
		// because their complete synthetic collection is available locally.
		if override.FilterMode != "" {
			source.FilterMode = override.FilterMode
		}
		if override.PaginationMode != "" {
			source.PaginationMode = override.PaginationMode
		}
		if override.Paging != nil {
			source.Paging = override.Paging
		}
		if len(override.FilterSet) > 0 {
			source.FilterSet = override.FilterSet
		}
		if override.QuickFilterSet != nil {
			source.QuickFilterSet = override.QuickFilterSet
		}
		a.workspace.DataSources[id] = source
	}
	return nil
}

func (a *App) configureMCPEndpoints() error {
	shared := strings.TrimRight(strings.TrimSpace(a.config.MCPURL), "/")
	platform := strings.TrimRight(strings.TrimSpace(a.config.PlatformMCPURL), "/")
	steward := strings.TrimRight(strings.TrimSpace(a.config.StewardMCPURL), "/")
	if platform == "" {
		platform = shared
	}
	if steward == "" {
		steward = shared
	}
	for name, endpoint := range map[string]string{"platform": platform, "steward": steward} {
		if endpoint == "" {
			continue
		}
		if !strings.HasPrefix(endpoint, "http://127.0.0.1:") && !strings.HasPrefix(endpoint, "http://localhost:") {
			return fmt.Errorf("%s MCP URL must be loopback HTTP", name)
		}
		a.workspace.Endpoints[name] = datasource.Endpoint{Type: "mcp", Transport: "streamable", BaseURL: endpoint}
	}
	return nil
}
func (a *App) read(relative string) ([]byte, error) {
	if filepath.IsAbs(relative) {
		return nil, fmt.Errorf("absolute paths are not allowed")
	}
	p, e := filepath.EvalSymlinks(filepath.Join(a.root, relative))
	if e != nil {
		return nil, e
	}
	rel, e := filepath.Rel(a.root, p)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path escapes workspace")
	}
	f, e := os.Open(p)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 2<<20+1))
	if len(b) > 2<<20 {
		return nil, fmt.Errorf("YAML exceeds 2 MB")
	}
	return b, e
}
func (a *App) LoadWindow(ctx context.Context, id string) (*types.Window, error) {
	return a.LoadWindowWithTarget(ctx, id, &meta.TargetContext{Platform: "web", FormFactor: "desktop", Surface: "app"})
}

// LoadWindowWithTarget resolves the same folderized window branch used by
// production before injecting the preview's synthetic datasources.
func (a *App) LoadWindowWithTarget(ctx context.Context, id string, target *meta.TargetContext) (*types.Window, error) {
	item, ok := a.workspace.Windows[id]
	if !ok {
		return nil, fmt.Errorf("unknown window %q", id)
	}
	windowFile := item.File
	loader := meta.New(afs.New(), "")
	base := filepath.Join(a.config.MetadataRoot, strings.TrimSuffix(item.File, filepath.Ext(item.File)), "main")
	if resolved, e := loader.ResolveWindowBase(ctx, base, target); e == nil {
		resolvedFile := resolved + ".yaml"
		if relative, relErr := filepath.Rel(a.config.MetadataRoot, resolvedFile); relErr == nil {
			windowFile = relative
		}
	}
	if _, e := a.readMetadata(windowFile); e != nil {
		return nil, e
	}
	// Preflight import paths before using the standard Forge metadata loader.
	if e := a.checkImports(windowFile, map[string]bool{}); e != nil {
		return nil, e
	}
	w := &types.Window{}
	if e := loader.LoadWithURLAndTarget(ctx, filepath.Join(a.config.MetadataRoot, windowFile), w, target); e != nil {
		return nil, e
	}
	if e := a.mergeActionRefs(w); e != nil {
		return nil, e
	}
	// Production Agently supplies workspace-level Forge datasources alongside
	// window metadata. Inject only the declared dependency set for this window:
	// opening Advertiser must not initialize unrelated workspace contexts.
	if w.DataSource == nil {
		w.DataSource = map[string]types.DataSource{}
	}
	for _, ref := range item.DataSources {
		if _, exists := w.DataSource[ref]; exists {
			continue
		}
		shared, exists := a.workspace.DataSources[ref]
		if !exists {
			return nil, fmt.Errorf("window %s references unknown shared datasource %s", id, ref)
		}
		w.DataSource[ref] = shared.DataSource
	}
	for local, ds := range w.DataSource {
		ref := ds.DataSourceRef
		if ref == "" {
			ref = local
		}
		shared, ok := a.workspace.DataSources[ref]
		if !ok {
			return nil, fmt.Errorf("window %s datasource %s references unknown shared datasource %s", id, local, ref)
		}
		base, _ := json.Marshal(shared.DataSource)
		override, _ := json.Marshal(ds)
		var merged, over map[string]any
		_ = json.Unmarshal(base, &merged)
		_ = json.Unmarshal(override, &over)
		for k, v := range over {
			if v != nil && v != "" {
				merged[k] = v
			}
		}
		// Keep browser calls on the preview bridge. The bridge uses MCPURL, when
		// configured, so a stable MCP service can outlive the window preview.
		merged["service"] = map[string]any{"endpoint": "preview", "uri": "/v1/api/datasources/" + ref + "/fetch", "method": "POST"}
		delete(merged, "dataSourceRef")
		raw, _ := json.Marshal(merged)
		ds = types.DataSource{}
		if e := json.Unmarshal(raw, &ds); e != nil {
			return nil, e
		}
		w.DataSource[local] = ds
	}
	if a.config.DisableAuthorization {
		w.Authorization = nil
	}
	if a.config.ReadOnly {
		// Suppress window mutations while retaining datasource lifecycle hooks:
		// these also resolve labels and prepare presentation data. Datasource
		// requests remain on the local synthetic MCP bridge.
		w.On = nil
		w.Dialogs = nil
		w.Schemas = nil
		w.ResourceModels = nil
		if item.AuthorizationSnapshot != nil {
			w.AuthorizationSnapshot = item.AuthorizationSnapshot
		}
		// Hosted chat regions are rendered by Agently's conversation shell. The
		// standalone preview has only a tab manager, so render this same window
		// in a regular tab without changing the authored workspace metadata.
		w.Presentation = ""
		w.Region = ""
		seedPreviewWindowForm(w, item.WindowFormDefaults)
		for ref, source := range w.DataSource {
			source.ResourceModelRef = ""
			w.DataSource[ref] = source
		}
	}
	if e := types.ValidateResourceModels(w); e != nil {
		return nil, e
	}
	w.WindowKey = id
	return w, nil
}

func seedPreviewWindowForm(window *types.Window, defaults map[string]string) {
	if window == nil || window.Window == nil {
		return
	}
	for _, execute := range window.Window.On {
		if execute == nil || execute.Handler != "dataSource.setWindowFormData" {
			continue
		}
		for _, parameter := range execute.Parameters {
			if parameter == nil || parameter.In != "const" || parameter.Location != "" {
				continue
			}
			if value, ok := defaults[parameter.Name]; ok {
				parameter.Location = value
			}
		}
	}
}

// mergeActionRefs mirrors Agently's workspace window loader: actionRefs are
// authored beside window metadata and must be compiled into Actions.Code before
// Forge can resolve namespace callbacks during rendering.
func (a *App) mergeActionRefs(window *types.Window) error {
	if window == nil || len(window.ActionRefs) == 0 {
		return nil
	}
	code := []string{}
	if window.Actions != nil && strings.TrimSpace(window.Actions.Code) != "" {
		code = append(code, strings.TrimSpace(window.Actions.Code))
	}
	seen := map[string]bool{}
	for _, ref := range window.ActionRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		source, err := a.readMetadata(filepath.Join("windows", ref+".js"))
		if err != nil {
			return fmt.Errorf("load window action ref %q: %w", ref, err)
		}
		code = append(code, strings.TrimSpace(string(source)))
	}
	if len(code) == 0 {
		return nil
	}
	window.SetCode([]byte("(() => Object.assign({},\n" + strings.Join(code, ",\n") + "\n))()"))
	return nil
}
func (a *App) readMetadata(relative string) ([]byte, error) {
	if filepath.IsAbs(relative) {
		return nil, fmt.Errorf("absolute paths are not allowed")
	}
	p, e := filepath.EvalSymlinks(filepath.Join(a.config.MetadataRoot, relative))
	if e != nil {
		return nil, e
	}
	rel, e := filepath.Rel(a.config.MetadataRoot, p)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("metadata path escapes workspace")
	}
	f, e := os.Open(p)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 2<<20+1))
	if len(b) > 2<<20 {
		return nil, fmt.Errorf("YAML exceeds 2 MB")
	}
	return b, e
}
func (a *App) checkImports(file string, seen map[string]bool) error {
	if seen[file] {
		return fmt.Errorf("cyclic YAML import")
	}
	seen[file] = true
	defer delete(seen, file)
	b, e := a.readMetadata(file)
	if e != nil {
		return e
	}
	var node yaml.Node
	if e = yaml.Unmarshal(b, &node); e != nil {
		return e
	}
	var walk func(*yaml.Node) error
	walk = func(n *yaml.Node) error {
		if n.Kind == yaml.AliasNode {
			return fmt.Errorf("YAML aliases are unsupported in preview packages")
		}
		if n.Tag != "" && !strings.HasPrefix(n.Tag, "!!") {
			return fmt.Errorf("custom YAML tags are unsupported")
		}
		for i, c := range n.Content {
			if n.Kind == yaml.MappingNode && i%2 == 0 && c.Value == "$import" {
				v := n.Content[i+1]
				values := []*yaml.Node{v}
				if v.Kind == yaml.SequenceNode {
					values = v.Content
				}
				for _, entry := range values {
					if entry.Kind != yaml.ScalarNode || strings.Contains(entry.Value, ":") {
						return fmt.Errorf("preview imports must be relative local YAML paths")
					}
					ref := filepath.Clean(filepath.Join(filepath.Dir(file), entry.Value))
					if e := a.checkImports(ref, seen); e != nil {
						return e
					}
				}
			}
			if e := walk(c); e != nil {
				return e
			}
		}
		return nil
	}
	return walk(&node)
}
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	a.styles.Register(mux)
	mux.Handle("/mcp", a.mock.HTTPHandler())
	mux.HandleFunc("/api/workspace", func(w http.ResponseWriter, r *http.Request) {
		publication := a.styles.Current(r.Context())
		root := a.config.WorkspaceRoot
		if root == "" {
			root = a.root
		}
		preferences, err := tablepreference.LoadConfig(root)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, struct {
			Workspace
			TablePreferences   tablepreference.AdapterConfig `json:"tablePreferences"`
			WorkspaceID        string                        `json:"workspaceId,omitempty"`
			UIStyles           *uistyle.Descriptor           `json:"uiStyles,omitempty"`
			UIThemes           *uistyle.Descriptor           `json:"uiThemes,omitempty"`
			UIStyleDiagnostics []string                      `json:"uiStyleDiagnostics,omitempty"`
		}{
			Workspace:          a.workspace,
			TablePreferences:   preferences.TablePreferences,
			WorkspaceID:        publication.WorkspaceID,
			UIStyles:           publication.Styles,
			UIThemes:           publication.Themes,
			UIStyleDiagnostics: publication.Diagnostics,
		})
	})
	mux.HandleFunc("/api/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		fetches := make(map[string]int, len(a.fetches))
		for id, count := range a.fetches {
			fetches[id] = count
		}
		routes := make(map[string]string, len(a.routes))
		for id, service := range a.routes {
			routes[id] = service
		}
		a.mu.Unlock()
		writeJSON(w, map[string]any{"fetches": fetches, "routes": routes})
	})
	mux.HandleFunc("/api/windows/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/windows/")
		query := r.URL.Query()
		target := &meta.TargetContext{
			Platform:     strings.TrimSpace(query.Get("platform")),
			FormFactor:   strings.TrimSpace(query.Get("formFactor")),
			Surface:      strings.TrimSpace(query.Get("surface")),
			Capabilities: query["capabilities"],
		}
		if target.Platform == "" && target.FormFactor == "" && target.Surface == "" && len(target.Capabilities) == 0 {
			target = &meta.TargetContext{Platform: "web", FormFactor: "desktop", Surface: "app"}
		}
		win, e := a.LoadWindowWithTarget(r.Context(), id, target)
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, map[string]any{"status": "ok", "data": win})
	})
	mux.HandleFunc("/v1/api/datasources/", a.fetch)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, filepath.Join(a.config.Assets, "window-preview.html"))
			return
		}
		http.FileServer(http.Dir(a.config.Assets)).ServeHTTP(w, r)
	})
	return mock.LocalOnly(mux)
}
func (a *App) fetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/api/datasources/")
	id, ok := strings.CutSuffix(path, "/fetch")
	s, found := a.workspace.DataSources[id]
	if !ok || !found {
		http.NotFound(w, r)
		return
	}
	a.mu.Lock()
	a.fetches[id]++
	a.routes[id] = s.Backend.Service
	a.mu.Unlock()
	var request struct {
		Inputs       map[string]any `json:"inputs"`
		Variant      string         `json:"variant"`
		Cache        any            `json:"cache"`
		InvocationID string         `json:"invocationId"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if e := decoder.Decode(&request); e != nil {
		writeError(w, e)
		return
	}
	inputs := request.Inputs
	if inputs == nil {
		inputs = map[string]any{}
	}
	for k, v := range s.Backend.Pinned {
		inputs[k] = v
	}
	raw, _ := json.Marshal(s.Query)
	var q mock.Query
	_ = json.Unmarshal(raw, &q)
	if value, ok := inputs["query"]; ok {
		b, _ := json.Marshal(value)
		d := json.NewDecoder(strings.NewReader(string(b)))
		d.DisallowUnknownFields()
		if e := d.Decode(&q); e != nil {
			writeError(w, e)
			return
		}
	}
	keys := []string{}
	for key := range s.FilterFields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value, ok := inputs[key]
		if !ok || value == nil || value == "" {
			continue
		}
		op := "eq"
		if _, ok := value.([]any); ok {
			op = "in"
		}
		leaf := mock.Predicate{Field: s.FilterFields[key], Op: op, Value: value}
		if q.Filter == nil {
			q.Filter = &leaf
		} else {
			q.Filter = &mock.Predicate{And: []mock.Predicate{*q.Filter, leaf}}
		}
	}
	if size, ok := inputs["size"].(float64); ok {
		if size < 1 || size != float64(int(size)) {
			writeError(w, fmt.Errorf("invalid size"))
			return
		}
		offset := 0
		if v, ok := inputs["offset"].(float64); ok {
			offset = int(v)
			if v < 0 || v != float64(offset) {
				writeError(w, fmt.Errorf("invalid offset"))
				return
			}
		}
		q.Page = &mock.Page{Limit: int(size), Offset: offset}
	}
	endpoint, tool, e := datasource.Resolve(&types.Service{Endpoint: s.Backend.Service, URI: s.Backend.Method}, a.workspace.Endpoints, "http://"+r.Host)
	if e != nil {
		writeError(w, e)
		return
	}
	mcpContext, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, e := datasource.Dial(mcpContext, endpoint)
	if e != nil {
		writeError(w, e)
		return
	}
	defer client.Close()
	body, e := client.Call(mcpContext, tool, map[string]any{"query": q, "variant": request.Variant})
	if e != nil {
		writeError(w, e)
		return
	}
	var result any
	_ = json.Unmarshal(body, &result)
	obj, _ := result.(map[string]any)
	if obj["status"] == "error" {
		writeError(w, fmt.Errorf("datasource %s: %v", id, obj["message"]))
		return
	}
	rows := result
	if obj != nil {
		rows = obj["rows"]
	}
	if rows == nil {
		writeError(w, fmt.Errorf("datasource %s must return record rows", id))
		return
	}
	// Preserve the public MCP envelope alongside the normalized rows. Forge
	// datasource selectors are authored per source (for example, Advertiser
	// Properties selects `data`, while tables commonly select `rows`).
	payload := map[string]any{"rows": rows, "dataInfo": map[string]any{"hasMore": obj["hasMore"]}}
	for key, value := range obj {
		if key != "status" {
			payload[key] = value
		}
	}
	if s.Selectors != nil {
		ensureSelectorValue(payload, s.Selectors.Data, rows)
		if recordRows, ok := rows.([]any); ok && len(recordRows) > 0 {
			ensureSelectorValue(payload, s.Selectors.Metrics, recordRows[0])
		}
	}
	writeJSON(w, payload)
}

// ensureSelectorValue supplies a synthetic value at an authored selector only
// when the fixture does not already provide one. This supports nested source
// contracts such as data.0.acl without copying production responses.
func ensureSelectorValue(root map[string]any, selector string, value any) {
	parts := strings.Split(strings.TrimSpace(selector), ".")
	if len(parts) == 0 || parts[0] == "" {
		return
	}
	var current any = root
	for index, part := range parts {
		last := index == len(parts)-1
		nextIsIndex := !last && isSelectorIndex(parts[index+1])
		switch node := current.(type) {
		case map[string]any:
			if last {
				if _, exists := node[part]; !exists {
					node[part] = value
				}
				return
			}
			next, exists := node[part]
			if !exists || next == nil {
				if nextIsIndex {
					position, _ := strconv.Atoi(parts[index+1])
					next = make([]any, position+1)
				} else {
					next = map[string]any{}
				}
				node[part] = next
			}
			current = next
		case []any:
			position, err := strconv.Atoi(part)
			if err != nil || position < 0 {
				return
			}
			for len(node) <= position {
				node = append(node, nil)
			}
			if last {
				if node[position] == nil {
					node[position] = value
				}
				return
			}
			if node[position] == nil {
				if nextIsIndex {
					node[position] = []any{}
				} else {
					node[position] = map[string]any{}
				}
			}
			current = node[position]
		default:
			return
		}
	}
}

func isSelectorIndex(value string) bool {
	_, err := strconv.Atoi(value)
	return err == nil
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, e error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(422)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": "windowPreviewError", "message": e.Error()})
}
func Serve(ctx context.Context, c Config, out io.Writer) error {
	if c.Addr == "" {
		c.Addr = "127.0.0.1:8098"
	}
	if c.Assets == "" {
		c.Assets = "preview/ui/dist"
	}
	if _, e := os.Stat(filepath.Join(c.Assets, "window-preview.html")); e != nil {
		return fmt.Errorf("build window preview assets first: npm --prefix ui run build:window-preview")
	}
	host, _, e := net.SplitHostPort(c.Addr)
	if e != nil {
		return e
	}
	if host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback()) {
		return fmt.Errorf("loopback address required")
	}
	a, e := New(c)
	if e != nil {
		return e
	}
	listener, e := net.Listen("tcp", c.Addr)
	if e != nil {
		return e
	}
	server := &http.Server{Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = server.Close()
		case <-done:
		}
	}()
	fmt.Fprintf(out, "Forge window preview: http://%s/?window=%s\n", listener.Addr(), a.workspace.DefaultWindow)
	e = server.Serve(listener)
	if e == http.ErrServerClosed {
		return nil
	}
	return e
}
