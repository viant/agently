package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/viant/agently-core/app/executor"
	agentmdl "github.com/viant/agently-core/protocol/agent"
	"github.com/viant/agently-core/protocol/tool"
	svc "github.com/viant/agently-core/protocol/tool/service"
	llmagents "github.com/viant/agently-core/protocol/tool/service/llm/agents"
	msgservice "github.com/viant/agently-core/protocol/tool/service/message"
	planservice "github.com/viant/agently-core/protocol/tool/service/orchestration/plan"
	resourcesvc "github.com/viant/agently-core/protocol/tool/service/resources"
	toolexec "github.com/viant/agently-core/protocol/tool/service/system/exec"
	toolos "github.com/viant/agently-core/protocol/tool/service/system/os"
	toolpatch "github.com/viant/agently-core/protocol/tool/service/system/patch"
	coresdk "github.com/viant/agently-core/sdk"
	svca2a "github.com/viant/agently-core/service/a2a"
	svcauth "github.com/viant/agently-core/service/auth"
	wscfg "github.com/viant/agently-core/workspace/config"
	"gopkg.in/yaml.v3"
)

var allInternalServices = []string{
	"system/exec",
	"system/os",
	"system/patch",
	"orchestration/plan",
	"llm/agents",
	"resources",
	"message",
}

func ConfigureRegistry(ctx context.Context, rt *executor.Runtime, workspaceRoot string) {
	if rt == nil || rt.Registry == nil {
		return
	}
	if debugEnabled() {
		rt.Registry.SetDebugLogger(os.Stdout)
	}
	enabled := resolveInternalServiceList(workspaceRoot)
	for _, name := range enabled {
		service := internalServiceFactory(rt, workspaceRoot, name)
		if service == nil {
			log.Printf("agently-app: unsupported internal MCP service %q (skipped)", name)
			continue
		}
		tool.AddInternalService(rt.Registry, service)
	}
	go func() {
		warmupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		log.Printf("agently-app: starting async registry warmup")
		rt.Registry.Initialize(warmupCtx)
		log.Printf("agently-app: registry warmup finished")
	}()
}

func debugEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AGENTLY_DEBUG"))) {
	case "1", "true", "yes", "y", "on":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AGENTLY_SCHEDULER_DEBUG"))) {
	case "1", "true", "yes", "y", "on":
		return true
	}
	return false
}

