package agently

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	Addr   string `short:"a" long:"addr" description:"listen address" default:":8080"`
	Policy string `short:"p" long:"policy" description:"tool policy: auto|ask|deny" default:"auto"`
}

func (s *ServeCmd) Execute(_ []string) error {
	exec := executorSingleton()
	if !exec.IsStarted() {
		exec.Start(context.Background())
	}

	svc := service.New(exec, service.Options{})

	// Build policies based on flag.
	toolPol := buildPolicy(s.Policy)
	fluxPol := buildFluxorPolicy(s.Policy)

	ctxBase := context.Background()
	ctxBase = tool.WithPolicy(ctxBase, toolPol)
	ctxBase = fluxpol.WithPolicy(ctxBase, fluxPol)

	// Launch stdin approval loop when ask mode (for server console decisions)
	stop := startApprovalLoop(ctxBase, exec, fluxPol)
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
