package agently

import (
	"context"
	"fmt"
	"log"
	"net/http"
	neturl "net/url"
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
	mcpexpose "github.com/viant/agently/internal/mcp/expose"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
	"github.com/viant/agently/internal/workspace"
	mcprepo "github.com/viant/agently/internal/workspace/repository/mcp"
	protoclient "github.com/viant/mcp-protocol/client"
	authtransport "github.com/viant/mcp/client/auth/transport"
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
	// Per-user shared CookieJar and auth RT so all conversations reuse BFF cookies
	var (
		jarMu      sync.Mutex
		jarsByUser = map[string]http.CookieJar{}
	)
	jarProvider := func(ctx context.Context) (http.CookieJar, error) {
		user := strings.TrimSpace(authctx.EffectiveUserID(ctx))
		if user == "" {
			user = "anonymous"
		}
		jarMu.Lock()
		defer jarMu.Unlock()
		if j, ok := jarsByUser[user]; ok && j != nil {
			return j, nil
		}
		// Shared per-user file jar (BFF): state/mcp/bff/<user>
		sharedDir := filepath.Join(workspace.Root(), "state", "mcp", "bff", user)
		sharedPath := filepath.Join(sharedDir, "cookies.json")
		_ = os.MkdirAll(sharedDir, 0o700)
		fj, _ := authtransport.NewFileJar(sharedPath)
		if os.Getenv("AGENTLY_MCP_DEBUG") != "" {
			log.Printf("[mcp] using shared cookie jar user=%s path=%s", user, sharedPath)
		}
		// Warm from per-provider and shared anonymous jars
		repo := mcprepo.New(afs.New())
		// Shared anonymous
		anonSharedPath := filepath.Join(workspace.Root(), "state", "mcp", "bff", "anonymous", "cookies.json")
		if src, err := authtransport.NewFileJar(anonSharedPath); err == nil && src != nil {
			if names, err := repo.List(context.Background()); err == nil {
				for _, name := range names {
					if cfg, err := repo.Load(context.Background(), name); err == nil && cfg != nil && cfg.ClientOptions != nil {
						if raw := strings.TrimSpace(cfg.ClientOptions.Transport.URL); raw != "" {
							if u, perr := neturl.Parse(raw); perr == nil {
								if cs := src.Cookies(u); len(cs) > 0 {
									fj.SetCookies(u, cs)
									if os.Getenv("AGENTLY_MCP_DEBUG") != "" {
										log.Printf("[mcp] preloaded %d cookies for %s from %s", len(cs), u.Host, anonSharedPath)
									}
									host, port := u.Hostname(), u.Port()
									var alt string
									if host == "localhost" {
										alt = "127.0.0.1"
									} else if host == "127.0.0.1" {
										alt = "localhost"
									}
									if alt != "" {
										altURL := *u
										if port != "" {
											altURL.Host = alt + ":" + port
										} else {
											altURL.Host = alt
										}
										fj.SetCookies(&altURL, cs)
										if os.Getenv("AGENTLY_MCP_DEBUG") != "" {
											log.Printf("[mcp] mirrored %d cookies to %s for dev host alias", len(cs), altURL.Host)
										}
									}
								}
							}
						}
					}
				}
			}
		}
		// Per-provider user and anonymous
		if names, err := repo.List(context.Background()); err == nil {
			for _, name := range names {
				cfg, err := repo.Load(context.Background(), name)
				if err != nil || cfg == nil || cfg.ClientOptions == nil {
					continue
				}
				raw := strings.TrimSpace(cfg.ClientOptions.Transport.URL)
				if raw == "" {
					continue
				}
				u, perr := neturl.Parse(raw)
				if perr != nil {
					continue
				}
				for _, scope := range []string{user, "anonymous"} {
					stateDir := filepath.Join(workspace.Root(), "state", "mcp", name, scope)
					cookiesPath := filepath.Join(stateDir, "cookies.json")
					if src, jerr := authtransport.NewFileJar(cookiesPath); jerr == nil && src != nil {
						if cs := src.Cookies(u); len(cs) > 0 {
							fj.SetCookies(u, cs)
							if os.Getenv("AGENTLY_MCP_DEBUG") != "" {
								log.Printf("[mcp] preloaded %d cookies for %s from %s", len(cs), u.Host, cookiesPath)
							}
							host, port := u.Hostname(), u.Port()
							var alt string
							if host == "localhost" {
								alt = "127.0.0.1"
							} else if host == "127.0.0.1" {
								alt = "localhost"
							}
							if alt != "" {
								altURL := *u
								if port != "" {
									altURL.Host = alt + ":" + port
								} else {
									altURL.Host = alt
								}
								fj.SetCookies(&altURL, cs)
								if os.Getenv("AGENTLY_MCP_DEBUG") != "" {
									log.Printf("[mcp] mirrored %d cookies to %s for dev host alias", len(cs), altURL.Host)
								}
							}
						}
					}
				}
				if os.Getenv("AGENTLY_MCP_DEBUG") != "" {
					if cs := fj.Cookies(u); len(cs) > 0 {
						log.Printf("[mcp] shared jar now has %d cookies for %s (user=%s)", len(cs), u.Host, user)
					} else {
						log.Printf("[mcp] shared jar has 0 cookies for %s (user=%s)", u.Host, user)
					}
				}
			}
		}
		jarsByUser[user] = fj
		return fj, nil
	}
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
		j, _ := jarProvider(ctx)
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
		log.Printf("Agently HTTP server listening on %s", s.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		errCh <- nil // server closed normally
	}()

	var (
		mcpSrv   *http.Server
		mcpErrCh chan error
	)
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
