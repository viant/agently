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
	svcscheduler "github.com/viant/agently-core/service/scheduler"
	"github.com/viant/agently-core/workspace"
	"github.com/viant/agently/bootstrap"
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

	defaults := loadWorkspaceDefaults(workspace.Root())
	applyWorkspacePathDefaults(defaults)

	rt, _, _, err := newRuntime(ctx, defaults)
	if err != nil {
		return fmt.Errorf("failed to initialize runtime: %w", err)
	}

	scheduleStore, err := svcscheduler.NewDatlyStore(ctx, rt.DAO, rt.Data)
	if err != nil {
		return fmt.Errorf("failed to initialize scheduler store: %w", err)
	}
	schedulerSvc := svcscheduler.New(
		scheduleStore,
		rt.Agent,
		svcscheduler.WithConversationClient(rt.Conversation),
		svcscheduler.WithTokenProvider(rt.TokenProvider),
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
