package agently

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/viant/afs"
	"github.com/viant/agently/adapter/http/router"
	mcpclienthandler "github.com/viant/agently/adapter/mcp"
	convfactory "github.com/viant/agently/client/conversation/factory"
	"github.com/viant/agently/cmd/service"
	elicitationpkg "github.com/viant/agently/genai/elicitation"
	elicrouter "github.com/viant/agently/genai/elicitation/router"
	"github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/tool"
	authctx "github.com/viant/agently/internal/auth"
	integrate "github.com/viant/agently/internal/auth/mcp/integrate"
	mcpcookies "github.com/viant/agently/internal/mcp/cookies"
	mcpexpose "github.com/viant/agently/internal/mcp/expose"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
	"github.com/viant/agently/internal/workspace"
	mcprepo "github.com/viant/agently/internal/workspace/repository/mcp"
	protoclient "github.com/viant/mcp-protocol/client"
	authtransport "github.com/viant/mcp/client/auth/transport"
	"gopkg.in/yaml.v3"
)

// ServeCmd starts the embedded HTTP server.
// Usage: agently serve --addr :8080
type ServeCmd struct {
	Addr      string `short:"a" long:"addr" description:"listen address" default:":8080"`
	Policy    string `short:"p" long:"policy" description:"tool policy: auto|ask|deny" default:"auto"`
	ExposeMCP bool   `long:"expose-mcp" description:"Expose Agently tools over an MCP HTTP server (requires mcpServer.port and tool patterns in config)"`

	// Unified log file capturing LLM, tool and task events. Defaults to
	// "agently.log" in the current working directory when empty.
	Log string `long:"log" description:"unified log (LLM, TOOL, TASK)" default:"agently.log"`
}

func (s *ServeCmd) Execute(_ []string) error {
	applyRuntimeConfig()
	// Construct shared MCP router and per-conversation manager before executor init
	r := elicrouter.New()
	prov := mcpmgr.NewRepoProvider()

	// Build conversation client from environment (Datly), so MCP client can persist
	// elicitation messages/results for the Web UI to poll.
	convClient, err := convfactory.NewFromEnv(context.Background())
	if err != nil {
		return err
	}
	// Ensure per-conversation MCP clients are created with the Router wired in
	// Inject a workspace-driven refiner service for consistent UX across HTTP
	// and CLI. The default workspace preset refiner is used by the client when
	// no service is supplied; passing it explicitly keeps behaviour predictable
	// if future defaults change.
	// NOTE: For now, we rely on the internal default preset refiner; so no explicit WithRefinerService here.
	// Per-user shared CookieJar and auth RT so all conversations reuse BFF cookies
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

	mgr, err := mcpmgr.New(prov, mcpmgr.WithHandlerFactory(func() protoclient.Handler {
		el := elicitationpkg.New(convClient, nil, r, nil)
		return mcpclienthandler.NewClient(el, convClient, nil)
	}), mcpmgr.WithCookieJarProvider(jarProvider), mcpmgr.WithAuthRoundTripperProvider(authRTProvider))
	if err != nil {
		return fmt.Errorf("init mcp manager: %w", err)
	}
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

	// In server mode we rely solely on the HTTP approval flow; no console/stdin
	// prompts are wired up. CLI sub-commands (chat, run, …) still attach the
	// interactive approval loop when needed.

	// Start manager reaper to clean up idle MCP clients automatically.
	reapStop := mgr.StartReaper(context.Background(), 0) // interval defaults to ttl/2
	defer reapStop()

	handler, err := router.New(exec, svc, toolPol, r)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:    s.Addr,
		Handler: handler,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		errCh <- nil // server closed normally
	}()

	var (
		mcpSrv   *http.Server
		mcpErrCh chan error
	)
	if s.ExposeMCP {
		if cfg := exec.Config(); cfg != nil && cfg.MCPServer != nil && cfg.MCPServer.Enabled() {
			var err error
			mcpSrv, err = mcpexpose.NewHTTPServer(context.Background(), exec, cfg.MCPServer)
			if err != nil {
				return fmt.Errorf("init mcp server: %w", err)
			}
			mcpErrCh = make(chan error, 1)
			go func() {
				log.Printf("Agently MCP server listening on %s", mcpSrv.Addr)
				if err := mcpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					mcpErrCh <- err
				}
				mcpErrCh <- nil
			}()
		}
	}

	// Wait for termination signal or server error.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("Received %s, initiating graceful shutdown", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		_ = srv.Shutdown(ctx)
		if mcpSrv != nil {
			_ = mcpSrv.Shutdown(ctx)
		}
		exec.Shutdown(ctx)
		return nil
	case err := <-errCh:
		if mcpSrv != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = mcpSrv.Shutdown(ctx)
		}
		return err
	case err := <-mcpErrCh:
		if srv != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = srv.Shutdown(ctx)
		}
		return err
	}
}

func applyRuntimeConfig() {
	cfgPath := getConfigPath()
	if strings.TrimSpace(cfgPath) == "" {
		cfgPath = filepath.Join(workspace.Root(), "config.yaml")
	}
	fs := afs.New()
	data, err := fs.DownloadWithURL(context.Background(), cfgPath)
	if err != nil || len(data) == 0 {
		return
	}
	var cfg executor.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return
	}
	if strings.TrimSpace(cfg.Default.RuntimeRoot) != "" {
		workspace.SetRuntimeRoot(cfg.Default.RuntimeRoot)
	}
	if strings.TrimSpace(cfg.Default.StatePath) != "" {
		workspace.SetStateRoot(cfg.Default.StatePath)
	}
	if strings.TrimSpace(cfg.Default.DBPath) != "" {
		_ = os.Setenv("AGENTLY_DB_PATH", cfg.Default.DBPath)
	}
}
