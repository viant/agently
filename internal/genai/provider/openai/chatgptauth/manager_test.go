package chatgptauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_APIKey(t *testing.T) {
	type testCase struct {
		name          string
		initialState  *TokenState
		allowedWorksp string
		handle        func(rec *responseRecorder, r *http.Request)
		wantErr       bool
		assertErr     func(t *testing.T, err error)
		expectedKey   string
		expectedCalls int32
	}

	jwt := fakeJWTWithWorkspaceAndExp("ws-1", time.Now().Add(30*time.Minute))

	testCases := []testCase{
		{
			name: "returns cached api key when fresh",
			initialState: &TokenState{
				IDToken:           jwt,
				AccessToken:       "access-1",
				RefreshToken:      "refresh-1",
				LastRefresh:       time.Now(),
				OpenAIAPIKey:      "sk-cached",
				OpenAIAPIKeyAt:    time.Now().Add(-5 * time.Minute),
				OpenAIAPIKeyTTLMS: time.Hour.Milliseconds(),
			},
			allowedWorksp: "ws-1",
			handle: func(rec *responseRecorder, _ *http.Request) {
				rec.WriteHeader(http.StatusInternalServerError)
			},
			wantErr:       false,
			expectedKey:   "sk-cached",
			expectedCalls: 0,
		},
		{
			name: "mints api key via token exchange",
			initialState: &TokenState{
				IDToken:      jwt,
				LastRefresh:  time.Now(),
				RefreshToken: "refresh-1",
			},
			allowedWorksp: "ws-1",
			handle: func(rec *responseRecorder, r *http.Request) {
				if r.URL.Path != "/oauth/token" {
					rec.WriteHeader(http.StatusNotFound)
					return
				}
				body, _ := io.ReadAll(r.Body)
				_ = r.Body.Close()
				values, _ := url.ParseQuery(string(body))
				if values.Get("grant_type") != "urn:ietf:params:oauth:grant-type:token-exchange" {
					rec.WriteHeader(http.StatusBadRequest)
					return
				}
				rec.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(rec).Encode(map[string]string{"access_token": "sk-minted"})
			},
			wantErr:       false,
			expectedKey:   "sk-minted",
			expectedCalls: 1,
		},
		{
			name: "returns typed error when org id missing",
			initialState: &TokenState{
				IDToken:      fakeJWTWithWorkspaceAndExp("ws-1", time.Now().Add(30*time.Minute)),
				LastRefresh:  time.Now(),
				RefreshToken: "refresh-1",
			},
			allowedWorksp: "ws-1",
			handle: func(rec *responseRecorder, r *http.Request) {
				if r.URL.Path != "/oauth/token" {
					rec.WriteHeader(http.StatusNotFound)
					return
				}
				body, _ := io.ReadAll(r.Body)
				_ = r.Body.Close()
				values, _ := url.ParseQuery(string(body))
				if values.Get("grant_type") != "urn:ietf:params:oauth:grant-type:token-exchange" {
					rec.WriteHeader(http.StatusBadRequest)
					return
				}
				rec.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(rec).Encode(map[string]any{
					"error": map[string]any{
						"message": "Invalid ID token: missing organization_id",
						"code":    "invalid_subject_token",
					},
				})
			},
			wantErr: true,
			assertErr: func(t *testing.T, err error) {
				var typed *MissingOrganizationIDError
				assert.EqualValues(t, true, errors.As(err, &typed))
				assert.EqualValues(t, "Invalid ID token: missing organization_id", typed.Error())
			},
			expectedCalls: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var calls int32
			httpClient := &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					atomic.AddInt32(&calls, 1)
					rec := newResponseRecorder()
					tc.handle(rec, r)
					return rec.Result(r), nil
				}),
			}

			tempDir := t.TempDir()
			clientPath := filepath.Join(tempDir, "client.json")
			tokensPath := filepath.Join(tempDir, "tokens.json")

			clientBytes, err := json.Marshal(map[string]string{"client_id": "cid"})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(clientPath, clientBytes, 0o600))

			stateBytes, err := json.Marshal(tc.initialState)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(tokensPath, stateBytes, 0o600))

			manager, err := NewManager(
				&Options{
					ClientURL:          clientPath,
					TokensURL:          tokensPath,
					Issuer:             "https://auth.example",
					AllowedWorkspaceID: tc.allowedWorksp,
				},
				NewScyOAuthClientLoader(clientPath),
				NewScyTokenStateStore(tokensPath),
				httpClient,
			)
			require.NoError(t, err)

			key, err := manager.APIKey(context.Background())
			if tc.wantErr {
				assert.EqualValues(t, "", key)
				assert.EqualValues(t, true, err != nil)
				if tc.assertErr != nil {
					tc.assertErr(t, err)
				}
			} else {
				require.NoError(t, err)
				assert.EqualValues(t, tc.expectedKey, key)
			}
			assert.EqualValues(t, tc.expectedCalls, calls)
		})
	}
}