func resolveInternalServiceList(workspaceRoot string) []string {
	raw := strings.TrimSpace(os.Getenv("AGENTLY_INTERNAL_MCP_SERVICES"))
	if raw == "" {
		if cfgServices, ok := loadInternalServicesFromConfig(workspaceRoot); ok {
			if len(cfgServices) == 0 {
				return append([]string{}, allInternalServices...)
			}
			raw = strings.Join(cfgServices, ",")
		}
	}
	if raw == "" {
		raw = strings.Join(allInternalServices, ",")
	}
	parts := strings.Split(raw, ",")
	seen := map[string]struct{}{}
	var out []string
	for _, part := range parts {
		name := normalizeServiceName(part)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func loadInternalServicesFromConfig(workspaceRoot string) ([]string, bool) {
	cfg, err := wscfg.Load(workspaceRoot)
	if err != nil {
		log.Printf("agently-app: failed to load workspace config: %v", err)
		return nil, false
	}
	if cfg == nil {
		return nil, false
	}
	return cfg.InternalServiceList()
}

func normalizeServiceName(value string) string {
	name := strings.ToLower(strings.TrimSpace(value))
	if name == "" {
		return ""
	}
	name = strings.ReplaceAll(name, ":", "/")
	name = strings.ReplaceAll(name, "_", "/")
	name = strings.ReplaceAll(name, "-", "/")
	name = strings.ReplaceAll(name, "//", "/")
	return strings.Trim(name, "/")
}

func internalServiceFactory(rt *executor.Runtime, workspaceRoot, name string) svc.Service {
	if rt == nil {
		return nil
	}
	switch normalizeServiceName(name) {
	case "system/exec":
		return toolexec.New()
	case "system/os":
		return toolos.New()
	case "system/patch":
		return toolpatch.New()
	case "orchestration/plan":
		return planservice.New()
	case "llm/agents":
		if rt.Agent == nil {
			return nil
		}
		opts := []llmagents.Option{
			llmagents.WithConversationClient(rt.Conversation),
			llmagents.WithDirectoryProvider(agentDirectoryProvider(rt, workspaceRoot)),
			llmagents.WithExternalRunner(externalA2ARunner(workspaceRoot)),
		}
		if rt.Streaming != nil {
			opts = append(opts, llmagents.WithStreamPublisher(rt.Streaming))
		}
		return llmagents.New(rt.Agent, opts...)
	case "resources":
		opts := []func(*resourcesvc.Service){
			resourcesvc.WithMCPManager(rt.MCPManager),
			resourcesvc.WithConversationClient(rt.Conversation),
		}
		if rt.Agent != nil {
			opts = append(opts, resourcesvc.WithAgentFinder(rt.Agent.Finder()))
		}
		if rt.Defaults != nil {
			opts = append(opts, resourcesvc.WithDefaultEmbedder(rt.Defaults.Embedder))
		}
		return resourcesvc.New(nil, opts...)
	case "internal/message", "message":
		summaryModel := ""
		defaultModel := ""
		embedModel := ""
		if rt.Defaults != nil {
			summaryModel = rt.Defaults.SummaryModel
			defaultModel = rt.Defaults.Model
			embedModel = rt.Defaults.Embedder
		}
		return msgservice.NewWithDeps(rt.Conversation, rt.Core, nil, 0, 0, summaryModel, "", defaultModel, embedModel)
	default:
		return nil
	}
}

func agentDirectoryProvider(rt *executor.Runtime, workspaceRoot string) func() []llmagents.ListItem {
	return func() []llmagents.ListItem {
		return loadAgentDirectory(rt, workspaceRoot)
	}
}

func loadAgentDirectory(rt *executor.Runtime, workspaceRoot string) []llmagents.ListItem {
	items := make([]llmagents.ListItem, 0)
	if rt != nil && rt.Agent != nil {
		finder := rt.Agent.Finder()
		if finder != nil {
			agentIDs := discoverAgentIDs(workspaceRoot)
			for _, id := range agentIDs {
				ag, err := finder.Find(context.Background(), id)
				if err != nil || ag == nil {
					if debugEnabled() {
						log.Printf("agently-app: skipped agent directory entry %q: %v", id, err)
					}
					continue
				}
				items = append(items, agentToListItem(ag))
			}
		}
	}
	for _, spec := range loadExternalA2ASpecs(workspaceRoot) {
		items = append(items, externalA2AToListItem(spec))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority > items[j].Priority
		}
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].ID < items[j].ID
	})
	return items
}

