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
	"strings"
	"time"

	"github.com/viant/afs"
	"github.com/viant/agently/preview/datasource"
	"github.com/viant/agently/preview/mcp/mock"
	"github.com/viant/forge/backend/service/meta"
	"github.com/viant/forge/backend/types"
	"gopkg.in/yaml.v3"
)

type Config struct{ Root, Assets, Addr string }
type Workspace struct {
	Title         string                         `yaml:"title" json:"title"`
	DefaultWindow string                         `yaml:"defaultWindow" json:"defaultWindow"`
	Windows       map[string]Window              `yaml:"windows" json:"windows"`
	Endpoints     map[string]datasource.Endpoint `yaml:"endpoints" json:"endpoints"`
	DataSources   map[string]Source              `yaml:"dataSources" json:"-"`
}
type Window struct {
	Title string `yaml:"title" json:"title"`
	File  string `yaml:"file" json:"-"`
}

// Backend follows agently-core's mcp_tool datasource declaration.
type Backend struct {
	Kind    string         `yaml:"kind"`
	Service string         `yaml:"service"`
	Method  string         `yaml:"method"`
	Pinned  map[string]any `yaml:"pinned"`
}
type Source struct {
	types.DataSource `yaml:",inline"`
	Backend          Backend           `yaml:"backend"`
	Query            mock.Query        `yaml:"query"`
	FilterFields     map[string]string `yaml:"filterFields"`
}
type App struct {
	config    Config
	workspace Workspace
	root      string
	mock      *mock.Server
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
	a := &App{config: config, root: root}
	b, err := a.read("preview.yaml")
	if err != nil {
		return nil, err
	}
	if err = yaml.Unmarshal(b, &a.workspace); err != nil {
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
	item, ok := a.workspace.Windows[id]
	if !ok {
		return nil, fmt.Errorf("unknown window %q", id)
	}
	if _, e := a.read(item.File); e != nil {
		return nil, e
	}
	// Preflight import paths before using the standard Forge metadata loader.
	if e := a.checkImports(item.File, map[string]bool{}); e != nil {
		return nil, e
	}
	loader := meta.New(afs.New(), "")
	w := &types.Window{}
	if e := loader.LoadWithURLAndTarget(ctx, filepath.Join(a.root, item.File), w, &meta.TargetContext{Platform: "web", FormFactor: "desktop", Surface: "app"}); e != nil {
		return nil, e
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
		merged["service"] = map[string]any{"endpoint": "preview", "uri": "/v1/api/datasources/" + ref + "/fetch", "method": "POST"}
		delete(merged, "dataSourceRef")
		raw, _ := json.Marshal(merged)
		ds = types.DataSource{}
		if e := json.Unmarshal(raw, &ds); e != nil {
			return nil, e
		}
		w.DataSource[local] = ds
	}
	if e := types.ValidateResourceModels(w); e != nil {
		return nil, e
	}
	w.WindowKey = id
	return w, nil
}
func (a *App) checkImports(file string, seen map[string]bool) error {
	if seen[file] {
		return fmt.Errorf("cyclic YAML import")
	}
	seen[file] = true
	defer delete(seen, file)
	b, e := a.read(file)
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
	mux.Handle("/mcp", a.mock.HTTPHandler())
	mux.HandleFunc("/api/workspace", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.workspace) })
	mux.HandleFunc("/api/windows/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/windows/")
		win, e := a.LoadWindow(r.Context(), id)
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
	client, e := datasource.Dial(r.Context(), endpoint)
	if e != nil {
		writeError(w, e)
		return
	}
	defer client.Close()
	body, e := client.Call(r.Context(), tool, map[string]any{"query": q, "variant": request.Variant})
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
	writeJSON(w, map[string]any{"rows": rows, "dataInfo": map[string]any{"hasMore": obj["hasMore"]}, "metrics": obj["meta"]})
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