func TestManager_AccessToken_RefreshTokenReused_ReloadsState(t *testing.T) {
	var refreshCalls int32
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			rec := newResponseRecorder()
			if r.URL.Path != "/oauth/token" {
				rec.WriteHeader(http.StatusNotFound)
				return rec.Result(r), nil
			}
			atomic.AddInt32(&refreshCalls, 1)
			rec.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(rec).Encode(map[string]any{
				"error": map[string]any{
					"message": "Your refresh token has already been used to generate a new access token. Please try signing in again.",
					"type":    "invalid_request_error",
					"code":    "refresh_token_reused",
				},
			})
			return rec.Result(r), nil
		}),
	}

	tempDir := t.TempDir()
	clientPath := filepath.Join(tempDir, "client.json")
	tokensPath := filepath.Join(tempDir, "tokens.json")

	require.NoError(t, os.WriteFile(clientPath, []byte(`{"client_id":"cid"}`), 0o600))

	// Initial stale state forces refresh attempt.
	stale := &TokenState{
		IDToken:      fakeJWTWithWorkspaceAndExp("ws-1", time.Now().Add(30*time.Minute)),
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		LastRefresh:  time.Now().Add(-8 * 24 * time.Hour),
	}
	stateBytes, err := json.Marshal(stale)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tokensPath, stateBytes, 0o600))

	manager, err := NewManager(
		&Options{
			ClientURL:          clientPath,
			TokensURL:          tokensPath,
			Issuer:             "https://auth.example",
			AllowedWorkspaceID: "ws-1",
		},
		NewScyOAuthClientLoader(clientPath),
		NewScyTokenStateStore(tokensPath),
		httpClient,
	)
	require.NoError(t, err)

	// Simulate another process having already rotated tokens.
	rotated := &TokenState{
		IDToken:      fakeJWTWithWorkspaceAndExp("ws-1", time.Now().Add(30*time.Minute)),
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		LastRefresh:  time.Now().UTC(),
	}
	require.NoError(t, NewScyTokenStateStore(tokensPath).Save(context.Background(), rotated))

	token, err := manager.AccessToken(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, "new-access", token)
}

func TestManager_ExchangeAuthorizationCode_PersistsTokens(t *testing.T) {
	var tokenCalls int32
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			rec := newResponseRecorder()
			if r.URL.Path != "/oauth/token" {
				rec.WriteHeader(http.StatusNotFound)
				return rec.Result(r), nil
			}
			atomic.AddInt32(&tokenCalls, 1)
			body, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			values, _ := url.ParseQuery(string(body))
			if values.Get("grant_type") != "authorization_code" {
				rec.WriteHeader(http.StatusBadRequest)
				return rec.Result(r), nil
			}
			jwt := fakeJWTWithWorkspaceAndExp("ws-1", time.Now().Add(30*time.Minute))
			rec.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(rec).Encode(map[string]string{
				"id_token":      jwt,
				"access_token":  "access-1",
				"refresh_token": "refresh-1",
			})
			return rec.Result(r), nil
		}),
	}

	tempDir := t.TempDir()
	clientPath := filepath.Join(tempDir, "client.yaml")
	tokensPath := filepath.Join(tempDir, "tokens.yaml")

	require.NoError(t, os.WriteFile(clientPath, []byte("client_id: cid\n"), 0o600))

	manager, err := NewManager(
		&Options{
			ClientURL:          clientPath,
			TokensURL:          tokensPath,
			Issuer:             "https://auth.example",
			AllowedWorkspaceID: "ws-1",
		},
		NewScyOAuthClientLoader(clientPath),
		NewScyTokenStateStore(tokensPath),
		httpClient,
	)
	require.NoError(t, err)

	_, err = manager.ExchangeAuthorizationCode(context.Background(), "http://localhost/callback", "verifier", "code")
	require.NoError(t, err)
	assert.EqualValues(t, int32(1), tokenCalls)

	loaded, err := NewScyTokenStateStore(tokensPath).Load(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, "access-1", loaded.AccessToken)
	assert.EqualValues(t, "refresh-1", loaded.RefreshToken)
	assert.EqualValues(t, false, loaded.LastRefresh.IsZero())
}

