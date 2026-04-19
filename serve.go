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
	"sync"
	"syscall"
	"time"

	"github.com/viant/afs"
	_ "github.com/viant/afs/file"
	"github.com/viant/agently-core/adapter/http/ui"
	"github.com/viant/agently-core/app/executor"
	execconfig "github.com/viant/agently-core/app/executor/config"
	appserver "github.com/viant/agently-core/app/server"
	mcpexpose "github.com/viant/agently-core/protocol/mcp/expose"
	svcauthctx "github.com/viant/agently-core/service/auth"
	svcscheduler "github.com/viant/agently-core/service/scheduler"
	"github.com/viant/agently-core/workspace"
	wscfg "github.com/viant/agently-core/workspace/config"
	"github.com/viant/agently/bootstrap"
	deployui "github.com/viant/agently/deployment/ui"
	coremeta "github.com/viant/agently/metadata"
	agentlyrt "github.com/viant/agently/runtime"
	"github.com/viant/agently/server"
)

// shutdownTimeout bounds how long Serve will wait for in-flight HTTP and MCP
// handlers to finish on SIGTERM/SIGINT before returning. Past this deadline we
// stop waiting and return — a stuck handler should not block process exit.
const shutdownTimeout = 30 * time.Second

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

const (
	htmlCacheControl  = "no-cache, must-revalidate"
	assetCacheControl = "public, max-age=31536000, immutable"
)

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

	wsConfig, err := wscfg.Load(workspace.Root())
	if err != nil {
		return fmt.Errorf("failed to load workspace config: %w", err)
	}
	defaults := (&wscfg.Root{}).DefaultsWithFallback(&execconfig.Defaults{
		Model:    "openai_gpt-5.2",
		Embedder: "openai_text",
		Agent:    "chatter",
	})
	if wsConfig != nil {
		defaults = wsConfig.DefaultsWithFallback(defaults)
	}
	wscfg.ApplyPathDefaults(defaults)

	rt, client, agentFndr, err := appserver.BuildWorkspaceRuntime(ctx, appserver.RuntimeOptions{
		WorkspaceRoot: workspace.Root(),
		Defaults:      defaults,
		ConfigureRuntime: func(ctx context.Context, rt *executor.Runtime, workspaceRoot string) {
			agentlyrt.ConfigureRegistry(ctx, rt, workspaceRoot)
		},
	})
	if err != nil {
		return fmt.Errorf("failed to initialize runtime: %w", err)
	}

	authRuntime, err := svcauthctx.NewRuntime(ctx, workspace.Root(), rt.DAO)
	if err != nil {
		return fmt.Errorf("failed to initialize auth runtime: %w", err)
	}
	speechHandler := server.NewSpeechHandler()

	scheduleStore, err := svcscheduler.NewDatlyStore(ctx, rt.DAO, rt.Data)
	if err != nil {
		return fmt.Errorf("failed to initialize scheduler store: %w", err)
	}
	schedulerSvcOpts := []svcscheduler.Option{
		svcscheduler.WithConversationClient(rt.Conversation),
		svcscheduler.WithAuthConfig(rt.AuthConfig),
		svcscheduler.WithTokenProvider(rt.TokenProvider),
		svcscheduler.WithUserService(svcauthctx.NewDatlyUserService(rt.DAO)),
	}
	if cap := agentlyrt.SchedulerMaxConcurrentRunsFromEnv(); cap > 0 {
		schedulerSvcOpts = append(schedulerSvcOpts, svcscheduler.WithMaxConcurrentRuns(cap))
		log.Printf("scheduler: max concurrent runs capped at %d", cap)
	}
	schedulerSvc := svcscheduler.New(scheduleStore, rt.Agent, schedulerSvcOpts...)
	schedulerOpts := agentlyrt.SchedulerOptionsFromEnv()
	apiHandler, err := appserver.NewAPIHandler(ctx, appserver.APIOptions{
		Version:          firstNonEmpty(strings.TrimSpace(Version), "agently-v1"),
		Runtime:          rt,
		Client:           client,
		AgentFinder:      agentFndr,
		AgentIDs:         appserver.DiscoverWorkspaceAgentIDs(workspace.Root()),
		AuthRuntime:      authRuntime,
		SchedulerService: schedulerSvc,
		SchedulerOptions: schedulerOpts,
	})
	if err != nil {
		return fmt.Errorf("failed to create api handler: %w", err)
	}
	metaRoot := "embed://localhost/"
	metaHandler := ui.NewEmbeddedHandler(metaRoot, &coremeta.FS)
	uiBundle := servedUIBundle{Name: "v1", FS: deployui.FS, Index: deployui.Index}

	h := newRouter(apiHandler, metaHandler, speechHandler, uiDist, uiBundle)
	srv := &http.Server{Addr: addr, Handler: h}

	// Expose MCP server when explicitly requested or when workspace config
	// declares an MCP server port. Build before the shutdown goroutine starts
	// so shutdown can target both servers together.
	var mcpSrv *http.Server
	var mcpCfg *mcpexpose.ServerConfig
	if wsConfig != nil {
		mcpCfg = wsConfig.MCPServer
	}
	exposeMCP := options.ExposeMCP || (mcpCfg != nil && mcpCfg.Port > 0)
	if exposeMCP && mcpCfg != nil && mcpCfg.Port > 0 {
		mcpSrv, err = appserver.NewExposedMCPServer(ctx, rt, mcpCfg, authRuntime)
		if err != nil {
			return fmt.Errorf("init mcp server: %w", err)
		}
		go func() {
			log.Printf("Agently MCP server listening on %s", mcpSrv.Addr)
			if err := mcpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("MCP server error: %v", err)
			}
		}()
	}

	// Shutdown coordinator: on signal, drain the main server and the MCP
	// server in parallel with a bounded deadline so a stuck handler cannot
	// hang the process.
	var shutdownWG sync.WaitGroup
	shutdownWG.Add(1)
	go func() {
		defer shutdownWG.Done()
		<-ctx.Done()
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancelShutdown()
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
				log.Printf("http shutdown error: %v", err)
			}
		}()
		if mcpSrv != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := mcpSrv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
					log.Printf("mcp shutdown error: %v", err)
				}
			}()
		}
		wg.Wait()
	}()

	log.Printf("agently serve listening on %s (workspace=%s ui=%s)", addr, workspace.Root(), uiBundle.Name)
	serveErr := srv.ListenAndServe()
	// Whether the server returned from a graceful shutdown or an error, wait
	// for the coordinator goroutine so we don't return while Shutdown is
	// still draining handlers (and so MCP is guaranteed closed on error
	// paths too).
	shutdownWG.Wait()
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		if mcpSrv != nil {
			_ = mcpSrv.Close()
		}
		return fmt.Errorf("server error: %w", serveErr)
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
		if path == "/v1/api/auth/oauth/callback" && r.Method == http.MethodGet {
			w.Header().Set("Cache-Control", htmlCacheControl)
			if localIndex != "" {
				http.ServeFile(w, r, localIndex)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write(bundle.Index)
			return
		}
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

		if strings.HasPrefix(path, "/assets/") {
			w.Header().Set("Cache-Control", assetCacheControl)
			if local != nil {
				local.ServeHTTP(w, r)
				return
			}
			embeddedServer.ServeHTTP(w, r)
			return
		}

		if path == "/" || path == "/ui" || path == "/ui/" ||
			strings.HasPrefix(path, "/conversation/") ||
			strings.HasPrefix(path, "/ui/conversation/") ||
			strings.HasPrefix(path, "/v1/conversation/") {
			w.Header().Set("Cache-Control", htmlCacheControl)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
