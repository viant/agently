package workspace

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/viant/afs"
	afsurl "github.com/viant/afs/url"
	agentmdl "github.com/viant/agently/genai/agent"
	"strconv"
)

// AgentEditView provides authoring-oriented metadata for the Agents UI.
// It is read-only and computed on demand; the stored YAML remains unchanged.
type AgentEditView struct {
	Agent *agentmdl.Agent     `json:"agent"`
	Meta  *AgentAuthoringMeta `json:"meta"`
}

type AgentAuthoringMeta struct {
	Source        SourceMeta         `json:"source"`
	Prompts       PromptMeta         `json:"prompts"`
	Knowledge     KnowledgeMeta      `json:"knowledge"`
	Capabilities  CapabilitiesMeta   `json:"capabilities"`
	ContextInputs *ContextInputsMeta `json:"contextInputs,omitempty"`
	A2A           *A2AMeta           `json:"a2a,omitempty"`
	Assets        []AssetMeta        `json:"assets,omitempty"`
}

type SourceMeta struct {
	URL     string `json:"url"`
	BaseDir string `json:"baseDir"`
}

type PromptMeta struct {
	User   PathMeta `json:"user"`
	System PathMeta `json:"system"`
}

type PathMeta struct {
	Display  string `json:"display"`
	Resolved string `json:"resolved"`
	Exists   bool   `json:"exists"`
	// Content preview (best-effort), truncated when large.
	Content     string `json:"content,omitempty"`
	ContentSize int    `json:"contentSize,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
}

type KnowledgeMeta struct {
	Roots []KnowledgeRoot `json:"roots"`
}

type KnowledgeRoot struct {
	Scope     string `json:"scope"` // user|system
	Index     int    `json:"index"`
	Display   string `json:"display"`
	Resolved  string `json:"resolved"`
	Exists    bool   `json:"exists"`
	Browsable bool   `json:"browsable"`
}

type CapabilitiesMeta struct {
	SupportsChains            bool `json:"supportsChains"`
	SupportsParallelToolCalls bool `json:"supportsParallelToolCalls"`
	SupportsAttachments       bool `json:"supportsAttachments"`
}

// AssetMeta represents a non‑YAML asset under the agent folder.
type AssetMeta struct {
	Path string `json:"path"`
	Type string `json:"type"` // file|dir
	Size int64  `json:"size,omitempty"`
}

// ContextInputsMeta surfaces authored elicitation (aux inputs) with
// a derived list of fields and basic validation hints.
type ContextInputsMeta struct {
	Enabled         bool                   `json:"enabled"`
	Message         string                 `json:"message,omitempty"`
	RequestedSchema map[string]interface{} `json:"requestedSchema,omitempty"`
	Fields          []InputFieldMeta       `json:"fields,omitempty"`
	Validation      InputValidationMeta    `json:"validation"`
}

type InputFieldMeta struct {
	Name        string      `json:"name"`
	Type        string      `json:"type,omitempty"`
	Description string      `json:"description,omitempty"`
	Default     interface{} `json:"default,omitempty"`
	Required    bool        `json:"required"`
}

type InputValidationMeta struct {
	SchemaValid     bool     `json:"schemaValid"`
	MissingRequired []string `json:"missingRequired,omitempty"`
}

// A2AMeta summarizes agent A2A exposure configuration for UI status.
type A2AMeta struct {
	Configured bool         `json:"configured"`
	Enabled    bool         `json:"enabled"`
	Port       int          `json:"port,omitempty"`
	Streaming  bool         `json:"streaming"`
	Addr       string       `json:"addr,omitempty"` // e.g. localhost:8091
	URLs       *A2AURLs     `json:"urls,omitempty"` // common entry points
	Auth       *A2AAuthMeta `json:"auth,omitempty"`
	State      string       `json:"state,omitempty"` // unknown|configured
}

type A2AURLs struct {
	Base      string `json:"base,omitempty"`      // http://localhost:port/
	AgentCard string `json:"agentCard,omitempty"` // /.well-known/agent-card.json
	SSEBase   string `json:"sseBase,omitempty"`   // /v1
	Streaming string `json:"streaming,omitempty"` // /a2a
}

type A2AAuthMeta struct {
	Enabled       bool     `json:"enabled"`
	Resource      string   `json:"resource,omitempty"`
	Scopes        []string `json:"scopes,omitempty"`
	UseIDToken    bool     `json:"useIDToken,omitempty"`
	ExcludePrefix string   `json:"excludePrefix,omitempty"`
}

// buildAgentEditView constructs an AgentEditView from an authored agent and its repository filename.
func buildAgentEditView(ctx context.Context, ag *agentmdl.Agent, repoFilename string) *AgentEditView {
	fs := afs.New()
	baseDir := filepath.Dir(repoFilename)
	baseDirAbs, _ := filepath.Abs(baseDir)

	// Helper to resolve authored paths relative to baseDir when no scheme present.
	resolve := func(authored string) (display, resolved string, exists bool) {
		authored = strings.TrimSpace(authored)
		if authored == "" {
			return "", "", false
		}
		// Default display is the authored form; adjust to relative when possible.
		display = authored
		// If path has a scheme, keep as-is; otherwise resolve relative to baseDir.
		if afsurl.Scheme(authored, "") == "" && !strings.HasPrefix(authored, string(filepath.Separator)) {
			// file path relative to baseDir
			resolved = filepath.Clean(filepath.Join(baseDir, authored))
		} else {
			resolved = authored
		}
		// Existence: rely on AFS to support both file:// and plain paths
		exists, _ = fs.Exists(ctx, resolved)

		// Improve display: if resolved is under baseDir, present path relative to baseDir
		// Works for both plain paths and file:// URIs.
		// Extract OS path from resolved when it has a scheme.
		resPath := resolved
		if sch := afsurl.Scheme(resolved, ""); sch != "" {
			resPath = afsurl.Path(resolved)
		}
		// Make both absolute to compute Rel safely
		resAbs, _ := filepath.Abs(resPath)
		if baseDirAbs != "" && resAbs != "" {
			if rel, err := filepath.Rel(baseDirAbs, resAbs); err == nil && rel != "" && !strings.HasPrefix(rel, "..") {
				// Normalize to forward slashes for UI consistency
				display = filepath.ToSlash(rel)
			}
		}
		return
	}

	// Helper to read small files best-effort (e.g., prompts)
	readPreview := func(resolved string, max int) (string, int, bool) {
		if strings.TrimSpace(resolved) == "" {
			return "", 0, false
		}
		url := resolved
		if afsurl.Scheme(url, "") == "" {
			url = "file://" + filepath.Clean(url)
		}
		data, err := fs.DownloadWithURL(ctx, url)
		if err != nil || len(data) == 0 {
			return "", 0, false
		}
		if max > 0 && len(data) > max {
			return string(data[:max]), len(data), true
		}
		return string(data), len(data), false
	}
	// Prompts meta
	var pUser, pSys PathMeta
	if ag != nil && ag.Prompt != nil {
		pUser.Display, pUser.Resolved, pUser.Exists = resolve(ag.Prompt.URI)
		if pUser.Exists && strings.TrimSpace(pUser.Resolved) != "" {
			pUser.Content, pUser.ContentSize, pUser.Truncated = readPreview(pUser.Resolved, 64*1024)
		}
	}
	if ag != nil && ag.SystemPrompt != nil {
		pSys.Display, pSys.Resolved, pSys.Exists = resolve(ag.SystemPrompt.URI)
		if pSys.Exists && strings.TrimSpace(pSys.Resolved) != "" {
			pSys.Content, pSys.ContentSize, pSys.Truncated = readPreview(pSys.Resolved, 64*1024)
		}
	}

	// Knowledge roots
	var roots []KnowledgeRoot
	for i, k := range ag.Knowledge {
		disp, res, ex := resolve(k.URL)
		scheme := afsurl.Scheme(res, "")
		browsable := scheme == "" || scheme == "file"
		roots = append(roots, KnowledgeRoot{Scope: "user", Index: i, Display: disp, Resolved: res, Exists: ex, Browsable: browsable})
	}
	for i, k := range ag.SystemKnowledge {
		disp, res, ex := resolve(k.URL)
		scheme := afsurl.Scheme(res, "")
		browsable := scheme == "" || scheme == "file"
		roots = append(roots, KnowledgeRoot{Scope: "system", Index: i, Display: disp, Resolved: res, Exists: ex, Browsable: browsable})
	}

	caps := CapabilitiesMeta{
		SupportsChains:            len(ag.Chains) > 0,
		SupportsParallelToolCalls: ag.ParallelToolCalls,
		SupportsAttachments:       ag.Attachment != nil,
	}

	// Collect non‑YAML assets under the agent base directory
	var assets []AssetMeta
	{
		files := make([]string, 0, 32)
		_ = listNonYAML(ctx, fs, baseDir, &files)
		// Track directories that contain at least one non‑YAML file
		dirSet := map[string]struct{}{}
		for _, p := range files {
			d := filepath.Dir(p)
			for d != "" && d != baseDir && d != "." && d != string(filepath.Separator) {
				dirSet[d] = struct{}{}
				nd := filepath.Dir(d)
				if nd == d {
					break
				}
				d = nd
			}
		}
		for d := range dirSet {
			if rel, err := filepath.Rel(baseDir, d); err == nil && rel != "." && rel != "" {
				assets = append(assets, AssetMeta{Path: filepath.ToSlash(rel), Type: "dir"})
			}
		}
		for _, p := range files {
			if rel, err := filepath.Rel(baseDir, p); err == nil && rel != "." && rel != "" {
				var sz int64
				if obj, err := fs.Object(ctx, p); err == nil && obj != nil {
					sz = obj.Size()
				}
				assets = append(assets, AssetMeta{Path: filepath.ToSlash(rel), Type: "file", Size: sz})
			}
		}
	}

	return &AgentEditView{
		Agent: ag,
		Meta: &AgentAuthoringMeta{
			Source:        SourceMeta{URL: repoFilename, BaseDir: baseDir},
			Prompts:       PromptMeta{User: pUser, System: pSys},
			Knowledge:     KnowledgeMeta{Roots: roots},
			Capabilities:  caps,
			ContextInputs: buildContextInputsMeta(ag),
			A2A:           buildA2AMeta(ag),
			Assets:        assets,
		},
	}
}

// listNonYAML recursively collects non‑YAML files under root path.
func listNonYAML(ctx context.Context, fs afs.Service, root string, out *[]string) error {
	entries, err := fs.List(ctx, root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		// Normalize to OS path for extension checks
		osPath := name
		if sch := afsurl.Scheme(name, ""); sch != "" {
			osPath = afsurl.Path(name)
		}
		if e.IsDir() {
			// recurse into subdir
			_ = listNonYAML(ctx, fs, name, out)
			continue
		}
		ext := strings.ToLower(filepath.Ext(osPath))
		if ext == ".yaml" || ext == ".yml" {
			continue
		}
		*out = append(*out, name)
	}
	return nil
}

// buildContextInputsMeta derives a UI-friendly view of agent.ContextInputs.
func buildContextInputsMeta(ag *agentmdl.Agent) *ContextInputsMeta {
	if ag == nil || ag.ContextInputs == nil {
		return nil
	}
	ci := ag.ContextInputs
	meta := &ContextInputsMeta{Enabled: ci.Enabled, Message: strings.TrimSpace(ci.Message)}
	// Convert schema to a plain map for the UI
	rs := ci.RequestedSchema
	req := map[string]interface{}{}
	if len(rs.Properties) > 0 || rs.Type != "" || len(rs.Required) > 0 {
		req["type"] = rs.Type
		if len(rs.Properties) > 0 {
			req["properties"] = rs.Properties
		} else {
			req["properties"] = map[string]interface{}{}
		}
		if len(rs.Required) > 0 {
			req["required"] = rs.Required
		}
	}
	meta.RequestedSchema = req

	// Derive fields from properties + required
	requiredSet := map[string]struct{}{}
	for _, k := range rs.Required {
		k = strings.TrimSpace(k)
		if k != "" {
			requiredSet[k] = struct{}{}
		}
	}
	var fields []InputFieldMeta
	var missing []string
	for name, raw := range rs.Properties {
		f := InputFieldMeta{Name: name}
		if m, ok := raw.(map[string]interface{}); ok {
			if t, ok := m["type"].(string); ok {
				f.Type = strings.TrimSpace(t)
			}
			if d, ok := m["description"].(string); ok {
				f.Description = d
			}
			if def, ok := m["default"]; ok {
				f.Default = def
			}
		}
		if _, ok := requiredSet[name]; ok {
			f.Required = true
			if f.Default == nil {
				missing = append(missing, name)
			}
		}
		fields = append(fields, f)
	}
	meta.Fields = fields
	// Basic validation: type object + has properties
	meta.Validation.SchemaValid = (strings.TrimSpace(rs.Type) == "object")
	meta.Validation.MissingRequired = missing
	return meta
}

// buildA2AMeta extracts serve.a2a (or legacy exposeA2A) into a compact summary.
func buildA2AMeta(ag *agentmdl.Agent) *A2AMeta {
	if ag == nil {
		return nil
	}
	var enabled bool
	var port int
	var streaming bool
	var auth *agentmdl.A2AAuth
	configured := false

	if ag.Serve != nil && ag.Serve.A2A != nil {
		configured = true
		enabled = ag.Serve.A2A.Enabled
		port = ag.Serve.A2A.Port
		streaming = ag.Serve.A2A.Streaming
		auth = ag.Serve.A2A.Auth
	} else if ag.ExposeA2A != nil {
		configured = true
		enabled = ag.ExposeA2A.Enabled
		port = ag.ExposeA2A.Port
		streaming = ag.ExposeA2A.Streaming
		auth = ag.ExposeA2A.Auth
	}
	if !configured {
		return nil
	}
	meta := &A2AMeta{Configured: true, Enabled: enabled, Port: port, Streaming: streaming, State: "unknown"}
	// Copy auth when present
	if auth != nil {
		meta.Auth = &A2AAuthMeta{Enabled: auth.Enabled, Resource: auth.Resource, Scopes: append([]string(nil), auth.Scopes...), UseIDToken: auth.UseIDToken, ExcludePrefix: auth.ExcludePrefix}
	}
	// Provide commonly used URLs using localhost for display. Actual external address may differ.
	if port > 0 {
		meta.Addr = "localhost:" + strconv.Itoa(port)
		base := "http://" + meta.Addr
		meta.URLs = &A2AURLs{Base: base + "/", AgentCard: base + "/.well-known/agent-card.json", SSEBase: "/v1", Streaming: "/a2a"}
	}
	return meta
}