func TestManager_APIKey_WhenTokenStateMissing(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			rec := newResponseRecorder()
			rec.WriteHeader(http.StatusInternalServerError)
			return rec.Result(r), nil
		}),
	}

	tempDir := t.TempDir()
	clientPath := filepath.Join(tempDir, "client.json")
	tokensPath := filepath.Join(tempDir, "missing.json")

	clientBytes, err := json.Marshal(map[string]string{"client_id": "cid"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(clientPath, clientBytes, 0o600))

	manager, err := NewManager(
		&Options{
			ClientURL:          clientPath,
			TokensURL:          tokensPath,
			Issuer:             "https://auth.example",
			AllowedWorkspaceID: "ws-1",
		},
		NewScyOAuthClientLoader(clientPath),
		NewScyTokenStateStore(tokensPath),
		httpClient,
	)
	require.NoError(t, err)

	_, err = manager.APIKey(context.Background())
	var notFound *TokenStateNotFoundError
	assert.EqualValues(t, true, errors.As(err, &notFound))
	assert.EqualValues(t, tokensPath, notFound.TokensURL)
}

func TestManager_BuildAuthorizeURL_IncludesExpectedParams(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			rec := newResponseRecorder()
			rec.WriteHeader(http.StatusInternalServerError)
			return rec.Result(r), nil
		}),
	}

	tempDir := t.TempDir()
	clientPath := filepath.Join(tempDir, "client.json")
	tokensPath := filepath.Join(tempDir, "tokens.json")

	clientBytes, err := json.Marshal(map[string]string{"client_id": "cid"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(clientPath, clientBytes, 0o600))
	require.NoError(t, os.WriteFile(tokensPath, []byte(`{}`), 0o600))

	manager, err := NewManager(
		&Options{
			ClientURL:          clientPath,
			TokensURL:          tokensPath,
			Issuer:             "https://auth.example",
			AllowedWorkspaceID: "ws-1",
		},
		NewScyOAuthClientLoader(clientPath),
		NewScyTokenStateStore(tokensPath),
		httpClient,
	)
	require.NoError(t, err)

	authURL, err := manager.BuildAuthorizeURL(context.Background(), "http://localhost/callback", "state123", "verifier123")
	require.NoError(t, err)

	parsed, err := url.Parse(authURL)
	require.NoError(t, err)
	q := parsed.Query()

	assert.EqualValues(t, "code", q.Get("response_type"))
	assert.EqualValues(t, "cid", q.Get("client_id"))
	assert.EqualValues(t, "http://localhost/callback", q.Get("redirect_uri"))
	assert.EqualValues(t, "S256", q.Get("code_challenge_method"))
	assert.EqualValues(t, "state123", q.Get("state"))
	assert.EqualValues(t, "true", q.Get("id_token_add_organizations"))
	assert.EqualValues(t, "true", q.Get("codex_cli_simplified_flow"))
	assert.EqualValues(t, "ws-1", q.Get("allowed_workspace_id"))

	scope := q.Get("scope")
	assert.EqualValues(t, true, strings.Contains(scope, "offline_access"))
	assert.EqualValues(t, true, strings.Contains(scope, "openid"))
}

func fakeJWTWithWorkspaceAndExp(workspaceID string, exp time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payloadMap := map[string]any{
		"exp": exp.Unix(),
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": workspaceID,
		},
	}
	payloadBytes, _ := json.Marshal(payloadMap)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return header + "." + payload + "."
}

type roundTripFunc func(r *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type responseRecorder struct {
	status int
	header http.Header
	body   strings.Builder
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{status: http.StatusOK, header: make(http.Header)}
}

func (r *responseRecorder) Header() http.Header { return r.header }

func (r *responseRecorder) WriteHeader(statusCode int) { r.status = statusCode }

func (r *responseRecorder) Write(p []byte) (int, error) { return r.body.Write(p) }

func (r *responseRecorder) Result(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: r.status,
		Header:     r.header,
		Body:       io.NopCloser(strings.NewReader(r.body.String())),
		Request:    req,
	}
}
