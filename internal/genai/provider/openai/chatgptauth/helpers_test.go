package chatgptauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseJWTExpiry(t *testing.T) {
	type testCase struct {
		name        string
		token       string
		expectOK    bool
		expectEpoch int64
	}

	exp := time.Now().Add(10 * time.Minute).Unix()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	withExpPayload, _ := json.Marshal(map[string]any{"exp": exp})
	withExp := header + "." + base64.RawURLEncoding.EncodeToString(withExpPayload) + "."
	noExpPayload, _ := json.Marshal(map[string]any{"sub": "x"})
	noExp := header + "." + base64.RawURLEncoding.EncodeToString(noExpPayload) + "."

	testCases := []testCase{
		{name: "valid exp", token: withExp, expectOK: true, expectEpoch: exp},
		{name: "missing exp", token: noExp, expectOK: false, expectEpoch: 0},
		{name: "invalid format", token: "abc", expectOK: false, expectEpoch: 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseJWTExpiry(tc.token)
			assert.EqualValues(t, tc.expectOK, ok)
			if tc.expectOK {
				assert.EqualValues(t, tc.expectEpoch, got.Unix())
			}
		})
	}
}

func TestNeedsRefresh(t *testing.T) {
	now := time.Now().UTC()
	soon := now.Add(2 * time.Minute)
	later := now.Add(2 * time.Hour)

	type testCase struct {
		name     string
		state    *TokenState
		expected bool
	}

	testCases := []testCase{
		{name: "nil state", state: nil, expected: false},
		{name: "zero last_refresh", state: &TokenState{IDToken: fakeJWTWithWorkspaceAndExp("ws", later)}, expected: true},
		{name: "old last_refresh", state: &TokenState{LastRefresh: now.Add(-8 * 24 * time.Hour), IDToken: fakeJWTWithWorkspaceAndExp("ws", later)}, expected: true},
		{name: "jwt exp soon", state: &TokenState{LastRefresh: now, IDToken: fakeJWTWithWorkspaceAndExp("ws", soon)}, expected: true},
		{name: "fresh", state: &TokenState{LastRefresh: now, IDToken: fakeJWTWithWorkspaceAndExp("ws", later)}, expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.EqualValues(t, tc.expected, needsRefresh(tc.state))
		})
	}
}

func TestScyTokenStateStore_RoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "tokens.json")
	store := NewScyTokenStateStore(path)

	expected := &TokenState{
		IDToken:           "id",
		AccessToken:       "access",
		RefreshToken:      "refresh",
		LastRefresh:       time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
		OpenAIAPIKey:      "sk",
		OpenAIAPIKeyAt:    time.Date(2025, 1, 2, 3, 5, 0, 0, time.UTC),
		OpenAIAPIKeyTTLMS: (15 * time.Minute).Milliseconds(),
	}

	require.NoError(t, store.Save(context.Background(), expected))
	actual, err := store.Load(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, expected, actual)
}

func TestManager_APIKey_UsesExpectedTokenExchangeRequest(t *testing.T) {
	var seen struct {
		grantType        string
		clientID         string
		requestedToken   string
		subjectToken     string
		subjectTokenType string
	}

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			rec := newResponseRecorder()
			body, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			values, _ := url.ParseQuery(string(body))
			seen.grantType = values.Get("grant_type")
			seen.clientID = values.Get("client_id")
			seen.requestedToken = values.Get("requested_token")
			seen.subjectToken = values.Get("subject_token")
			seen.subjectTokenType = values.Get("subject_token_type")

			rec.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(rec).Encode(map[string]string{"access_token": "sk-minted"})
			return rec.Result(r), nil
		}),
	}

	tempDir := t.TempDir()
	clientPath := filepath.Join(tempDir, "client.json")
	tokensPath := filepath.Join(tempDir, "tokens.json")

	jwt := fakeJWTWithWorkspaceAndExp("ws", time.Now().Add(time.Hour))
	require.NoError(t, os.WriteFile(clientPath, []byte(`{"client_id":"cid"}`), 0o600))
	require.NoError(t, os.WriteFile(tokensPath, []byte(`{"id_token":"`+jwt+`","last_refresh":"`+time.Now().UTC().Format(time.RFC3339Nano)+`"}`), 0o600))

	manager, err := NewManager(
		&Options{
			ClientURL:          clientPath,
			TokensURL:          tokensPath,
			Issuer:             "https://auth.example",
			AllowedWorkspaceID: "ws",
		},
		NewScyOAuthClientLoader(clientPath),
		NewScyTokenStateStore(tokensPath),
		httpClient,
	)
	require.NoError(t, err)

	key, err := manager.APIKey(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, "sk-minted", key)

	assert.EqualValues(t, "urn:ietf:params:oauth:grant-type:token-exchange", seen.grantType)
	assert.EqualValues(t, "cid", seen.clientID)
	assert.EqualValues(t, "openai-api-key", seen.requestedToken)
	assert.EqualValues(t, jwt, seen.subjectToken)
	assert.EqualValues(t, "urn:ietf:params:oauth:token-type:id_token", seen.subjectTokenType)
}
