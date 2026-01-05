package cookies

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/viant/afs"
	"github.com/viant/agently/internal/auth"
	mcpcfg "github.com/viant/agently/internal/mcp/config"
	mcprepo "github.com/viant/agently/internal/workspace/repository/mcp"
	"github.com/viant/mcp"
	authtransport "github.com/viant/mcp/client/auth/transport"
)

func TestProvider_Jar(t *testing.T) {
	type testCase struct {
		description           string
		user                  string
		includeAnonymousScope bool
		setup                 func(t *testing.T, workspaceRoot string, serverName string)
		probeURL              string
		expectCookieName      string
		expectCookieValue     string
	}

	serverName := "testmcp"

	testCases := []testCase{
		{
			description:           "loads user scope cookies",
			user:                  "alice",
			includeAnonymousScope: false,
			setup: func(t *testing.T, workspaceRoot string, serverName string) {
				t.Helper()
				writeMCPConfig(t, workspaceRoot, serverName, "http://localhost:5000")
				writeCookieJar(t, filepath.Join(workspaceRoot, "state", "mcp", serverName, "alice", "cookies.json"), "http://localhost:5000", "sid", "u1")
			},
			probeURL:          "http://localhost:5000",
			expectCookieName:  "sid",
			expectCookieValue: "u1",
		},
		{
			description:           "does not migrate anonymous cookies by default",
			user:                  "alice",
			includeAnonymousScope: false,
			setup: func(t *testing.T, workspaceRoot string, serverName string) {
				t.Helper()
				writeMCPConfig(t, workspaceRoot, serverName, "http://localhost:5000")
				writeCookieJar(t, filepath.Join(workspaceRoot, "state", "mcp", serverName, "anonymous", "cookies.json"), "http://localhost:5000", "sid", "anon")
			},
			probeURL:          "http://localhost:5000",
			expectCookieName:  "",
			expectCookieValue: "",
		},
		{
			description:           "can migrate anonymous cookies when enabled",
			user:                  "alice",
			includeAnonymousScope: true,
			setup: func(t *testing.T, workspaceRoot string, serverName string) {
				t.Helper()
				writeMCPConfig(t, workspaceRoot, serverName, "http://localhost:5000")
				writeCookieJar(t, filepath.Join(workspaceRoot, "state", "mcp", serverName, "anonymous", "cookies.json"), "http://localhost:5000", "sid", "anon")
			},
			probeURL:          "http://localhost:5000",
			expectCookieName:  "sid",
			expectCookieValue: "anon",
		},
		{
			description:           "mirrors localhost cookies to 127.0.0.1",
			user:                  "alice",
			includeAnonymousScope: false,
			setup: func(t *testing.T, workspaceRoot string, serverName string) {
				t.Helper()
				writeMCPConfig(t, workspaceRoot, serverName, "http://localhost:5000")
				writeCookieJar(t, filepath.Join(workspaceRoot, "state", "mcp", serverName, "alice", "cookies.json"), "http://localhost:5000", "sid", "u1")
			},
			probeURL:          "http://127.0.0.1:5000",
			expectCookieName:  "sid",
			expectCookieValue: "u1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			workspaceRoot := t.TempDir()
			t.Setenv("AGENTLY_WORKSPACE", workspaceRoot)
			t.Setenv("AGENTLY_WORKSPACE_NO_DEFAULTS", "1")

			if tc.setup != nil {
				tc.setup(t, workspaceRoot, serverName)
			}

			fs := afs.New()
			repo := mcprepo.New(fs)
			provider := New(fs, repo, WithAnonymousScope(tc.includeAnonymousScope))

			ctx := context.Background()
			if tc.user != "" {
				ctx = auth.WithUserInfo(ctx, &auth.UserInfo{Subject: tc.user})
			}

			jar, err := provider.Jar(ctx)
			assert.EqualValues(t, nil, err)

			u, err := url.Parse(tc.probeURL)
			assert.EqualValues(t, nil, err)

			cookies := jar.Cookies(u)
			gotName, gotValue := findCookie(cookies)

			assert.EqualValues(t, tc.expectCookieName, gotName)
			assert.EqualValues(t, tc.expectCookieValue, gotValue)
		})
	}
}

func writeMCPConfig(t *testing.T, workspaceRoot, serverName, origin string) {
	t.Helper()
	t.Setenv("AGENTLY_WORKSPACE", workspaceRoot)
	fs := afs.New()
	repo := mcprepo.New(fs)
	cfg := &mcpcfg.MCPClient{
		ClientOptions: &mcp.ClientOptions{
			Transport: mcp.ClientTransport{
				Type: "sse",
				ClientTransportHTTP: mcp.ClientTransportHTTP{
					URL: origin,
				},
			},
		},
	}
	err := repo.Save(context.Background(), serverName, cfg)
	assert.EqualValues(t, nil, err)
}

func writeCookieJar(t *testing.T, jarPath, origin, cookieName, cookieValue string) {
	t.Helper()
	j, err := authtransport.NewFileJar(jarPath)
	assert.EqualValues(t, nil, err)
	u, err := url.Parse(origin)
	assert.EqualValues(t, nil, err)
	j.SetCookies(u, []*http.Cookie{{
		Name:    cookieName,
		Value:   cookieValue,
		Path:    "/",
		Expires: time.Now().Add(1 * time.Hour),
	}})
}

func findCookie(cookies []*http.Cookie) (string, string) {
	if len(cookies) == 0 {
		return "", ""
	}
	for _, c := range cookies {
		if c != nil {
			return c.Name, c.Value
		}
	}
	return "", ""
}
