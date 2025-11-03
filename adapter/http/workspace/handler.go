package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/oauth2"
	toolext "github.com/viant/agently/genai/tool"
	"github.com/viant/agently/internal/workspace"
	a2arepo "github.com/viant/agently/internal/workspace/repository/a2a"
	"github.com/viant/agently/internal/workspace/repository/oauth"
	"github.com/viant/mcp"

	afsurl "github.com/viant/afs/url"
	"github.com/viant/agently/cmd/service"
	llmprovider "github.com/viant/agently/genai/llm/provider"
	"gopkg.in/yaml.v3"
)

// Handler exposes CRUD operations for any workspace repository kind
// (agents, models, workflows, mcp …).
//
// Routes (all under /v1/workspace/):
//
//	GET    /{kind}            -> JSON []string
//	GET    /{kind}/{name}     -> raw YAML/JSON (as stored)
//	PUT    /{kind}/{name}     -> create/overwrite
//	DELETE /{kind}/{name}     -> delete
//
// It is intentionally stateless; repositories are resolved via service.Service.
func NewHandler(svc *service.Service) http.Handler {
	return &handler{svc: svc}
}

type handler struct {
	svc *service.Service
}

// newInstance returns a zero value pointer for the Go struct that corresponds
// to the given workspace kind ("models", "agents" …). It returns nil when no
// mapping is defined – the caller should fall back to generic map handling.
func newInstance(kind string) interface{} {
	// Keep the registry very small; extend with additional kinds as needed.
	switch kind {
	case workspace.KindModel, "model":
		return &llmprovider.Config{}
	case workspace.KindMCP:
		return &mcp.ClientOptions{}
	case workspace.KindAgent:
		return &agent.Agent{}
	case workspace.KindFeeds:
		return &toolext.FeedSpec{}
	case workspace.KindA2A:
		return &a2arepo.A2AClientConfig{}
	default:
		return nil
	}
}

type apiResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func encode(w http.ResponseWriter, statusCode int, data interface{}, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		if statusCode == 0 {
			statusCode = http.StatusInternalServerError
		}
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(apiResponse{Status: "ERROR", Message: err.Error()})
		return
	}
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(apiResponse{Status: "ok", Data: data})
}

