package agently

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/viant/agently/adapter/http/router"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/service"

	// Logging & workflow listener
	"github.com/viant/agently/genai/executor"
	elog "github.com/viant/agently/internal/log"
	"github.com/viant/fluxor"
	fluxexec "github.com/viant/fluxor/service/executor"
)

// ServeCmd starts the embedded HTTP server.
// Usage: agently serve --addr :8080
type ServeCmd struct {
	Addr   string `short:"a" long:"addr" description:"listen address" default:":8080"`
	Policy string `short:"p" long:"policy" description:"tool policy: auto|ask|deny" default:"auto"`

	// Unified log file capturing LLM, tool and task events. Defaults to
	// "agently.log" in the current working directory when empty.
	Log string `long:"log" description:"unified log (LLM, TOOL, TASK)" default:"agently.log"`
}

func (s *ServeCmd) Execute(_ []string) error {
	exec := executorSingleton()
	if !exec.IsStarted() {
		exec.Start(context.Background())
	}

	// ------------------------------------------------------------------
	// Optional unified log writer (agently.log by default)
	// ------------------------------------------------------------------

	var logWriter *os.File
	if s.Log == "" {
		s.Log = "agently.log"
	}
	if lf, err := os.OpenFile(s.Log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		logWriter = lf
		// Subscribe to relevant event types.
		elog.FileSink(logWriter, elog.LLMInput, elog.LLMOutput, elog.TaskInput, elog.TaskOutput)

		// Attach Fluxor task listener so that each task is encoded as JSON to
		// the same sink.
		registerExecOption(executor.WithWorkflowOptions(
			fluxor.WithExecutorOptions(
				fluxexec.WithListener(newJSONTaskListener()),
			),
		))
	} else {
		// Logging is best-effort; continue if the file cannot be opened.
		log.Printf("warning: unable to open log file %s: %v", s.Log, err)
	}

	svc := service.New(exec, service.Options{})

	// Build policies based on flag.
	// Build tool policy. For web server we never use the stdin/console "Ask"
	// helper as approvals are handled through the HTTP UI. Therefore even in
	// ask mode we leave Policy.Ask unset so that no interactive prompt is
	// triggered on the server terminal.
	toolPol := buildPolicy(s.Policy)
	if strings.ToLower(toolPol.Mode) == tool.ModeAsk {
		toolPol.Ask = nil
	}
	fluxPol := buildFluxorPolicy(s.Policy)

	// In server mode we rely solely on the HTTP approval flow; no console/stdin
	// prompts are wired up. CLI sub-commands (chat, run, …) still attach the
	// interactive approval loop when needed.

	handler := router.New(exec, svc, toolPol, fluxPol)

	srv := &http.Server{
		Addr:    s.Addr,
		Handler: handler,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("Agently HTTP server listening on %s", s.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		errCh <- nil // server closed normally
	}()

	// Wait for termination signal or server error.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("Received %s, initiating graceful shutdown", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		exec.Shutdown(ctx)
		return nil
	case err := <-errCh:
		return err
	}
}
