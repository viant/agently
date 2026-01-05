package router

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/viant/afs"
	fsadapter "github.com/viant/afs/adapter/http"
	_ "github.com/viant/afs/file"
	chatserver "github.com/viant/agently/adapter/http"
	authhttp "github.com/viant/agently/adapter/http/auth"
	"github.com/viant/agently/adapter/http/filebrowser"
	schedulerhttp "github.com/viant/agently/adapter/http/scheduler"
	"github.com/viant/agently/adapter/http/speech"
	toolhttp "github.com/viant/agently/adapter/http/tool"
	"github.com/viant/agently/adapter/http/workflow"
	schapi "github.com/viant/agently/client/scheduler"
	schstorecli "github.com/viant/agently/client/scheduler/store"
	"github.com/viant/agently/deployment/ui"
	elicrouter "github.com/viant/agently/genai/elicitation/router"
	schsvc "github.com/viant/agently/internal/service/scheduler"
	schstore "github.com/viant/agently/internal/service/scheduler/store"
	mcpname "github.com/viant/agently/pkg/mcpname"

	mw "github.com/viant/agently/adapter/http/middleware"
	"github.com/viant/agently/adapter/http/router/metadata"
	userpref "github.com/viant/agently/adapter/http/userpref"
	"github.com/viant/agently/adapter/http/workspace"
	"github.com/viant/agently/cmd/service"
	execsvc "github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/tool"
	iauth "github.com/viant/agently/internal/auth"
	chatsvc "github.com/viant/agently/internal/service/chat"
	convdao "github.com/viant/agently/internal/service/conversation"
	invk "github.com/viant/agently/pkg/agently/tool/invoker"
	oauthread "github.com/viant/agently/pkg/agently/user/oauth"
	oauthwrite "github.com/viant/agently/pkg/agently/user/oauth/write"
	fhandlers "github.com/viant/forge/backend/handlers"
	fservice "github.com/viant/forge/backend/service/file"
)

func isSchedulerAPIEnabled() bool {
	v := strings.TrimSpace(os.Getenv("AGENTLY_SCHEDULER_API"))
	switch strings.ToLower(v) {
	case "0", "false", "no", "n", "off":
		return false
	default:
		return true
	}
}

func isSchedulerWatchdogEnabled() bool {
	v := strings.TrimSpace(os.Getenv("AGENTLY_SCHEDULER_RUNNER"))
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func schedulerWatchdogInterval() time.Duration {
	v := strings.TrimSpace(os.Getenv("AGENTLY_SCHEDULER_INTERVAL"))
	if v == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 30 * time.Second
	}
	return d
}

func isSchedulerRunNowEnabled() bool {
	v := strings.TrimSpace(os.Getenv("AGENTLY_SCHEDULER_RUN_NOW"))
	switch strings.ToLower(v) {
	case "0", "false", "no", "n", "off":
		return false
	case "1", "true", "yes", "y", "on":
		return true
	default:
		// Preserve backward compatibility: keep run-now enabled unless explicitly disabled.
		return true
	}
}

// execInvoker provides a conversation-scoped invoker backed by executor.
type execInvoker struct{ exec *execsvc.Service }

var _ invk.Invoker = (*execInvoker)(nil)

// Invoke executes a tool by service/method in the current conversation context.
func (e *execInvoker) Invoke(ctx context.Context, service, method string, args map[string]interface{}) (interface{}, error) {
	name := service
	if method != "" {
		name = service + "/" + method
	}
	cName := mcpname.Canonical(name)
	return e.exec.ExecuteTool(ctx, cName, args, 0)
}

