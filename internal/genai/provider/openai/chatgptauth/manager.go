package chatgptauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/viant/scy/auth/flow"
)

const (
	defaultIssuer         = "https://auth.openai.com"
	defaultOriginator     = "codex_cli_rs"
	defaultAPIKeyCacheTTL = 30 * time.Minute
	defaultRefreshPeriod  = 7 * 24 * time.Hour
	defaultRefreshSkew    = 10 * time.Minute
)

type Manager struct {
	issuer             string
	issuerExplicit     bool
	allowedWorkspaceID string
	originator         string

	httpClient *http.Client
	client     OAuthClientLoader
	store      TokenStateStore

	mu sync.Mutex
}

func NewManager(options *Options, client OAuthClientLoader, store TokenStateStore, httpClient *http.Client) (*Manager, error) {
	if options == nil {
		return nil, fmt.Errorf("chatgptOAuth options were nil")
	}
	if client == nil {
		return nil, fmt.Errorf("oauth client loader was nil")
	}
	if store == nil {
		return nil, fmt.Errorf("token store was nil")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	issuer := strings.TrimSpace(options.Issuer)
	issuerExplicit := issuer != ""
	originator := strings.TrimSpace(options.Originator)
	if originator == "" {
		originator = defaultOriginator
	}
	return &Manager{
		issuer:             strings.TrimRight(issuer, "/"),
		issuerExplicit:     issuerExplicit,
		allowedWorkspaceID: strings.TrimSpace(options.AllowedWorkspaceID),
		originator:         originator,
		httpClient:         httpClient,
		client:             client,
		store:              store,
	}, nil
}

// BuildAuthorizeURL builds the ChatGPT OAuth authorization URL for Code+PKCE.
// This is used by the interactive login flow (implemented separately).
func (m *Manager) BuildAuthorizeURL(ctx context.Context, redirectURI string, state string, codeVerifier string) (string, error) {
	oauthClient, err := m.client.Load(ctx)
	if err != nil {
		return "", err
	}
	redirectURI = strings.TrimSpace(redirectURI)
	if redirectURI == "" {
		return "", fmt.Errorf("redirectURI was empty")
	}
	codeVerifier = strings.TrimSpace(codeVerifier)
	if codeVerifier == "" {
		return "", fmt.Errorf("codeVerifier was empty")
	}
	if state == "" {
		state = ""
	}

	issuer := m.issuerForClient(oauthClient)
	scope := "openid profile email offline_access"
	codeChallenge := flow.GenerateCodeChallenge(codeVerifier)

	params := [][2]string{
		{"response_type", "code"},
		{"client_id", oauthClient.ClientID},
		{"redirect_uri", redirectURI},
		{"scope", scope},
		{"code_challenge", codeChallenge},
		{"code_challenge_method", "S256"},
		{"id_token_add_organizations", "true"},
		{"codex_cli_simplified_flow", "true"},
		{"state", state},
		{"originator", m.originator},
	}
	if strings.TrimSpace(m.allowedWorkspaceID) != "" {
		params = append(params, [2]string{"allowed_workspace_id", strings.TrimSpace(m.allowedWorkspaceID)})
	}

	qs := make([]string, 0, len(params))
	for _, kv := range params {
		qs = append(qs, kv[0]+"="+escapeQueryValue(kv[1]))
	}
	return issuer + "/oauth/authorize?" + strings.Join(qs, "&"), nil
}

// ExchangeAuthorizationCode exchanges an OAuth authorization code for tokens and persists them.
func (m *Manager) ExchangeAuthorizationCode(ctx context.Context, redirectURI string, codeVerifier string, code string) (*TokenState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	oauthClient, err := m.client.Load(ctx)
	if err != nil {
		return nil, err
	}
	redirectURI = strings.TrimSpace(redirectURI)
	if redirectURI == "" {
		return nil, fmt.Errorf("redirectURI was empty")
	}
	codeVerifier = strings.TrimSpace(codeVerifier)
	if codeVerifier == "" {
		return nil, fmt.Errorf("codeVerifier was empty")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("authorization code was empty")
	}

	issuer := m.issuerForClient(oauthClient)
	tokens, err := m.exchangeCodeForTokens(ctx, issuer, oauthClient.ClientID, oauthClient.ClientSecret, redirectURI, codeVerifier, code)
	if err != nil {
		return nil, err
	}
	if err := ensureWorkspaceAllowed(m.allowedWorkspaceID, tokens.IDToken); err != nil {
		return nil, err
	}

	state := &TokenState{
		IDToken:      tokens.IDToken,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		LastRefresh:  time.Now().UTC(),
	}
	if err := m.store.Save(ctx, state); err != nil {
		return nil, err
	}
	return state, nil
}

// APIKey returns an OpenAI API key minted from ChatGPT OAuth tokens (token exchange).
// Intended to be used as an OpenAI client APIKeyProvider.
func (m *Manager) APIKey(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	oauthClient, err := m.client.Load(ctx)
	if err != nil {
		return "", err
	}
	issuer := m.issuerForClient(oauthClient)
	state, err := m.store.Load(ctx)
	if err != nil {
		return "", err
	}
	if state == nil {
		return "", fmt.Errorf("token state was empty")
	}

	if cached := strings.TrimSpace(state.OpenAIAPIKey); cached != "" && !isAPIKeyExpired(state) {
		return cached, nil
	}

	state, err = m.refreshIfNeeded(ctx, issuer, oauthClient.ClientID, oauthClient.ClientSecret, state)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(state.IDToken) == "" {
		return "", fmt.Errorf("id_token is required to mint API key")
	}
	if err := ensureWorkspaceAllowed(m.allowedWorkspaceID, state.IDToken); err != nil {
		return "", err
	}

	apiKey, err := m.obtainAPIKey(ctx, issuer, oauthClient.ClientID, state.IDToken)
	if err != nil {
		return "", err
	}
	state.OpenAIAPIKey = apiKey
	state.OpenAIAPIKeyAt = time.Now().UTC()
	if state.OpenAIAPIKeyTTLMS == 0 {
		state.OpenAIAPIKeyTTLMS = defaultAPIKeyCacheTTL.Milliseconds()
	}
	if err := m.store.Save(ctx, state); err != nil {
		return "", err
	}
	return apiKey, nil
}

type exchangedTokens struct {
	IDToken      string
	AccessToken  string
	RefreshToken string
}

func (m *Manager) exchangeCodeForTokens(ctx context.Context, issuer string, clientID string, clientSecret string, redirectURI string, codeVerifier string, code string) (*exchangedTokens, error) {
	endpoint := issuer + "/oauth/token"
	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("code", code)
	values.Set("redirect_uri", redirectURI)
	values.Set("client_id", clientID)
	values.Set("code_verifier", codeVerifier)
	if strings.TrimSpace(clientSecret) != "" {
		values.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return &exchangedTokens{
		IDToken:      parsed.IDToken,
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
	}, nil
}

func (m *Manager) refreshIfNeeded(ctx context.Context, issuer string, clientID string, clientSecret string, state *TokenState) (*TokenState, error) {
	if state == nil {
		return nil, fmt.Errorf("token state was nil")
	}
	if strings.TrimSpace(state.RefreshToken) == "" {
		return state, nil
	}
	if !needsRefresh(state) {
		return state, nil
	}

	endpoint := issuer + "/oauth/token"
	payload := map[string]any{
		"client_id":     clientID,
		"grant_type":    "refresh_token",
		"refresh_token": state.RefreshToken,
		"scope":         "openid profile email",
	}
	if strings.TrimSpace(clientSecret) != "" {
		payload["client_secret"] = clientSecret
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("refresh token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		IDToken      *string `json:"id_token"`
		AccessToken  *string `json:"access_token"`
		RefreshToken *string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if parsed.IDToken != nil && strings.TrimSpace(*parsed.IDToken) != "" {
		state.IDToken = *parsed.IDToken
		state.OpenAIAPIKey = ""
	}
	if parsed.AccessToken != nil && strings.TrimSpace(*parsed.AccessToken) != "" {
		state.AccessToken = *parsed.AccessToken
	}
	if parsed.RefreshToken != nil && strings.TrimSpace(*parsed.RefreshToken) != "" {
		state.RefreshToken = *parsed.RefreshToken
		state.OpenAIAPIKey = ""
	}
	state.LastRefresh = time.Now().UTC()
	if err := m.store.Save(ctx, state); err != nil {
		return nil, err
	}
	return state, nil
}

func (m *Manager) obtainAPIKey(ctx context.Context, issuer string, clientID string, idToken string) (string, error) {
	endpoint := issuer + "/oauth/token"
	values := url.Values{}
	values.Set("grant_type", "urn:ietf:params:oauth:grant-type:token-exchange")
	values.Set("client_id", clientID)
	values.Set("requested_token", "openai-api-key")
	values.Set("subject_token", idToken)
	values.Set("subject_token_type", "urn:ietf:params:oauth:token-type:id_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if err := classifyMintAPIKeyError(body); err != nil {
			return "", err
		}
		return "", fmt.Errorf("api key exchange failed with status %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return "", fmt.Errorf("api key exchange returned empty access_token")
	}
	return parsed.AccessToken, nil
}

func classifyMintAPIKeyError(body []byte) error {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil
	}
	var parsed struct {
		Error *struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	if parsed.Error == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(parsed.Error.Message), "missing organization_id") {
		message := strings.TrimSpace(parsed.Error.Message)
		if message == "" {
			message = "Invalid ID token: missing organization_id"
		}
		return &MissingOrganizationIDError{Message: message}
	}
	return nil
}

func needsRefresh(state *TokenState) bool {
	if state == nil {
		return false
	}
	if state.LastRefresh.IsZero() {
		return true
	}
	if time.Since(state.LastRefresh) >= defaultRefreshPeriod {
		return true
	}
	if exp, ok := parseJWTExpiry(state.IDToken); ok {
		if time.Until(exp) <= defaultRefreshSkew {
			return true
		}
	}
	if exp, ok := parseJWTExpiry(state.AccessToken); ok {
		if time.Until(exp) <= defaultRefreshSkew {
			return true
		}
	}
	return false
}

func (m *Manager) issuerForClient(client *OAuthClientConfig) string {
	if m.issuerExplicit && strings.TrimSpace(m.issuer) != "" {
		return strings.TrimRight(m.issuer, "/")
	}
	if client != nil && strings.TrimSpace(client.Issuer) != "" {
		return strings.TrimRight(strings.TrimSpace(client.Issuer), "/")
	}
	return defaultIssuer
}

func isAPIKeyExpired(state *TokenState) bool {
	if state == nil {
		return true
	}
	ttl := defaultAPIKeyCacheTTL
	if state.OpenAIAPIKeyTTLMS > 0 {
		ttl = time.Duration(state.OpenAIAPIKeyTTLMS) * time.Millisecond
	}
	if state.OpenAIAPIKeyAt.IsZero() {
		return true
	}
	return time.Since(state.OpenAIAPIKeyAt) >= ttl
}

func withOptionalAuthURLParam(key string, value string) flow.Option {
	return func(o *flow.Options) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		flow.WithAuthURLParam(key, trimmed)(o)
	}
}

func escapeQueryValue(value string) string {
	escaped := url.QueryEscape(value)
	// Match Rust `urlencoding::encode` behavior for spaces in query values.
	return strings.ReplaceAll(escaped, "+", "%20")
}
