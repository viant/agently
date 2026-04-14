package agently

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/viant/afs"
	_ "github.com/viant/afs/file"
	"github.com/viant/agently-core/app/executor"
	execconfig "github.com/viant/agently-core/app/executor/config"
	appserver "github.com/viant/agently-core/app/server"
	svcauth "github.com/viant/agently-core/service/auth"
	svcscheduler "github.com/viant/agently-core/service/scheduler"
	"github.com/viant/agently-core/workspace"
	wscfg "github.com/viant/agently-core/workspace/config"
	"github.com/viant/agently/bootstrap"
	agentlyrt "github.com/viant/agently/runtime"
)

type SchedulerRunOptions struct {
	Interval time.Duration
	Once     bool
}

func RunScheduler(options SchedulerRunOptions) error {
	interval := options.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if strings.EqualFold(strings.TrimSpace(os.Getenv("AGENTLY_DEBUG")), "1") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("AGENTLY_DEBUG")), "true") {
		enableDebugLogging()
	}

	workspacePath := envOr("AGENTLY_WORKSPACE", defaultWorkspace())
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

	rt, _, _, err := appserver.BuildWorkspaceRuntime(ctx, appserver.RuntimeOptions{
		WorkspaceRoot:     workspace.Root(),
		Defaults:          defaults,
		SchedulerHeadless: true,
		ConfigureRuntime: func(ctx context.Context, rt *executor.Runtime, workspaceRoot string) {
			agentlyrt.ConfigureRegistry(ctx, rt, workspaceRoot)
		},
	})
	if err != nil {
		return fmt.Errorf("failed to initialize runtime: %w", err)
	}
	authCfg, err := svcauth.LoadWorkspaceConfig(workspace.Root())
	if err != nil {
		return fmt.Errorf("failed to load workspace auth config: %w", err)
	}
	var userCredAuthCfg *svcscheduler.UserCredAuthConfig
	if authCfg != nil && authCfg.OAuth != nil && authCfg.OAuth.Client != nil {
		userCredAuthCfg = &svcscheduler.UserCredAuthConfig{
			Mode:            strings.TrimSpace(authCfg.OAuth.Mode),
			ClientConfigURL: strings.TrimSpace(authCfg.OAuth.Client.ConfigURL),
			Scopes:          append([]string(nil), authCfg.OAuth.Client.Scopes...),
		}
	}
	tokenProvider := rt.TokenProvider
	if tokenProvider == nil {
		tokenProvider = svcauth.NewCreatedByUserTokenProvider(authCfg, rt.DAO)
	}

	scheduleStore, err := svcscheduler.NewDatlyStore(ctx, rt.DAO, rt.Data)
	if err != nil {
		return fmt.Errorf("failed to initialize scheduler store: %w", err)
	}
	schedulerSvc := svcscheduler.New(
		scheduleStore,
		rt.Agent,
		svcscheduler.WithConversationClient(rt.Conversation),
		svcscheduler.WithAuthConfig(authCfg),
		svcscheduler.WithTokenProvider(tokenProvider),
		svcscheduler.WithUserService(svcauth.NewDatlyUserService(rt.DAO)),
		svcscheduler.WithUserCredAuthConfig(userCredAuthCfg),
		svcscheduler.WithInterval(interval),
	)

	if options.Once {
		_, err = schedulerSvc.RunDue(ctx)
		return err
	}

	log.Printf("agently scheduler run started (workspace=%s interval=%s)", workspace.Root(), interval)
	schedulerSvc.StartWatchdog(ctx)
	return nil
}