// ServeHTTP implements http.Handler.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Remove leading slash and known prefix.
	path := strings.TrimPrefix(r.URL.Path, "/")
	if strings.HasPrefix(path, "v1/workspace/") {
		path = strings.TrimPrefix(path, "v1/workspace/")
	} else if strings.HasPrefix(path, "workspace/") {
		// Allow alias when upstream router already stripped "/v1/" prefix.
		path = strings.TrimPrefix(path, "workspace/")
	}
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	// Kind is the first path segment (e.g. "mcp", "agents").
	kind := parts[0]
	// Special read-only handling for tools (not stored in repository)

	if kind == workspace.KindTool || kind == "tool" {
		if len(parts) != 1 || r.Method != http.MethodGet {
			encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
			return
		}
		defs := h.svc.ToolDefinitions()

		// Ensure deterministic order for the UI
		sort.Slice(defs, func(i, j int) bool {
			return defs[i].Name < defs[j].Name
		})

		// Optional pattern filter supporting legacy "pattern" as well as
		// alias query parameter "name" to improve discoverability.
		pat := strings.TrimSpace(r.URL.Query().Get("pattern"))
		if pat == "" {
			pat = strings.TrimSpace(r.URL.Query().Get("name"))
		}
		pat = strings.ToLower(pat)

		// Convert to plain objects with expected fields for UI tables
		filtered := make([]map[string]interface{}, 0, len(defs))
		for _, d := range defs {
			if pat != "" {
				if !strings.Contains(strings.ToLower(d.Name), pat) && !strings.Contains(strings.ToLower(d.Description), pat) {
					continue
				}
			}
			// Enrich schema lazily – only for matched entry.
			h.svc.EnrichToolDefinition(&d)

			filtered = append(filtered, map[string]interface{}{
				"name":         d.Name,
				"schema":       d.Parameters,
				"outputSchema": d.OutputSchema,
				"description":  d.Description,
			})
		}

		// Pagination: page (1-based) and size (default 50).
		pageSize := 50
		if sizeStr := r.URL.Query().Get("size"); sizeStr != "" {
			if v, err := strconv.Atoi(sizeStr); err == nil && v > 0 {
				pageSize = v
			}
		}
		page := 1
		if pageStr := r.URL.Query().Get("page"); pageStr != "" {
			if v, err := strconv.Atoi(pageStr); err == nil && v > 0 {
				page = v
			}
		}

		start := (page - 1) * pageSize
		end := start + pageSize
		if start > len(filtered) {
			start = len(filtered)
		}
		if end > len(filtered) {
			end = len(filtered)
		}
		items := make([]interface{}, 0, end-start)
		for _, it := range filtered[start:end] {
			items = append(items, it)
		}
		encode(w, http.StatusOK, items, nil)
		return
	}

	repo, ok := h.repo(kind)
	if !ok {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()

	// List all resources of kind
	if len(parts) == 1 {
		// ------------------------------------------------------------
		// Special cases when no explicit item name is present in the
		// URL. Historically the API only supported listing (GET) in
		// this form but for some kinds – notably "oauth" – it is more
		// convenient to let the server derive the storage key from the
		// incoming payload. We therefore allow implicit PUT for oauth.
		// ------------------------------------------------------------

		if r.Method != http.MethodGet {
			if (kind == workspace.KindOAuth || kind == "oauth") && (r.Method == http.MethodPut || r.Method == http.MethodPost) {
				// Derive name from payload's `name` or `id` field.
				body, err := io.ReadAll(r.Body)
				if err != nil {
					encode(w, http.StatusBadRequest, nil, err)
					return
				}
				var tmp struct {
					ID   string `json:"id" yaml:"id"`
					Name string `json:"name" yaml:"name"`
				}
				_ = yaml.Unmarshal(body, &tmp)
				key := tmp.Name
				if key == "" {
					key = tmp.ID
				}
				if key == "" {
					encode(w, http.StatusBadRequest, nil, fmt.Errorf("missing name/id field in payload"))
					return
				}
				// Rewind the body for downstream processing.
				r.Body = io.NopCloser(strings.NewReader(string(body)))

				// Rewrite URL to include derived key and fall through to
				// normal single-item PUT logic below.
				parts = append(parts, key)
				name := key
				// Now proceed similar to the single item PUT case later.
				// But easiest: delegate by recursively invoking ServeHTTP with updated URL.
				r.URL.Path = "/v1/workspace/" + kind + "/" + name
				h.ServeHTTP(w, r)
				return
			}
			encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
			return
		}
		names, err := repo.List(ctx)
		if err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}

		var items []interface{}
		for _, n := range names {
			raw, err := repo.GetRaw(ctx, n)
			if err != nil {
				continue // skip bad entries
			}
			var obj map[string]interface{}
			if err := yaml.Unmarshal(raw, &obj); err != nil {
				continue
			}
			// Ensure name present for UI tables when missing.
			if _, ok := obj["name"]; !ok {
				if id, ok := obj["id"]; ok {
					obj["name"] = id
				} else {
					obj["name"] = n
				}
			}
			items = append(items, obj)
		}

		encode(w, http.StatusOK, items, nil)
		return
	}

	// Single item operations: allow names with slashes after {kind}
	// Special view: /agent/{name}/edit -> enriched authoring meta
	// Detect early to avoid falling through to raw repo fetch.
	if (kind == workspace.KindAgent || kind == "agent") && len(parts) >= 3 && strings.EqualFold(parts[len(parts)-1], "edit") {
		if r.Method != http.MethodGet {
			encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
			return
		}
		// name may itself contain slashes; rebuild without the trailing '/edit'
		name := strings.Join(parts[1:len(parts)-1], "/")
		repo, _ := h.repo(kind)
		if repo == nil {
			encode(w, http.StatusNotFound, nil, fmt.Errorf("repository not found"))
			return
		}
		// Load authored agent YAML and unmarshal to typed struct
		raw, err := repo.GetRaw(ctx, name)
		if err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}
		var ag agent.Agent
		if err := yaml.Unmarshal(raw, &ag); err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}
		// Resolve repository filename (absolute) to compute baseDir
		var repoFilename string
		if ar, ok := h.svc.AgentRepo(), true; ok && ar != nil {
			repoFilename = ar.Filename(name)
		} else {
			repoFilename = name // fallback
		}
		view := buildAgentEditView(ctx, &ag, repoFilename)
		encode(w, http.StatusOK, view, nil)
		return
	}

	// Convenience wrapper: /agent/{name}/knowledge?scope=&idx=&path=&folderOnly=
	if (kind == workspace.KindAgent || kind == "agent") && len(parts) >= 3 && strings.EqualFold(parts[len(parts)-1], "knowledge") {
		if r.Method != http.MethodGet {
			encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
			return
		}
		name := strings.Join(parts[1:len(parts)-1], "/")
		repo, _ := h.repo(kind)
		if repo == nil {
			encode(w, http.StatusNotFound, nil, fmt.Errorf("repository not found"))
			return
		}
		raw, err := repo.GetRaw(ctx, name)
		if err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}
		var ag agent.Agent
		if err := yaml.Unmarshal(raw, &ag); err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}
		// Base dir for resolving relative URLs
		var repoFilename string
		if ar, ok := h.svc.AgentRepo(), true; ok && ar != nil {
			repoFilename = ar.Filename(name)
		} else {
			repoFilename = name
		}
		baseDir := filepath.Dir(repoFilename)

		q := r.URL.Query()
		scope := strings.TrimSpace(q.Get("scope"))
		if scope == "" {
			scope = "user"
		}
		idx := 0
		if s := strings.TrimSpace(q.Get("idx")); s != "" {
			if v, err := strconv.Atoi(s); err == nil && v >= 0 {
				idx = v
			}
		}
		subPath := strings.TrimPrefix(strings.TrimSpace(q.Get("path")), "/")
		folderOnly := strings.TrimSpace(q.Get("folderOnly"))

		// Select knowledge root by scope/index
		var authored string
		if scope == "system" {
			if idx >= 0 && idx < len(ag.SystemKnowledge) && ag.SystemKnowledge[idx] != nil {
				authored = strings.TrimSpace(ag.SystemKnowledge[idx].URL)
			}
		} else {
			if idx >= 0 && idx < len(ag.Knowledge) && ag.Knowledge[idx] != nil {
				authored = strings.TrimSpace(ag.Knowledge[idx].URL)
			}
		}
		if authored == "" {
			encode(w, http.StatusBadRequest, nil, fmt.Errorf("invalid knowledge root: scope=%s idx=%d", scope, idx))
			return
		}
		// Resolve authored URL relative to baseDir when needed
		resolved := authored
		if afsurl.Scheme(authored, "") == "" && !strings.HasPrefix(authored, string(filepath.Separator)) {
			resolved = filepath.Clean(filepath.Join(baseDir, authored))
		}
		// Append subPath when provided
		if subPath != "" {
			if afsurl.Scheme(resolved, "") != "" {
				// When scheme present (e.g., file://), append using URL join
				resolved = afsurl.Join(resolved, subPath)
			} else {
				resolved = filepath.Join(resolved, subPath)
			}
		}
		// Build URI for file-browser; ensure scheme is present so AFS treats as absolute
		uri := resolved
		if afsurl.Scheme(uri, "") == "" {
			// Treat as local file path
			if strings.HasPrefix(uri, string(filepath.Separator)) {
				uri = "file://" + uri
			} else {
				// Relative paths shouldn’t happen here, but guard anyway
				uri = afsurl.Join("file://", uri)
			}
		}
		// Redirect to the existing file-browser list endpoint to reuse its payload shape.
		v := neturl.Values{}
		v.Set("uri", uri)
		if folderOnly != "" {
			v.Set("folderOnly", folderOnly)
		}
		target := "/v1/workspace/file-browser/list?" + v.Encode()
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
		return
	}

	name := strings.Join(parts[1:], "/")

	switch r.Method {
	case http.MethodGet:
		raw, err := repo.GetRaw(ctx, name)
		if err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}
		var obj interface{}
		if err := yaml.Unmarshal(raw, &obj); err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}
		encode(w, http.StatusOK, obj, nil)
	case http.MethodPut, http.MethodPost: // allow POST as alias for PUT
		body, err := io.ReadAll(r.Body)
		if err != nil {
			encode(w, http.StatusBadRequest, nil, err)
			return
		}

		// If kind is agent and caller did not provide nested path, derive it
		// from the payload's id field so we store <kind>/<id>/<id>.yaml.
		if kind == workspace.KindAgent && !strings.Contains(name, "/") {
			var payload struct {
				ID   string `json:"id" yaml:"id"`
				Name string `json:"name" yaml:"name"`
			}
			_ = yaml.Unmarshal(body, &payload) // ignore error – empty ID is fine
			if payload.ID != "" {
				name = filepath.Join(payload.ID, payload.ID)
			} else if payload.Name != "" {
				name = filepath.Join(payload.Name, payload.Name)
			}
		}

		// Special secure handling first – must use original JSON body because
		// oauth.Config expects JSON fields like clientId, clientSecret.
		if kind == workspace.KindOAuth || kind == "oauth" {
			if orepo, ok := repo.(*oauthrepo.Repository); ok {
				var cfg oauth2.Config
				if uErr := json.Unmarshal(body, &cfg); uErr != nil {
					encode(w, http.StatusBadRequest, nil, uErr)
					return
				}
				if cfg.Name == "" {
					cfg.Name = name
				}
				if err := orepo.Save(ctx, name, &cfg); err != nil {
					encode(w, http.StatusInternalServerError, nil, err)
					return
				}
				encode(w, http.StatusOK, "ok", nil)
				return
			}
		}

		var warnings []string
		// Prefer typed structs when we have a mapping for this kind – ensures YAML tags.
		if inst := newInstance(kind); inst != nil {
			if err := json.Unmarshal(body, inst); err == nil {
				// Additional authoring validation for agents (non-blocking).
				if ag, ok := inst.(*agent.Agent); ok && ag != nil {
					// Compute baseDir from destination filename to resolve relative paths
					repoFilename := h.svc.AgentRepo().Filename(name)
					baseDir := filepath.Dir(repoFilename)
					// helper resolve
					resolve := func(authored string) (resolved string) {
						authored = strings.TrimSpace(authored)
						if authored == "" {
							return ""
						}
						if afsurl.Scheme(authored, "") == "" && !strings.HasPrefix(authored, string(filepath.Separator)) {
							return filepath.Clean(filepath.Join(baseDir, authored))
						}
						return authored
					}
					// prompt existence
					if ag.Prompt != nil && strings.TrimSpace(ag.Prompt.URI) != "" {
						p := resolve(ag.Prompt.URI)
						if ok, _ := afs.New().Exists(ctx, p); !ok {
							warnings = append(warnings, fmt.Sprintf("Prompt URI not found: %s (resolved %s)", strings.TrimSpace(ag.Prompt.URI), p))
						}
					}
					if ag.SystemPrompt != nil && strings.TrimSpace(ag.SystemPrompt.URI) != "" {
						p := resolve(ag.SystemPrompt.URI)
						if ok, _ := afs.New().Exists(ctx, p); !ok {
							warnings = append(warnings, fmt.Sprintf("System Prompt URI not found: %s (resolved %s)", strings.TrimSpace(ag.SystemPrompt.URI), p))
						}
					}
					// knowledge existence
					for i, k := range ag.Knowledge {
						if k == nil || strings.TrimSpace(k.URL) == "" {
							continue
						}
						r := resolve(k.URL)
						if ok, _ := afs.New().Exists(ctx, r); !ok {
							warnings = append(warnings, fmt.Sprintf("Knowledge[%d] not found: %s (resolved %s)", i, strings.TrimSpace(k.URL), r))
						}
					}
					for i, k := range ag.SystemKnowledge {
						if k == nil || strings.TrimSpace(k.URL) == "" {
							continue
						}
						r := resolve(k.URL)
						if ok, _ := afs.New().Exists(ctx, r); !ok {
							warnings = append(warnings, fmt.Sprintf("SystemKnowledge[%d] not found: %s (resolved %s)", i, strings.TrimSpace(k.URL), r))
						}
					}
					// ContextInputs schema checks
					if ag.ContextInputs != nil {
						rs := ag.ContextInputs.RequestedSchema
						if strings.TrimSpace(rs.Type) != "object" {
							warnings = append(warnings, "ContextInputs.requestedSchema.type should be 'object'")
						}
						// Missing required without default
						req := map[string]struct{}{}
						for _, k := range rs.Required {
							req[strings.TrimSpace(k)] = struct{}{}
						}
						for name, raw := range rs.Properties {
							if _, need := req[name]; need {
								if m, ok := raw.(map[string]interface{}); ok {
									if _, has := m["default"]; !has {
										warnings = append(warnings, fmt.Sprintf("ContextInputs field '%s' is required and has no default", name))
									}
								}
							}
						}
					}
				}
				if data, marshalErr := yaml.Marshal(inst); marshalErr == nil && len(data) > 0 {
					body = data
				}
			}
		} else {
			// Generic map fallback.
			transient := map[string]interface{}{}
			if err := json.Unmarshal(body, &transient); err == nil {
				if data, _ := yaml.Marshal(transient); len(data) > 0 {
					body = data
				}
			}
		}

		if err := repo.Add(ctx, name, body); err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}
		// Return OK plus any non-blocking warnings so UI can surface them.
		if len(warnings) > 0 {
			encode(w, http.StatusOK, map[string]interface{}{"result": "ok", "warnings": warnings}, nil)
			return
		}
		encode(w, http.StatusOK, "ok", nil)
	case http.MethodDelete:
		if err := repo.Delete(ctx, name); err != nil {
			encode(w, http.StatusInternalServerError, nil, err)
			return
		}
		encode(w, http.StatusOK, "ok", nil)
	default:
		encode(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
	}
}

// repo resolves kind → repository.
func (h *handler) repo(kind string) (rawRepository, bool) {
	switch kind {
	case workspace.KindAgent, "agent":
		return h.svc.AgentRepo(), true
	case workspace.KindModel, "model":
		return h.svc.ModelRepo(), true
	case workspace.KindWorkflow, "workflow":
		return h.svc.WorkflowRepo(), true
	case workspace.KindOAuth:
		return h.svc.OAuthRepo(), true
	case workspace.KindMCP:
		return h.svc.MCPRepo(), true
	case workspace.KindFeeds:
		return h.svc.FeednRepo(), true
		//	case workspace.KindA2A:
		//		return h.svc.A2ARepo(), true
	default:
		return nil, false
	}
}

// rawRepository is the minimal set of operations the HTTP layer requires.
type rawRepository interface {
	List(ctx context.Context) ([]string, error)
	GetRaw(ctx context.Context, name string) ([]byte, error)
	Add(ctx context.Context, name string, data []byte) error
	Delete(ctx context.Context, name string) error
}

// Deprecated helpers jsonEncode/writeErr removed – unified encode() used instead.