func discoverAgentIDs(workspaceRoot string) []string {
	root := filepath.Join(strings.TrimSpace(workspaceRoot), "agents")
	entries, err := os.ReadDir(root)
	if err != nil {
		if debugEnabled() {
			log.Printf("agently-app: failed to read agent directory %s: %v", root, err)
		}
		return nil
	}
	seen := map[string]struct{}{}
	var ids []string
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if entry.IsDir() {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			ids = append(ids, name)
			continue
		}
		if filepath.Ext(name) != ".yaml" {
			continue
		}
		id := strings.TrimSuffix(name, filepath.Ext(name))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func agentToListItem(ag *agentmdl.Agent) llmagents.ListItem {
	item := llmagents.ListItem{
		ID:       strings.TrimSpace(ag.ID),
		Name:     strings.TrimSpace(ag.Name),
		Internal: true,
		Source:   "internal",
	}
	if item.ID == "" {
		item.ID = strings.TrimSpace(ag.Name)
	}
	if profile := ag.Profile; profile != nil {
		if name := strings.TrimSpace(profile.Name); name != "" {
			item.Name = name
		}
		item.Description = strings.TrimSpace(profile.Description)
		item.Tags = append(item.Tags, profile.Tags...)
		item.Priority = profile.Rank
		item.Capabilities = profile.Capabilities
		item.Responsibilities = append(item.Responsibilities, profile.Responsibilities...)
		item.InScope = append(item.InScope, profile.InScope...)
		item.OutOfScope = append(item.OutOfScope, profile.OutOfScope...)
	}
	if item.Name == "" {
		item.Name = item.ID
	}
	if item.Description == "" && ag.Persona != nil {
		item.Description = strings.TrimSpace(ag.Persona.Summary)
	}
	if item.Summary == "" {
		item.Summary = item.Description
	}
	return item
}

func externalA2AToListItem(spec *svca2a.ExternalSpec) llmagents.ListItem {
	item := llmagents.ListItem{
		ID:       strings.TrimSpace(spec.ID),
		Name:     strings.TrimSpace(spec.Directory.Name),
		Internal: false,
		Source:   "external",
	}
	if item.Name == "" {
		item.Name = item.ID
	}
	item.Description = strings.TrimSpace(spec.Directory.Description)
	item.Tags = append(item.Tags, spec.Directory.Tags...)
	item.Priority = spec.Directory.Priority
	item.Summary = item.Description
	return item
}

func externalA2ARunner(workspaceRoot string) func(context.Context, string, string, map[string]interface{}) (string, string, string, string, bool, []string, error) {
	return func(ctx context.Context, agentID, objective string, payload map[string]interface{}) (string, string, string, string, bool, []string, error) {
		specs := loadExternalA2ASpecs(workspaceRoot)
		spec, ok := specs[strings.TrimSpace(agentID)]
		if !ok || spec == nil {
			return "", "", "", "", false, nil, nil
		}
		messages := []svca2a.Message{{
			Role: "user",
			Parts: []svca2a.Part{{
				Type: "text",
				Text: objective,
			}},
		}}
		contextID := payloadString(payload, "conversationId")
		if contextID == "" {
			contextID = payloadString(payload, "contextId")
		}
		if contextID == "" {
			contextID = payloadString(payload, "linkedConversationId")
		}
		var contextRef *string
		if contextID != "" {
			contextRef = &contextID
		}
		effectiveUser := strings.TrimSpace(svcauth.EffectiveUserID(ctx))
		idToken := strings.TrimSpace(svcauth.MCPAuthToken(ctx, true))
		accessToken := strings.TrimSpace(svcauth.MCPAuthToken(ctx, false))
		tokenMode := "none"
		tokenFP := ""
		if idToken != "" {
			tokenMode = "id_token"
			tokenFP = tokenFingerprint(idToken)
		} else if accessToken != "" {
			tokenMode = "bearer"
			tokenFP = tokenFingerprint(accessToken)
		}
		log.Printf("[a2a-external] dispatch agent=%q user=%q context_id=%q endpoint=%q token_mode=%s token_fp=%s", strings.TrimSpace(agentID), effectiveUser, contextID, strings.TrimSpace(spec.JSONRPCURL), tokenMode, tokenFP)
		task, err := executeExternalA2A(ctx, spec, strings.TrimSpace(agentID), messages, contextRef)
		if err != nil {
			log.Printf("[a2a-external] error agent=%q user=%q context_id=%q endpoint=%q err=%v", strings.TrimSpace(agentID), effectiveUser, contextID, strings.TrimSpace(spec.JSONRPCURL), err)
			return "", "", "", "", false, nil, err
		}
		answer := extractA2ATaskAnswer(task)
		status := strings.TrimSpace(string(task.Status.State))
		if status == "" {
			status = "completed"
		}
		log.Printf("[a2a-external] completed agent=%q user=%q task_id=%q remote_context_id=%q status=%q", strings.TrimSpace(agentID), effectiveUser, strings.TrimSpace(task.ID), strings.TrimSpace(task.ContextID), status)
		return answer, status, strings.TrimSpace(task.ID), strings.TrimSpace(task.ContextID), strings.TrimSpace(spec.StreamURL) != "", nil, nil
	}
}

func extractA2ATaskAnswer(task *svca2a.Task) string {
	if task == nil {
		return ""
	}
	var parts []string
	for _, artifact := range task.Artifacts {
		for _, part := range artifact.Parts {
			if strings.TrimSpace(part.Text) != "" {
				parts = append(parts, part.Text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func payloadString(payload map[string]interface{}, key string) string {
	if payload == nil {
		return ""
	}
	raw, ok := payload[key]
	if !ok || raw == nil {
		return ""
	}
	if text, ok := raw.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func loadExternalA2ASpecs(workspaceRoot string) map[string]*svca2a.ExternalSpec {
	root := filepath.Join(strings.TrimSpace(workspaceRoot), "a2a")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	result := map[string]*svca2a.ExternalSpec{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(root, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			if debugEnabled() {
				log.Printf("agently-app: failed to read external A2A spec %s: %v", path, err)
			}
			continue
		}
		spec := &svca2a.ExternalSpec{}
		if err := yaml.Unmarshal(data, spec); err != nil {
			if debugEnabled() {
				log.Printf("agently-app: failed to parse external A2A spec %s: %v", path, err)
			}
			continue
		}
		spec.ID = strings.TrimSpace(spec.ID)
		if spec.ID == "" {
			spec.ID = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		if spec.ID == "" || !spec.IsEnabled() || strings.TrimSpace(spec.JSONRPCURL) == "" {
			continue
		}
		result[spec.ID] = spec
	}
	return result
}

func tokenFingerprint(token string) string {
	if strings.TrimSpace(token) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:6])
}

func executeExternalA2A(ctx context.Context, spec *svca2a.ExternalSpec, fallbackAgentID string, messages []svca2a.Message, contextRef *string) (*svca2a.Task, error) {
	if spec == nil {
		return nil, fmt.Errorf("nil external a2a spec")
	}
	if baseURL, routeAgentID, ok := sharedA2ARoute(strings.TrimSpace(spec.JSONRPCURL)); ok {
		agentID := routeAgentID
		if agentID == "" {
			agentID = strings.TrimSpace(fallbackAgentID)
		}
		if agentID == "" {
			return nil, fmt.Errorf("shared a2a route requires agent id")
		}
		httpClient, err := newSessionHTTPClient(ctx, baseURL)
		if err != nil {
			return nil, err
		}
		client, err := coresdk.NewHTTP(baseURL, coresdk.WithHTTPClient(httpClient))
		if err != nil {
			return nil, err
		}
		req := &svca2a.SendMessageRequest{Messages: messages}
		if contextRef != nil {
			req.ContextID = strings.TrimSpace(*contextRef)
		}
		resp, err := client.SendA2AMessage(ctx, agentID, req)
		if err != nil {
			return nil, err
		}
		return &resp.Task, nil
	}
	client := svca2a.NewClient(strings.TrimSpace(spec.JSONRPCURL), svca2a.WithHeaders(spec.Headers))
	return client.SendMessage(ctx, messages, contextRef)
}

func sharedA2ARoute(raw string) (baseURL, agentID string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", false
	}
	const prefix = "/v1/api/a2a/agents/"
	if !strings.HasPrefix(u.Path, prefix) || !strings.HasSuffix(u.Path, "/message") {
		return "", "", false
	}
	trimmed := strings.TrimSuffix(strings.TrimPrefix(u.Path, prefix), "/message")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "", "", false
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String(), trimmed, true
}

func newSessionHTTPClient(ctx context.Context, baseURL string) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Jar: jar, Timeout: 60 * time.Second}
	idToken := strings.TrimSpace(svcauth.MCPAuthToken(ctx, true))
	accessToken := strings.TrimSpace(svcauth.MCPAuthToken(ctx, false))
	bearer := idToken
	if bearer == "" {
		bearer = accessToken
	}
	if bearer == "" {
		return nil, fmt.Errorf("missing bearer for shared a2a session bootstrap")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/v1/api/auth/session", strings.NewReader(`{}`))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("shared a2a session bootstrap failed: %s", resp.Status)
	}
	return client, nil
}
