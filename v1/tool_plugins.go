package v1

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	"gopkg.in/yaml.v3"
)

const defaultInternalServices = "system/exec,system/os,system/patch,orchestration/plan,llm/agents,resources,internal/message"

func configureRegistry(ctx context.Context, rt *executor.Runtime, workspaceRoot string) {
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
	rt.Registry.Initialize(ctx)
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
		raw = strings.Join(loadInternalServicesFromConfig(workspaceRoot), ",")
	}
	if raw == "" {
		raw = defaultInternalServices
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

func loadInternalServicesFromConfig(workspaceRoot string) []string {
	if strings.TrimSpace(workspaceRoot) == "" {
		return nil
	}
	path := filepath.Join(workspaceRoot, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg map[string]interface{}
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		log.Printf("agently-app: failed to parse %s: %v", path, err)
		return nil
	}

	out := valuesToServices(cfg["internalMCPServices"])
	if len(out) > 0 {
		return out
	}
	internal := mapLookup(cfg, "internalMCP")
	if len(internal) == 0 {
		internal = mapLookup(cfg, "internal_mcp")
	}
	return valuesToServices(internal["services"])
}

func mapLookup(source map[string]interface{}, key string) map[string]interface{} {
	value, ok := source[key]
	if !ok || value == nil {
		return nil
	}
	result, _ := value.(map[string]interface{})
	return result
}

func valuesToServices(value interface{}) []string {
	switch actual := value.(type) {
	case []interface{}:
		out := make([]string, 0, len(actual))
		for _, item := range actual {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
		return out
	case []string:
		return actual
	case string:
		if strings.TrimSpace(actual) == "" {
			return nil
		}
		parts := strings.Split(actual, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if text := strings.TrimSpace(part); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
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
	case "internal/message":
		summaryModel := ""
		defaultModel := ""
		embedModel := ""
		if rt.Defaults != nil {
			summaryModel = rt.Defaults.Model
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
	if rt == nil || rt.Agent == nil {
		return nil
	}
	finder := rt.Agent.Finder()
	if finder == nil {
		return nil
	}
	agentIDs := discoverAgentIDs(workspaceRoot)
	if len(agentIDs) == 0 {
		return nil
	}
	items := make([]llmagents.ListItem, 0, len(agentIDs))
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