// New constructs an http.Handler that combines chat API and workspace CRUD API.
//
// Chat endpoints are mounted under /v1/api/… (see adapter/http/server.go).
// Workspace endpoints under /v1/workspace/… (see adapter/http/workspace).
func New(exec *execsvc.Service, svc *service.Service, toolPol *tool.Policy, mcpR elicrouter.ElicitationRouter) (http.Handler, error) {
	mux := http.NewServeMux()

	// Forge file service singleton (reused for upload handlers and chat service)
	fs := fservice.New(os.TempDir())

	cfg := exec.Config()
	// Build chat service with env and propagate errors to caller to prevent start
	chatSvc, err := chatsvc.NewServiceFromEnv(context.Background())
	if err != nil {
		return nil, err
	}

	// defer chat server mount until after we build DAO and authCfg
	mux.Handle("/v1/workspace/", workspace.NewHandler(svc))
	// Backward-compatible alias so callers using /v1/api/workspace/* keep working
	mux.Handle("/v1/api/workspace/", http.StripPrefix("/v1/api/", workspace.NewHandler(svc)))

	// Workflow run endpoint

	// Single datly initialization for http handlers needing DAO
	dao, err := convdao.NewDatly(context.Background())
	if err != nil {
		return nil, err
	}

	mux.Handle("/v1/api/workflow/run", workflow.New(exec, svc))

	// Auth endpoints (local login, me, logout) – using shared DAO and shared session manager
	// Use auth config from workspace config (single source of truth)
	authCfg := cfg.Auth
	if authCfg == nil {
		authCfg = &iauth.Config{}
	}
	sess := iauth.NewManager(authCfg)
	if ah, err := authhttp.NewWithDatlyAndConfigExt(dao, sess, authCfg, cfg.Default.Model, cfg.Default.Agent, cfg.Default.Embedder); err == nil {
		mux.Handle("/v1/api/auth/", ah)
	}

	// Mount chat API now that dao/authCfg are available
	mux.Handle("/v1/api/", chatserver.NewServer(exec.Conversation(),
		chatserver.WithPolicies(toolPol),
		chatserver.WithApprovalService(exec.ApprovalService()),
		chatserver.WithFileService(fs),
		chatserver.WithMCPRouter(mcpR),
		chatserver.WithAgentFinder(exec.AgentService().Finder()),
		chatserver.WithCore(exec.LLMCore()),
		chatserver.WithDefaults(&cfg.Default),
		chatserver.WithInvoker(&execInvoker{exec: exec}),
		chatserver.WithChatService(chatSvc),
		chatserver.WithAuthConfig(authCfg),
		chatserver.WithDAO(dao),
	))

	var (
		schedulerStore schstorecli.Client
		schedulerOrch  schapi.Client
	)
	if isSchedulerAPIEnabled() || isSchedulerWatchdogEnabled() {
		// Scheduler store + orchestration service (reuses the shared chat service)
		client, err := schstore.New(context.Background(), dao)
		if err != nil {
			return nil, err
		}
		orch, err := schsvc.New(client, chatSvc)
		if err != nil {
			return nil, err
		}
		schedulerStore = client
		schedulerOrch = orch
	}

	if isSchedulerAPIEnabled() {
		sch, err := schedulerhttp.NewHandler(dao, schedulerStore, schedulerOrch)
		if err != nil {
			return nil, err
		}
		registerSchedulerRoutes(mux, sch, isSchedulerRunNowEnabled())
	}

	// Start schedule watchdog only when explicitly enabled.
	// This prevents duplicate schedule runs in horizontally scaled/serverless deployments.
	if isSchedulerWatchdogEnabled() && schedulerOrch != nil {
		interval := schedulerWatchdogInterval()
		wdCtx := context.Background()
		_ = *schsvc.StartWatchdog(wdCtx, schedulerOrch, interval)
	}

	// OAuth token components (read + write) – used by token store via dao.Operate only.
	if err := oauthread.DefineTokenComponent(context.Background(), dao); err != nil {
		return nil, err
	}
	if _, err := oauthwrite.DefineComponent(context.Background(), dao); err != nil {
		return nil, err
	}

	// User preferences endpoint (/v1/api/me/preferences)
	if uph, err := userpref.Handler(); err == nil {
		mux.Handle("/v1/api/me/preferences", uph)
	}

	// Admin-only token list (provider + updated_at), guarded by AGENTLY_ADMINS
	// Admin list disabled for now to simplify build; can be re-enabled with ops guard

	// Ad-hoc tool execution
	mux.Handle("/v1/api/tools/", http.StripPrefix("/v1/api/tools", toolhttp.New(svc)))

	// Speech-to-text
	mux.HandleFunc("/v1/api/speech/transcribe", speech.NewHandler())

	// File browser (Forge)
	mux.Handle("/v1/workspace/file-browser/", http.StripPrefix("/v1/workspace/file-browser", filebrowser.New()))

	// Preferred path
	mux.HandleFunc("/v1/workspace/metadata", metadata.NewAgently(exec))

	// MCP init/discovery endpoint: initialise client and return tools/prompts/resources

	// Forge file upload/list/download endpoints for chat attachments
	mux.HandleFunc("/upload", fhandlers.UploadHandler(fs))
	fb := fhandlers.NewFileBrowser(fs)
	mux.HandleFunc("/download", fb.DownloadHandler)
	mux.HandleFunc("/list", fb.ListHandler)

	fileSystem := fsadapter.New(afs.New(), "embed://localhost", &ui.FS)
	fileServer := http.FileServer(fileSystem)

	// static content
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			// Do not call WriteHeader after Write; default status is 200
			if _, err := w.Write(ui.Index); err != nil {
				// Unable to write response; nothing else to do in handler
				return
			}
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	mux.HandleFunc("/.well-known/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Kick off background sync that surfaces approval requests as chat messages
	ctx := context.Background()
	chatserver.StartApprovalBridge(ctx, exec, exec.Conversation())

	// Wrap with Protect middleware when auth enabled
	// Attach auth config to request context for middleware claim checks
	handlerWithCtx := mw.WithAuthConfig(mux, authCfg)
	protected := mw.Protect(authCfg, sess)(handlerWithCtx)
	return chatserver.WithCORS(protected), nil
}

// newStore removed — chat service now uses conversation client directly

// registerSchedulerRoutes mounts all scheduler-related endpoints using a single handler.
// This avoids duplicating registration code and ensures patterns remain consistent.
func registerSchedulerRoutes(mux *http.ServeMux, h http.Handler, runNowEnabled bool) {
	mux.Handle("/v1/api/agently/scheduler/", h)
	mux.Handle("/v1/api/agently/schedule", h)
	mux.Handle("/v1/api/agently/schedule-run", h)
	if runNowEnabled {
		mux.Handle("/v1/api/agently/scheduler/run-now/", h)
		mux.Handle("/v1/api/agently/schedule-run-now", h)
	}
}
