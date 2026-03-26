package agently

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"sync"

	"github.com/viant/afs"
	_ "github.com/viant/afs/file"
	"github.com/viant/agently-core/adapter/http/ui"
	"github.com/viant/agently-core/app/executor"
	execconfig "github.com/viant/agently-core/app/executor/config"
	cancels "github.com/viant/agently-core/app/store/conversation/cancel"
	embedprovider "github.com/viant/agently-core/genai/embedder/provider"
	"github.com/viant/agently-core/genai/llm"
	provider "github.com/viant/agently-core/genai/llm/provider"
	agentmodel "github.com/viant/agently-core/protocol/agent"
	agentfinder "github.com/viant/agently-core/protocol/agent/finder"
	agentloader "github.com/viant/agently-core/protocol/agent/loader"
	integrate "github.com/viant/agently-core/protocol/mcp/auth/integrate"
	mcpcookies "github.com/viant/agently-core/protocol/mcp/cookies"
	mcpexpose "github.com/viant/agently-core/protocol/mcp/expose"
	"github.com/viant/agently-core/protocol/tool"
	"github.com/viant/agently-core/sdk"
	svca2a "github.com/viant/agently-core/service/a2a"
	svcauthctx "github.com/viant/agently-core/service/auth"
	svcscheduler "github.com/viant/agently-core/service/scheduler"
	svcworkspace "github.com/viant/agently-core/service/workspace"
	"github.com/viant/agently-core/workspace"
	legacyworkspace "github.com/viant/agently-core/workspace"
	embedderloader "github.com/viant/agently-core/workspace/loader/embedder"
	wsfs "github.com/viant/agently-core/workspace/loader/fs"
	modelloader "github.com/viant/agently-core/workspace/loader/model"
	mcprepo "github.com/viant/agently-core/workspace/repository/mcp"
	"github.com/viant/agently-core/workspace/service/meta"
	"github.com/viant/agently/bootstrap"
	deployui "github.com/viant/agently/deployment/ui"
	coremeta "github.com/viant/agently/metadata"
	agentlyrt "github.com/viant/agently/runtime"
	"github.com/viant/agently/server"
	authtransport "github.com/viant/mcp/client/auth/transport"
	"gopkg.in/yaml.v3"
)

type servedUIBundle struct {
	Name  string
	FS    iofs.FS
	Index []byte
}

type ServeOptions struct {
	Addr          string
	WorkspacePath string
	UIDist        string
	Debug         bool
	Policy        string // tool policy: auto|ask|deny
	ExposeMCP     bool   // expose tools over MCP HTTP server
}

