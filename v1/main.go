package v1

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
	provider "github.com/viant/agently-core/genai/llm/provider"
	agentfinder "github.com/viant/agently-core/protocol/agent/finder"
	agentloader "github.com/viant/agently-core/protocol/agent/loader"
	"github.com/viant/agently-core/sdk"
	svcauthctx "github.com/viant/agently-core/service/auth"
	svcscheduler "github.com/viant/agently-core/service/scheduler"
	svcworkspace "github.com/viant/agently-core/service/workspace"
	"github.com/viant/agently-core/workspace"
	embedderloader "github.com/viant/agently-core/workspace/loader/embedder"
	wsfs "github.com/viant/agently-core/workspace/loader/fs"
	modelloader "github.com/viant/agently-core/workspace/loader/model"
	"github.com/viant/agently-core/workspace/service/meta"
	deployui "github.com/viant/agently/deployment/ui/v1"
	integrate "github.com/viant/agently/internal/auth/mcp/integrate"
	mcpcookies "github.com/viant/agently/internal/mcp/cookies"
	mcprepo "github.com/viant/agently/internal/workspace/repository/mcp"
	coremeta "github.com/viant/agently/v1/metadata"
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
}

func Serve(options ServeOptions) error {
	addr := envOr("AGENTLY_ADDR", ":8585")
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
	setBootstrapHook()
	workspace.EnsureDefault(afs.New())
	defer workspace.SetBootstrapHook(nil)

	rt, client, err := newRuntime(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize runtime: %w", err)
	}

	authRuntime, err := newAuthRuntime(ctx, workspace.Root(), rt)
	if err != nil {
		return fmt.Errorf("failed to initialize auth runtime: %w", err)
	}
	speechHandler := newSpeechHandler()

	metadataHandler := svcworkspace.NewMetadataHandler(rt.Defaults, rt.Store, "agently-v1").
		SetStarterTasks(defaultStarterTasks())
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
	schedulerOpts := schedulerOptionsFromEnv()
	if schedulerOpts != nil { // TODO delete
		//TODO
	}
	sdkHandler, err := sdk.NewHandlerWithContext(ctx, client,
		sdk.WithMetadataHandler(metadataHandler),
		sdk.WithFileBrowser(fileBrowserHandler),
		// sdk.WithScheduler(schedulerSvc, schedulerHandler, schedulerOpts), // TODO uncomment
		// TODO replace with line above when works
		sdk.WithScheduler(schedulerSvc, schedulerHandler, &sdk.SchedulerOptions{
			EnableAPI:      true,
			EnableRunNow:   true,
			EnableWatchdog: true,
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create sdk handler: %w", err)
	}
	apiHandler := withAuthExtensions(sdkHandler, authRuntime)
	metaRoot := "embed://localhost/"
	metaHandler := ui.NewEmbeddedHandler(metaRoot, &coremeta.FS)
	uiBundle := servedUIBundle{Name: "v1", FS: deployui.FS, Index: deployui.Index}

	h := newRouter(apiHandler, metaHandler, speechHandler, uiDist, uiBundle)
	srv := &http.Server{Addr: addr, Handler: h}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	log.Printf("agently v1 serve listening on %s (workspace=%s ui=%s)", addr, workspace.Root(), uiBundle.Name)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

func newRuntime(ctx context.Context) (*executor.Runtime, sdk.Client, error) {
	fs := afs.New()
	wsMeta := meta.New(fs, workspace.Root())
	agentLdr := agentloader.New(agentloader.WithMetaService(wsMeta))
	agentFndr := agentfinder.New(agentfinder.WithLoader(agentLdr))
	modelLdr := modelloader.New(wsfs.WithMetaService[provider.Config](wsMeta))
	modelFndr := newModelFinder(modelLdr)
	embedderLdr := embedderloader.New(wsfs.WithMetaService[embedprovider.Config](wsMeta))
	embedderFndr := newEmbedderFinder(embedderLdr)
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
		WithDefaults(loadWorkspaceDefaults(workspace.Root())).
		WithMCPAuthRTProvider(authRTProvider).
		WithMCPCookieJarProvider(jarProvider).
		WithMCPUserIDExtractor(func(ctx context.Context) string {
			return strings.TrimSpace(svcauthctx.EffectiveUserID(ctx))
		}).
		Build(ctx)
	if err != nil {
		return nil, nil, err
	}
	configureRegistry(ctx, rt, workspace.Root())
	client, err := sdk.NewEmbeddedFromRuntime(rt)
	if err != nil {
		return nil, nil, err
	}
	// Wire the lazy reference so auth elicitations use the proper DB-backed flow.
	embeddedClient = client
	return rt, client, nil
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
		Default struct {
			Agent    string `yaml:"agent"`
			Model    string `yaml:"model"`
			Embedder string `yaml:"embedder"`
		} `yaml:"default"`
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
	return fallback
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
		if path == "/healthz" || (strings.HasPrefix(path, "/v1/") && !strings.HasPrefix(path, "/v1/conversation/")) {
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
