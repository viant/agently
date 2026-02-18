package agently

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/viant/afs"
	mcpclienthandler "github.com/viant/agently/adapter/mcp"
	elicitationpkg "github.com/viant/agently/genai/elicitation"
	elicrouter "github.com/viant/agently/genai/elicitation/router"
	"github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/tool"
	authctx "github.com/viant/agently/internal/auth"
	iauth "github.com/viant/agently/internal/auth"
	integrate "github.com/viant/agently/internal/auth/mcp/integrate"
	mcpcookies "github.com/viant/agently/internal/mcp/cookies"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
	chatsvc "github.com/viant/agently/internal/service/chat"
	convdao "github.com/viant/agently/internal/service/conversation"
	schsvc "github.com/viant/agently/internal/service/scheduler"
	schstore "github.com/viant/agently/internal/service/scheduler/store"
	"github.com/viant/agently/internal/workspace"
	mcprepo "github.com/viant/agently/internal/workspace/repository/mcp"
	fservice "github.com/viant/forge/backend/service/file"
	protoclient "github.com/viant/mcp-protocol/client"
	authtransport "github.com/viant/mcp/client/auth/transport"
)

// SchedulerRunCmd starts the scheduler watchdog loop as a dedicated process.
// Usage: agently scheduler run --interval 30s
type SchedulerRunCmd struct {
	Interval string `long:"interval" description:"RunDue polling interval (e.g. 30s, 1m)" default:"30s"`
	Once     bool   `long:"once" description:"Run one RunDue cycle and exit"`
}

func (s *SchedulerRunCmd) Execute(_ []string) error {
	applyRuntimeConfig()
	interval := 30 * time.Second
	if v := s.Interval; v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid --interval: %w", err)
		}
		if parsed > 0 {
			interval = parsed
		}
	}

	// Scheduler runner is headless: use router-based elicitation + no-op awaiter (never blocks on stdin).
	r := elicrouter.New()
	registerExecOption(executor.WithElicitationRouter(r))
	registerExecOption(executor.WithNewElicitationAwaiter(elicitationpkg.NoopAwaiterFactory()))

	// Shared DAO and conversation client for persistence.
	dao, err := convdao.NewDatly(context.Background())
	if err != nil {
		return err
	}
	convClient, err := convdao.New(context.Background(), dao)
	if err != nil {
		return err
	}
	if convClient != nil {
		registerExecOption(executor.WithConversionClient(convClient))
	}

	// Wire MCP manager so scheduled runs can discover and call MCP tools.
	prov := mcpmgr.NewRepoProvider()
	fs := afs.New()
	repo := mcprepo.New(fs)
	cookieProvider := mcpcookies.New(fs, repo)
	jarProvider := cookieProvider.Jar
	var (
		rtMu     sync.Mutex
		rtByUser = map[string]*authtransport.RoundTripper{}
	)
	authRTProvider := func(ctx context.Context) *authtransport.RoundTripper {
		user := strings.TrimSpace(authctx.EffectiveUserID(ctx))
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
		rt, _ := integrate.NewAuthRoundTripperWithPrompt(j, http.DefaultTransport, 0, nil)
		rtByUser[user] = rt
		return rt
	}

	mgr, err := mcpmgr.New(
		prov,
		mcpmgr.WithHandlerFactory(func() protoclient.Handler {
			el := elicitationpkg.New(convClient, nil, r, nil)
			return mcpclienthandler.NewClient(el, convClient, nil)
		}),
		mcpmgr.WithCookieJarProvider(jarProvider),
		mcpmgr.WithAuthRoundTripperProvider(authRTProvider),
	)
	if err != nil {
		return fmt.Errorf("init mcp manager: %w", err)
	}
	stopReap := mgr.StartReaper(context.Background(), 0)
	defer stopReap()
	registerExecOption(executor.WithMCPManager(mgr))

	exec := executorSingleton()
	if !exec.IsStarted() {
		exec.Start(context.Background())
	}
	baseCtx := context.Background()
	if cfg := exec.Config(); cfg != nil {
		baseCtx = iauth.EnsureUser(baseCtx, cfg.Auth)
	}

	// Chat service that actually executes queued turns.
	chat := chatsvc.NewServiceWithClient(convClient, dao)
	chat.AttachManager(exec.Conversation(), &tool.Policy{Mode: tool.ModeAuto})
	chat.AttachFileService(fservice.New(workspace.Root()))

	store, err := schstore.New(context.Background(), dao)
	if err != nil {
		return err
	}
	orch, err := schsvc.New(store, chat)
	if err != nil {
		return err
	}
	if cfg := exec.Config(); cfg != nil {
		if svc, ok := orch.(*schsvc.Service); ok {
			svc.AttachAuthConfig(cfg.Auth)
		}
	}

	if s.Once {
		_, err := orch.RunDue(baseCtx)
		return err
	}

	wd := schsvc.StartWatchdog(baseCtx, orch, interval)
	if wd == nil {
		return fmt.Errorf("failed to start scheduler watchdog")
	}

	// Wait for termination signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	wd.Stop()
	exec.Shutdown(context.Background())
	return nil
}