func Serve(options ServeOptions) error {
	addr := envOr("AGENTLY_ADDR", ":8080")
	if value := strings.TrimSpace(options.Addr); value != "" {
		addr = value
	}
	workspacePath := envOr("AGENTLY_WORKSPACE", defaultWorkspace())
	if value := strings.TrimSpace(options.WorkspacePath); value != "" {
		workspacePath = value
	}
	uiDist := strings.TrimSpace(options.UIDist)
	if uiDist == "" {
		uiDist = strings.TrimSpace(os.Getenv("AGENTLY_UI_DIST"))
	}
	debugEnabled := options.Debug ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("AGENTLY_DEBUG")), "1") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("AGENTLY_DEBUG")), "true")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if debugEnabled {
		enableDebugLogging()
	}

	workspace.SetRoot(workspacePath)
	log.Printf("workspace: %s", workspacePath)
	bootstrap.SetBootstrapHook()
	workspace.EnsureDefault(afs.New())
	defer workspace.SetBootstrapHook(nil)

	defaults := loadWorkspaceDefaults(workspace.Root())
	applyWorkspacePathDefaults(defaults)

	rt, client, agentFndr, err := newRuntime(ctx, defaults)
	if err != nil {
		return fmt.Errorf("failed to initialize runtime: %w", err)
	}

	authRuntime, err := server.NewAuthRuntime(ctx, workspace.Root(), rt)
	if err != nil {
		return fmt.Errorf("failed to initialize auth runtime: %w", err)
	}
	speechHandler := server.NewSpeechHandler()

	metadataHandler := svcworkspace.NewMetadataHandler(rt.Defaults, rt.Store, "agently-v1")
	fileBrowserHandler := svcworkspace.NewFileBrowserHandler()
	scheduleStore, err := svcscheduler.NewDatlyStore(ctx, rt.DAO, rt.Data)
	if err != nil {
		return fmt.Errorf("failed to initialize scheduler store: %w", err)
	}
	schedulerSvc := svcscheduler.New(scheduleStore, rt.Agent,
		svcscheduler.WithConversationClient(rt.Conversation),
		svcscheduler.WithTokenProvider(rt.TokenProvider),
	)
	schedulerHandler := svcscheduler.NewHandler(schedulerSvc)
	schedulerOpts := agentlyrt.SchedulerOptionsFromEnv()
	a2aSvc := svca2a.New(rt.Agent, agentFndr)
	a2aHandler := svca2a.NewHandler(a2aSvc)

	handlerOpts := []sdk.HandlerOption{
		sdk.WithMetadataHandler(metadataHandler),
		sdk.WithFileBrowser(fileBrowserHandler),
		sdk.WithA2AHandler(a2aHandler),
	}
	// Only mount scheduler endpoints/watchdog when not suppressed.
	// For serverless deployments, set AGENTLY_SCHEDULER_API=false to suppress.
	if schedulerOpts.EnableAPI || schedulerOpts.EnableWatchdog {
		handlerOpts = append(handlerOpts, sdk.WithScheduler(schedulerSvc, schedulerHandler, schedulerOpts))
	}
	sdkHandler, err := sdk.NewHandlerWithContext(ctx, client, handlerOpts...)
	if err != nil {
		return fmt.Errorf("failed to create sdk handler: %w", err)
	}
	svca2a.StartServers(ctx, &svca2a.ServerConfig{
		AgentService: rt.Agent,
		AgentFinder:  agentFndr,
		AgentIDs:     discoverWorkspaceAgentIDs(workspace.Root()),
		JWTService:   authRuntime.JWTService(),
	})
	apiHandler := server.WithAuthExtensions(sdkHandler, authRuntime)
	metaRoot := "embed://localhost/"
	metaHandler := ui.NewEmbeddedHandler(metaRoot, &coremeta.FS)
	uiBundle := servedUIBundle{Name: "v1", FS: deployui.FS, Index: deployui.Index}

	h := newRouter(apiHandler, metaHandler, speechHandler, uiDist, uiBundle)
	srv := &http.Server{Addr: addr, Handler: h}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	// Expose MCP server when explicitly requested or when workspace config
	// declares an MCP server port.
	var mcpSrv *http.Server
	mcpCfg := loadMCPServerConfig(workspace.Root())
	exposeMCP := options.ExposeMCP || (mcpCfg != nil && mcpCfg.Port > 0)
	if exposeMCP && mcpCfg != nil && mcpCfg.Port > 0 {
		mcpSrv, err = mcpexpose.NewHTTPServer(ctx, &runtimeExecutorAdapter{rt: rt}, mcpCfg)
		if err != nil {
			return fmt.Errorf("init mcp server: %w", err)
		}
		mcpSrv.Handler = server.WithAuthProtection(mcpSrv.Handler, authRuntime)
		go func() {
			log.Printf("Agently MCP server listening on %s", mcpSrv.Addr)
			if err := mcpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("MCP server error: %v", err)
			}
		}()
	}

	log.Printf("agently serve listening on %s (workspace=%s ui=%s)", addr, workspace.Root(), uiBundle.Name)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		if mcpSrv != nil {
			_ = mcpSrv.Close()
		}
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func enableDebugLogging() {
	for _, name := range []string{
		"AGENTLY_DEBUG",
		"AGENTLY_SCHEDULER_DEBUG",
	} {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			_ = os.Setenv(name, "1")
		}
	}
	log.Printf("agently v1 debug logging enabled")
}

