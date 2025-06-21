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
	fluxpol "github.com/viant/fluxor/policy"
)

// ServeCmd starts the embedded HTTP server.
// Usage: agently serve --addr :8080
type ServeCmd struct {
	Addr            string `short:"a" long:"addr" description:"listen address" default:":8080"`
	Policy          string `short:"p" long:"policy" description:"tool policy: auto|ask|deny" default:"auto"`
	ConsoleApproval bool   `long:"console-approval" description:"enable interactive approval prompts on the server console (defaults to false)"`
}

func (s *ServeCmd) Execute(_ []string) error {
	exec := executorSingleton()
	if !exec.IsStarted() {
		exec.Start(context.Background())
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

	ctxBase := context.Background()
	ctxBase = tool.WithPolicy(ctxBase, toolPol)
	ctxBase = fluxpol.WithPolicy(ctxBase, fluxPol)

	// Launch stdin approval loop only when explicitly requested by the user.
	// The HTTP server exposes a web–based approval UI therefore, by default,
	// duplicate prompts on the console are suppressed to avoid confusion.
	var stop func()
	if s.ConsoleApproval {
		stop = startApprovalLoop(ctxBase, exec, fluxPol)
	} else {
		stop = func() {}
	}
	defer stop()

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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		exec.Shutdown(ctx)
		return nil
	case err := <-errCh:
		return err
	}
}
