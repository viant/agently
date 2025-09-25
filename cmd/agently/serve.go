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
	convfactory "github.com/viant/agently/client/conversation/factory"
	"github.com/viant/agently/cmd/service"
	elicitationpkg "github.com/viant/agently/genai/elicitation"
	elicrouter "github.com/viant/agently/genai/elicitation/router"
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
	r := elicrouter.New()
	prov := mcpmgr.NewRepoProvider()

	// Build conversation client from environment (Datly), so MCP client can persist
	// elicitation messages/results for the Web UI to poll.
	convClient, _ := convfactory.NewFromEnv(context.Background())
	// Ensure per-conversation MCP clients are created with the Router wired in
	// Inject a workspace-driven refiner service for consistent UX across HTTP
	// and CLI. The default workspace preset refiner is used by the client when
	// no service is supplied; passing it explicitly keeps behaviour predictable
	// if future defaults change.
	// NOTE: For now, we rely on the internal default preset refiner; so no explicit WithRefinerService here.
	mgr := mcpmgr.New(prov, mcpmgr.WithHandlerFactory(func() protoclient.Handler {
		el := elicitationpkg.New(convClient, nil, r, nil)
		return mcpclienthandler.NewClient(el, convClient, nil)
	}))
	// Inject manager into executor options so tool registry can use it
	registerExecOption(executor.WithMCPManager(mgr))
	// Share the same elicitation router with the agent so LLM-originated
	// elicitations block and resume via the same channel as tool elicitations.
	registerExecOption(executor.WithElicitationRouter(r))
	// Ensure awaiterFactory is non-nil to satisfy runtime requirements; server uses UI, so no-op awaiter.
	registerExecOption(executor.WithNewElicitationAwaiter(elicitationpkg.NoopAwaiterFactory()))
	// Also supply conversation client to executor so orchestration can persist messages/tool events.
	if convClient != nil {
		registerExecOption(executor.WithConversionClient(convClient))
	}

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
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		_ = srv.Shutdown(ctx)
		exec.Shutdown(ctx)
		return nil
	case err := <-errCh:
		return err
	}
}
