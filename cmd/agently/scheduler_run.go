package agently

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	elicitationpkg "github.com/viant/agently/genai/elicitation"
	elicrouter "github.com/viant/agently/genai/elicitation/router"
	"github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/tool"
	iauth "github.com/viant/agently/internal/auth"
	chatsvc "github.com/viant/agently/internal/service/chat"
	convdao "github.com/viant/agently/internal/service/conversation"
	schsvc "github.com/viant/agently/internal/service/scheduler"
	schstore "github.com/viant/agently/internal/service/scheduler/store"
	fservice "github.com/viant/forge/backend/service/file"
)

// SchedulerRunCmd starts the scheduler watchdog loop as a dedicated process.
// Usage: agently scheduler run --interval 30s
type SchedulerRunCmd struct {
	Interval string `long:"interval" description:"RunDue polling interval (e.g. 30s, 1m)" default:"30s"`
	Once     bool   `long:"once" description:"Run one RunDue cycle and exit"`
}

func (s *SchedulerRunCmd) Execute(_ []string) error {
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
	chat.AttachFileService(fservice.New(os.TempDir()))

	store, err := schstore.New(context.Background(), dao)
	if err != nil {
		return err
	}
	orch, err := schsvc.New(store, chat)
	if err != nil {
		return err
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
