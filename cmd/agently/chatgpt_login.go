package agently

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/viant/agently/internal/genai/provider/openai/chatgptauth"
)

type ChatGPTLoginCmd struct {
	ClientURL          string `long:"clientURL" description:"scy resource for OAuth client config (must contain client_id, optional client_secret)"`
	TokensURL          string `long:"tokensURL" description:"scy resource where ChatGPT OAuth token state will be stored (default: derived from clientURL)"`
	Issuer             string `long:"issuer" description:"OAuth issuer base URL (default: from client config, else https://auth.openai.com)"`
	AllowedWorkspaceID string `long:"allowedWorkspaceID" description:"restrict login to a specific ChatGPT workspace/account id"`
	Originator         string `long:"originator" description:"originator query param used by OpenAI auth (default: codex_cli_rs)" default:"codex_cli_rs"`
	Port               int    `long:"port" description:"local callback server port (must match OAuth redirect allowlist; default: 1455)" default:"1455"`

	NoOpenBrowser bool `long:"no-open-browser" description:"do not open the authorization URL in the default browser"`
	NoMintAPIKey  bool `long:"no-mint-api-key" description:"do not mint and cache an OpenAI API key after login"`
	RequireMint   bool `long:"require-mint-api-key" description:"fail the command if API key minting fails"`
	TimeoutSec    int  `long:"timeout" description:"callback wait timeout in seconds" default:"300"`
}

func (c *ChatGPTLoginCmd) Execute(_ []string) error {
	if strings.TrimSpace(c.ClientURL) == "" {
		return fmt.Errorf("--clientURL is required")
	}
	if strings.TrimSpace(c.TokensURL) == "" {
		derived, err := deriveTokensURLFromClientURL(c.ClientURL)
		if err != nil {
			return err
		}
		c.TokensURL = derived
		fmt.Printf("Using derived tokensURL: %s\n", c.TokensURL)
	}

	codeVerifier, err := randomToken(32)
	if err != nil {
		return err
	}
	state, err := randomToken(32)
	if err != nil {
		return err
	}

	endpoint, err := newIPv4CallbackEndpoint(state, timeoutFromSeconds(c.TimeoutSec), c.Port)
	if err != nil {
		return err
	}
	defer endpoint.Close()

	manager, err := chatgptauth.NewManager(
		&chatgptauth.Options{
			ClientURL:          c.ClientURL,
			TokensURL:          c.TokensURL,
			Issuer:             c.Issuer,
			AllowedWorkspaceID: c.AllowedWorkspaceID,
			Originator:         c.Originator,
		},
		chatgptauth.NewScyOAuthClientLoader(c.ClientURL),
		chatgptauth.NewScyTokenStateStore(c.TokensURL),
		&http.Client{Timeout: 60 * time.Second},
	)
	if err != nil {
		return err
	}

	ctx := context.Background()
	authURL, err := manager.BuildAuthorizeURL(ctx, endpoint.RedirectURL(), state, codeVerifier)
	if err != nil {
		return err
	}

	fmt.Printf("Open this URL to authenticate:\n%s\n\n", authURL)
	fmt.Printf("Waiting for callback at %s ...\n", endpoint.RedirectURL())

	if c.openBrowserEnabled() {
		if err := openBrowser(authURL); err != nil {
			return fmt.Errorf("failed to open browser: %w", err)
		}
	}

	if err := endpoint.Start(); err != nil {
		return err
	}
	code, err := endpoint.WaitForCode()
	if err != nil {
		return err
	}

	_, err = manager.ExchangeAuthorizationCode(ctx, endpoint.RedirectURL(), codeVerifier, code)
	if err != nil {
		return err
	}

	apiKeyMinted := false
	if c.mintAPIKeyEnabled() {
		if _, err := manager.APIKey(ctx); err != nil {
			fmt.Printf("Login successful, but failed to mint OpenAI API key: %v\n", err)
			fmt.Printf("If this persists, ensure your OpenAI Platform account is set up (organization/project), then re-run `agently chatgpt-login`.\n")
			if c.RequireMint {
				return err
			}
		} else {
			apiKeyMinted = true
		}
	}

	fmt.Printf("Login successful. Tokens persisted to %s\n", c.TokensURL)
	if apiKeyMinted {
		fmt.Printf("OpenAI API key minted and cached.\n")
	}
	return nil
}

func deriveTokensURLFromClientURL(clientURL string) (string, error) {
	clientURL = strings.TrimSpace(clientURL)
	if clientURL == "" {
		return "", fmt.Errorf("clientURL was empty")
	}
	base, keySuffix := splitScyKeySuffix(clientURL)
	tokensBase := deriveTokensPath(base)
	if tokensBase == "" {
		return "", fmt.Errorf("failed to derive tokensURL from clientURL %q", clientURL)
	}
	if keySuffix == "" {
		return tokensBase, nil
	}
	return tokensBase + "|" + keySuffix, nil
}

func splitScyKeySuffix(encoded string) (base string, key string) {
	parts := strings.SplitN(encoded, "|", 2)
	base = strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		key = strings.TrimSpace(parts[1])
	}
	return base, key
}

