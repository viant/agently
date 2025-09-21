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
	mcpclienthandler "github.com/viant/agently/adapter/mcp"
	mcpmgr "github.com/viant/agently/adapter/mcp/manager"
	mcprouter "github.com/viant/agently/adapter/mcp/router"
	"github.com/viant/agently/cmd/service"
	"github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/tool"
	protoclient "github.com/viant/mcp-protocol/client"
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
	// Construct shared MCP router and per-conversation manager before executor init
	r := mcprouter.New()
	prov := mcpmgr.NewRepoProvider()
	// Ensure per-conversation MCP clients are created with the Router wired in
	mgr := mcpmgr.New(prov, mcpmgr.WithHandlerFactory(func() protoclient.Handler {
		return mcpclienthandler.NewClient(mcpclienthandler.WithRouter(r))
	}))
	// Inject manager into executor options so tool registry can use it
	registerExecOption(executor.WithMCPManager(mgr))

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

	// In server mode we rely solely on the HTTP approval flow; no console/stdin
	// prompts are wired up. CLI sub-commands (chat, run, …) still attach the
	// interactive approval loop when needed.

	// Start manager reaper to clean up idle MCP clients automatically.
	reapStop := mgr.StartReaper(context.Background(), 0) // interval defaults to ttl/2
	defer reapStop()

	handler := router.New(exec, svc, toolPol, fluxPol, r)

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