func discoverWorkspaceAgentIDs(workspaceRoot string) []string {
	root := filepath.Join(strings.TrimSpace(workspaceRoot), "agents")
	entries, err := os.ReadDir(root)
	if err != nil {
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
	return ids
}

func newRuntime(ctx context.Context, defaults *execconfig.Defaults) (*executor.Runtime, sdk.Client, agentmodel.Finder, error) {
	fs := afs.New()
	wsMeta := meta.New(fs, workspace.Root())
	agentLdr := agentloader.New(agentloader.WithMetaService(wsMeta))
	agentFndr := agentfinder.New(agentfinder.WithLoader(agentLdr))
	modelLdr := modelloader.New(wsfs.WithMetaService[provider.Config](wsMeta))
	modelFndr := agentlyrt.NewModelFinder(modelLdr)
	embedderLdr := embedderloader.New(wsfs.WithMetaService[embedprovider.Config](wsMeta))
	embedderFndr := agentlyrt.NewEmbedderFinder(embedderLdr)
	cancelRegistry := cancels.NewMemory()

	// Per-user MCP auth RoundTripper and cookie jar — mirrors original agently
	// serve.go so that MCP servers requiring OAuth (e.g. sqlkit -o) get BFF auth.
	mcpRepo := mcprepo.New(fs)
	cookieProvider := mcpcookies.New(fs, mcpRepo)
	jarProvider := cookieProvider.Jar
	// Lazy reference to the embedded client — populated after Build().
	var embeddedClient *sdk.EmbeddedClient
	var (
		rtMu     sync.Mutex
		rtByUser = map[string]*authtransport.RoundTripper{}
	)
	authRTProvider := func(ctx context.Context) *authtransport.RoundTripper {
		user := strings.TrimSpace(svcauthctx.EffectiveUserID(ctx))
		if user == "" {
			user = "anonymous"
		}
		rtMu.Lock()
		defer rtMu.Unlock()
		if v, ok := rtByUser[user]; ok && v != nil {
			return v
		}
		j, jerr := jarProvider(ctx)
		if jerr != nil {
			return nil
		}
		// Use ElicitationFlow so OAuth URLs are surfaced to the web UI as
		// OOB elicitations (popup) instead of CLI browser.Open().
		authRT, _ := integrate.NewAuthRoundTripperWithElicitation(j, http.DefaultTransport, 0, func(ctx context.Context, authURL string) error {
			if embeddedClient != nil {
				return embeddedClient.RecordOOBAuthElicitation(ctx, authURL)
			}
			log.Printf("[mcp-auth] OAuth URL (client not ready): %s", authURL)
			return nil
		})
		rtByUser[user] = authRT
		return authRT
	}

	rt, err := executor.NewBuilder().
		WithAgentFinder(agentFndr).
		WithModelFinder(modelFndr).
		WithEmbedderFinder(embedderFndr).
		WithCancelRegistry(cancelRegistry).
		WithDefaults(defaults).
		WithMCPAuthRTProvider(authRTProvider).
		WithMCPCookieJarProvider(jarProvider).
		WithMCPUserIDExtractor(func(ctx context.Context) string {
			return strings.TrimSpace(svcauthctx.EffectiveUserID(ctx))
		}).
		Build(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	agentlyrt.ConfigureRegistry(ctx, rt, workspace.Root())
	client, err := sdk.NewEmbeddedFromRuntime(rt)
	if err != nil {
		return nil, nil, nil, err
	}
	// Wire the lazy reference so auth elicitations use the proper DB-backed flow.
	embeddedClient = client
	return rt, client, agentFndr, nil
}

// loadWorkspaceDefaults reads the workspace config.yaml and merges with
// hardcoded fallbacks. Workspace values take priority.
func loadWorkspaceDefaults(wsRoot string) *execconfig.Defaults {
	fallback := &execconfig.Defaults{
		Model:    "openai_gpt-5.2",
		Embedder: "openai_text",
		Agent:    "chatter",
	}
	configPath := filepath.Join(wsRoot, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fallback
	}
	var cfg struct {
		Default execconfig.Defaults `yaml:"default"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fallback
	}
	if strings.TrimSpace(cfg.Default.Agent) != "" {
		fallback.Agent = strings.TrimSpace(cfg.Default.Agent)
	}
	if strings.TrimSpace(cfg.Default.Model) != "" {
		fallback.Model = strings.TrimSpace(cfg.Default.Model)
	}
	if strings.TrimSpace(cfg.Default.Embedder) != "" {
		fallback.Embedder = strings.TrimSpace(cfg.Default.Embedder)
	}
	if strings.TrimSpace(cfg.Default.RuntimeRoot) != "" {
		fallback.RuntimeRoot = strings.TrimSpace(cfg.Default.RuntimeRoot)
	}
	if strings.TrimSpace(cfg.Default.StatePath) != "" {
		fallback.StatePath = strings.TrimSpace(cfg.Default.StatePath)
	}
	if strings.TrimSpace(cfg.Default.DBPath) != "" {
		fallback.DBPath = strings.TrimSpace(cfg.Default.DBPath)
	}
	if strings.TrimSpace(cfg.Default.SummaryModel) != "" {
		fallback.SummaryModel = strings.TrimSpace(cfg.Default.SummaryModel)
	}
	if strings.TrimSpace(cfg.Default.SummaryPrompt) != "" {
		fallback.SummaryPrompt = strings.TrimSpace(cfg.Default.SummaryPrompt)
	}
	if cfg.Default.SummaryLastN > 0 {
		fallback.SummaryLastN = cfg.Default.SummaryLastN
	}
	if strings.TrimSpace(cfg.Default.AgentAutoSelection.Model) != "" {
		fallback.AgentAutoSelection.Model = strings.TrimSpace(cfg.Default.AgentAutoSelection.Model)
	}
	if strings.TrimSpace(cfg.Default.AgentAutoSelection.Prompt) != "" {
		fallback.AgentAutoSelection.Prompt = strings.TrimSpace(cfg.Default.AgentAutoSelection.Prompt)
	}
	if strings.TrimSpace(cfg.Default.AgentAutoSelection.OutputKey) != "" {
		fallback.AgentAutoSelection.OutputKey = strings.TrimSpace(cfg.Default.AgentAutoSelection.OutputKey)
	}
	if cfg.Default.AgentAutoSelection.TimeoutSec > 0 {
		fallback.AgentAutoSelection.TimeoutSec = cfg.Default.AgentAutoSelection.TimeoutSec
	}
	if cfg.Default.ToolAutoSelection.Enabled {
		fallback.ToolAutoSelection.Enabled = true
	}
	if strings.TrimSpace(cfg.Default.ToolAutoSelection.Model) != "" {
		fallback.ToolAutoSelection.Model = strings.TrimSpace(cfg.Default.ToolAutoSelection.Model)
	}
	if strings.TrimSpace(cfg.Default.ToolAutoSelection.Prompt) != "" {
		fallback.ToolAutoSelection.Prompt = strings.TrimSpace(cfg.Default.ToolAutoSelection.Prompt)
	}
	if strings.TrimSpace(cfg.Default.ToolAutoSelection.OutputKey) != "" {
		fallback.ToolAutoSelection.OutputKey = strings.TrimSpace(cfg.Default.ToolAutoSelection.OutputKey)
	}
	if cfg.Default.ToolAutoSelection.MaxBundles > 0 {
		fallback.ToolAutoSelection.MaxBundles = cfg.Default.ToolAutoSelection.MaxBundles
	}
	if cfg.Default.ToolAutoSelection.TimeoutSec > 0 {
		fallback.ToolAutoSelection.TimeoutSec = cfg.Default.ToolAutoSelection.TimeoutSec
	}
	if strings.TrimSpace(cfg.Default.CapabilityPrompt) != "" {
		fallback.CapabilityPrompt = strings.TrimSpace(cfg.Default.CapabilityPrompt)
	}
	return fallback
}

func applyWorkspacePathDefaults(defaults *execconfig.Defaults) {
	if defaults == nil {
		return
	}
	if strings.TrimSpace(os.Getenv("AGENTLY_RUNTIME_ROOT")) == "" && strings.TrimSpace(defaults.RuntimeRoot) != "" {
		_ = os.Setenv("AGENTLY_RUNTIME_ROOT", defaults.RuntimeRoot)
		workspace.SetRuntimeRoot(defaults.RuntimeRoot)
		legacyworkspace.SetRuntimeRoot(defaults.RuntimeRoot)
	}
	if strings.TrimSpace(os.Getenv("AGENTLY_STATE_PATH")) == "" && strings.TrimSpace(defaults.StatePath) != "" {
		_ = os.Setenv("AGENTLY_STATE_PATH", defaults.StatePath)
		workspace.SetStateRoot(defaults.StatePath)
		legacyworkspace.SetStateRoot(defaults.StatePath)
	}
	if strings.TrimSpace(os.Getenv("AGENTLY_DB_PATH")) == "" {
		dbPath := strings.TrimSpace(defaults.DBPath)
		if dbPath == "" {
			dbPath = filepath.Join(workspace.RuntimeRoot(), "db", "agently.db")
		}
		_ = os.Setenv("AGENTLY_DB_PATH", dbPath)
	}
}

func newRouter(api http.Handler, meta http.Handler, speech http.Handler, uiDist string, bundle servedUIBundle) http.Handler {
	embeddedServer := http.FileServer(http.FS(bundle.FS))
	localIndex := ""

	var local http.Handler
	if uiDist != "" {
		local = http.FileServer(http.Dir(uiDist))
		localIndex = filepath.Join(uiDist, "index.html")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/v1/api/agently/forge/") {
			http.StripPrefix("/v1/api/agently/forge", meta).ServeHTTP(w, r)
			return
		}
		if path == "/v1/api/speech/transcribe" {
			speech.ServeHTTP(w, r)
			return
		}
		if path == "/healthz" || path == "/health" || (strings.HasPrefix(path, "/v1/") && !strings.HasPrefix(path, "/v1/conversation/")) {
			api.ServeHTTP(w, r)
			return
		}

		if path == "/" || path == "/ui" || path == "/ui/" ||
			strings.HasPrefix(path, "/conversation/") ||
			strings.HasPrefix(path, "/ui/conversation/") ||
			strings.HasPrefix(path, "/v1/conversation/") {
			if localIndex != "" {
				http.ServeFile(w, r, localIndex)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write(bundle.Index)
			return
		}

		if local != nil {
			local.ServeHTTP(w, r)
			return
		}
		embeddedServer.ServeHTTP(w, r)
	})
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func defaultWorkspace() string {
	wd, err := os.Getwd()
	if err != nil {
		return ".agently"
	}
	return filepath.Join(wd, ".agently")
}

// runtimeExecutorAdapter bridges the agently-core Runtime to the
// mcpexpose.Executor interface for MCP tool exposure.
type runtimeExecutorAdapter struct {
	rt *executor.Runtime
}

type registryLLMCore struct {
	reg tool.Registry
}

func (r *registryLLMCore) ToolDefinitions() []llm.ToolDefinition {
	return r.reg.Definitions()
}

func (a *runtimeExecutorAdapter) LLMCore() mcpexpose.LLMCore {
	return &registryLLMCore{reg: a.rt.Registry}
}

func (a *runtimeExecutorAdapter) ExecuteTool(ctx context.Context, name string, args map[string]interface{}, _ int) (interface{}, error) {
	result, err := a.rt.Registry.Execute(ctx, name, args)
	return result, err
}

// loadMCPServerConfig reads mcpServer config from workspace config.yaml.
func loadMCPServerConfig(root string) *mcpexpose.ServerConfig {
	data, err := os.ReadFile(filepath.Join(root, "config.yaml"))
	if err != nil {
		return nil
	}
	var raw struct {
		MCPServer *mcpexpose.ServerConfig `yaml:"mcpServer"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil
	}
	return raw.MCPServer
}