func deriveTokensPath(clientURL string) string {
	clientURL = strings.TrimSpace(clientURL)
	if clientURL == "" {
		return ""
	}

	u, err := url.Parse(clientURL)
	if err == nil && u.Scheme != "" {
		path := u.Path
		ext := strings.ToLower(filepath.Ext(path))
		tokensExt := ext
		switch ext {
		case ".yaml", ".yml":
			tokensExt = ext
		case ".json":
			tokensExt = ext
		default:
			tokensExt = ".json"
			ext = ""
		}
		if ext == "" {
			u.Path = path + ".tokens" + tokensExt
		} else {
			u.Path = strings.TrimSuffix(path, ext) + ".tokens" + tokensExt
		}
		return u.String()
	}

	ext := strings.ToLower(filepath.Ext(clientURL))
	switch ext {
	case ".yaml", ".yml", ".json":
		return strings.TrimSuffix(clientURL, ext) + ".tokens" + ext
	default:
		return clientURL + ".tokens.json"
	}
}

func (c *ChatGPTLoginCmd) openBrowserEnabled() bool {
	return !c.NoOpenBrowser
}

func (c *ChatGPTLoginCmd) mintAPIKeyEnabled() bool {
	return !c.NoMintAPIKey
}

func timeoutFromSeconds(seconds int) time.Duration {
	if seconds <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(seconds) * time.Second
}

func randomToken(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("token length must be positive")
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func openBrowser(targetURL string) error {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return fmt.Errorf("url was empty")
	}
	if _, err := url.Parse(targetURL); err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
		args = []string{}
	default:
		cmd = "xdg-open"
		args = []string{}
	}
	args = append(args, targetURL)
	return exec.Command(cmd, args...).Start()
}

type ipv4CallbackEndpoint struct {
	server      *http.Server
	listener    net.Listener
	redirectURL string

	stateExpected string
	timeout       time.Duration

	mu     sync.Mutex
	code   string
	waitCh chan error
}

func newIPv4CallbackEndpoint(stateExpected string, timeout time.Duration, port int) (*ipv4CallbackEndpoint, error) {
	listener, actualPort, err := listenWithCancel(port)
	if err != nil {
		return nil, err
	}
	endpoint := &ipv4CallbackEndpoint{
		listener: listener,
		// Match Codex redirect path: /auth/callback.
		redirectURL:   fmt.Sprintf("http://localhost:%d/auth/callback", actualPort),
		stateExpected: stateExpected,
		timeout:       timeout,
		waitCh:        make(chan error, 1),
	}

	mux := http.NewServeMux()
	// Primary: match Codex callback path.
	mux.HandleFunc("/auth/callback", endpoint.handleCallback)
	// Backward-compatible alias.
	mux.HandleFunc("/callback", endpoint.handleCallback)
	mux.HandleFunc("/cancel", endpoint.handleCancel)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	endpoint.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
	return endpoint, nil
}

func (e *ipv4CallbackEndpoint) RedirectURL() string { return e.redirectURL }

func (e *ipv4CallbackEndpoint) Start() error {
	go func() {
		err := e.server.Serve(e.listener)
		if err != nil && !strings.Contains(err.Error(), "closed") {
			select {
			case e.waitCh <- err:
			default:
			}
		}
	}()
	return nil
}

func (e *ipv4CallbackEndpoint) WaitForCode() (string, error) {
	select {
	case err := <-e.waitCh:
		if err != nil {
			return "", err
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.code == "" {
			return "", fmt.Errorf("authorization code was empty")
		}
		return e.code, nil
	case <-time.After(e.timeout):
		return "", fmt.Errorf("authentication timed out after %s", e.timeout)
	}
}

func (e *ipv4CallbackEndpoint) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = e.server.Shutdown(ctx)
}

func (e *ipv4CallbackEndpoint) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	state := query.Get("state")
	code := query.Get("code")
	if e.stateExpected != "" && state != e.stateExpected {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("State mismatch"))
		select {
		case e.waitCh <- fmt.Errorf("state mismatch"):
		default:
		}
		return
	}
	if code == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Missing authorization code"))
		select {
		case e.waitCh <- fmt.Errorf("missing authorization code"):
		default:
		}
		return
	}

	e.mu.Lock()
	e.code = code
	e.mu.Unlock()

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Login successful. You can close this window."))
	select {
	case e.waitCh <- nil:
	default:
	}
}

func (e *ipv4CallbackEndpoint) handleCancel(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Login canceled. You can close this window."))
	select {
	case e.waitCh <- fmt.Errorf("login canceled"):
	default:
	}
	go e.Close()
}

func listenWithCancel(port int) (net.Listener, int, error) {
	if port <= 0 || port > 65535 {
		return nil, 0, fmt.Errorf("invalid port %d", port)
	}

	address := fmt.Sprintf("127.0.0.1:%d", port)
	cancelAttempted := false
	for attempt := 0; attempt < 10; attempt++ {
		listener, err := net.Listen("tcp4", address)
		if err == nil {
			actual := listener.Addr().(*net.TCPAddr).Port
			return listener, actual, nil
		}
		if !isAddrInUse(err) {
			return nil, 0, fmt.Errorf("failed to listen for callback on %s: %w", address, err)
		}
		if !cancelAttempted {
			cancelAttempted = true
			_ = sendCancelRequest(port)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, 0, fmt.Errorf("port %d is already in use", port)
}

func isAddrInUse(err error) bool {
	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return false
	}
	var sysErr *os.SyscallError
	if !errors.As(opErr.Err, &sysErr) {
		return false
	}
	return errors.Is(sysErr.Err, syscall.EADDRINUSE)
}

func sendCancelRequest(port int) error {
	address := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp4", address, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	req := fmt.Sprintf("GET /cancel HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", address)
	if _, err := conn.Write([]byte(req)); err != nil {
		return err
	}
	buf := make([]byte, 64)
	_, _ = conn.Read(buf)
	return nil
}
