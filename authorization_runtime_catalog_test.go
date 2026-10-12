package agently

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	uihttp "github.com/viant/agently-core/adapter/http/ui"
	execconfig "github.com/viant/agently-core/app/executor/config"
	appserver "github.com/viant/agently-core/app/server"
	svcauth "github.com/viant/agently-core/service/auth"
	"github.com/viant/agently-core/service/policy"
	forge "github.com/viant/agently-core/service/primitiveprovider"
	windows "github.com/viant/agently-core/service/ui/window"
	metadata "github.com/viant/agently-core/service/workspace"
	"github.com/viant/agently-core/workspace"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/authz/oauth"
	"github.com/viant/forge/backend/types"
	scyauth "github.com/viant/scy/auth"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfiguredRuntimeConsumersRetainFrameworkAfterLateInternalBind(t *testing.T) {
	root := t.TempDir()
	originalRoot := workspace.Root()
	defer workspace.SetRoot(originalRoot)
	t.Setenv("AGENTLY_DB_DSN", "")
	t.Setenv("AGENTLY_DB_PATH", filepath.Join(root, "owned.sqlite"))
	write := func(name string, raw []byte) {
		p := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0700))
		require.NoError(t, os.WriteFile(p, raw, 0600))
	}
	write("extension/forge/windows/arbitrary.yaml", []byte("windowKey: arbitrary\nview: {title: Local, content: {id: local}}\n"))
	write("extension/forge/reporting/builder.yaml", []byte("kind: forge.reporting.builder\nid: fixture\nreportBuilder:\n  document: {title: Fixture}\n"))
	write("extension/forge/reporting/fixture.yaml", []byte("kind: forge.reporting.report\nid: fixture\nnamespace: example\nname: fixture\nresourceUri: report://example/fixture\nbuilderRef: fixture\nlabel: Fixture\ndocument: {title: Fixture, blocks: [{id: text, kind: markdownBlock, markdown: fixture}]}\n"))
	write("catalog.yaml", []byte("baseURL: .\nwindows:\n  - {windowId: arbitrary, key: arbitrary, title: Local, namespace: example, name: arbitrary, resourceUri: 'window://example/arbitrary'}\n"))
	settings := authorizationWorkspaceConfig("authz", "http-fixture", "mapped")
	settingsRaw, err := json.Marshal(settings.Raw)
	require.NoError(t, err)
	write("config.yaml", settingsRaw)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	authority := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "fixture", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
		case "/identity":
			_ = json.NewEncoder(w).Encode(map[string]any{"sub": "fixture-user", "account": "fixture-account", "roles": []string{"reader"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer authority.Close()
	identityRaw, err := json.Marshal(oauth.HTTPIdentityConfig{Kind: "oidc-userinfo", Issuer: authority.URL, Audience: "fixture", Algorithms: []string{"RS256"}, UserInfoURL: authority.URL + "/identity", JWKSURL: authority.URL + "/keys", JWKSRefreshInterval: "1m", FactLease: "30s", TenantForAllAccounts: "fixture-tenant", Mapping: oauth.HTTPIdentityMapping{Subject: "sub", AccountID: "account", JWTAccountID: "account", Roles: "roles"}})
	require.NoError(t, err)
	host := hostAuthorizationFile{SchemaVersion: 1, ProviderRef: "http-fixture", CapabilityMappingRef: "mapped", PolicyVersion: "one", CatalogPath: "catalog.yaml", Identity: identityRaw, ReportResources: json.RawMessage(`{"source":"workspace","builderWindows":{"fixture":"arbitrary"}}`), WindowResources: []windows.ResourceBinding{{WindowKey: "arbitrary", URI: "window://example/arbitrary"}}, FrameworkWindowResources: []windows.ResourceBinding{{WindowKey: "chat/new", CatalogID: "chat/new", URI: "window://agently/chat-new"}}}
	family := authz.ResourceFamily{Kind: "report", ID: "report://example/fixture", Tenant: "fixture-tenant"}
	reportResource := authz.Resource{Kind: "report", ID: family.ID, Tenant: family.Tenant, Version: "working"}
	host.Policies = []authz.Document{{Resource: reportResource, Revision: 1, Policies: map[string]authz.Policy{"read": {Mode: "protected", Rule: &authz.Rule{Kind: "role", Value: "reader"}}}}}
	host.Requirements = []gating.Binding{{Resource: reportResource, Action: "read", Document: gating.RequirementsDocument{Revision: "one", Requirements: gating.Requirements{SchemaVersion: 1}}}}
	host.ResourceSelections = []authz.SelectionDocument{{Resource: family, Revision: 1, DefaultVersion: "working"}}
	host.ResourceRevisionBindings = []policy.ResourceRevisionBinding{{Operation: "report.retrieve", URI: family.ID, Resource: family, Action: "read"}}
	hostRaw, err := json.Marshal(host)
	require.NoError(t, err)
	write("authz.json", hostRaw)
	registration := &internalHostRegistration{}
	options := ServeOptions{AuthzConfigPath: filepath.Join(root, "authz.json"), InternalResourceProviderIdentity: "internal"}
	configure, err := configureHostAuthorization(options, root, settings, nil, registration)
	require.NoError(t, err)
	rt, _, _, err := appserver.BuildWorkspaceRuntime(context.Background(), appserver.RuntimeOptions{WorkspaceRoot: root, Defaults: &execconfig.Defaults{Reporting: execconfig.ReportingDefaults{Enabled: true}}, SkipRegistryInitialize: true, ConfigureBuilder: configure})
	require.NoError(t, err)
	defer rt.Close(context.Background())
	require.NotNil(t, rt.PrimitiveWindows, "standard configured builder must construct the composite before consumers capture it")
	remote := rt.PrimitiveWindows.Remote
	require.NotNil(t, remote)
	require.NotNil(t, remote.Gateway)
	require.NotNil(t, remote.TargetProof)
	require.NotNil(t, remote.Admission)
	var captured forge.WindowDefinitionCatalog
	rt.UIBridge.ConfigureWindowCatalog(func(catalog forge.WindowDefinitionCatalog) forge.WindowDefinitionCatalog {
		captured = catalog
		return catalog
	})
	require.Same(t, rt.PrimitiveWindows, captured)
	handler := metadata.NewMetadataHandler(rt.Defaults, rt.Store, "fixture")
	handler.SetWindowResourceCatalog(rt.UIBridge)
	handler.SetAuthorizationRuntimes(nil, nil)
	handler.SetLayoutDefault([]byte("version: 1\nid: fixture\napplications: []\n"))
	windowHandler := uihttp.NewEmbeddedHandlerWithAuthorization("embed://localhost/window", nil, rt.AuthorizationPolicy, nil, uihttp.WithWindowResourceProvider(rt.UIBridge))
	require.NoError(t, registration.Bind(context.Background(), rt))
	require.Same(t, remote, rt.PrimitiveWindows.Remote, "late bind must retain owning gateway, admission and target proof")
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": authority.URL, "aud": "fixture", "sub": "fixture-user", "account": "fixture-account", "exp": time.Now().Add(time.Minute).Unix()})
	token.Header["kid"] = "fixture"
	signed, err := token.SignedString(key)
	require.NoError(t, err)
	ctx := svcauth.InjectTokens(svcauth.InjectUser(context.Background(), "fixture-user"), &scyauth.Token{IDToken: signed})
	got, err := captured.Get(ctx, &forge.WindowDefinitionGetInput{WindowID: "chat/new", Target: &types.WindowTarget{Platform: "web", FormFactor: "desktop"}})
	require.NoError(t, err)
	require.NotNil(t, got.Definition)
	require.Equal(t, "window://agently/chat-new", got.Definition.Resource.URI)
	bridge, err := rt.UIBridge.WindowDefinitionGet(ctx, &forge.WindowDefinitionGetInput{WindowID: "chat/new", Target: &types.WindowTarget{Platform: "web", FormFactor: "desktop"}})
	require.NoError(t, err)
	require.Equal(t, got.Definition.Resource.URI, bridge.Definition.Resource.URI)
	listed, listErr := captured.List(ctx, &forge.WindowDefinitionListInput{})
	require.NoError(t, listErr)
	require.Len(t, listed.Windows, 2, "local windows must occur exactly once in the composite")

	for _, path := range []string{"/window/chat%2Fnew", "/window/window%3A%2F%2Fagently%2Fchat-new"} {
		request := httptest.NewRequest(http.MethodGet, path+"?platform=web&formFactor=desktop", nil).WithContext(ctx)
		request.Header.Set("Authorization", "Bearer "+signed)
		response := httptest.NewRecorder()
		windowHandler.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, "HTTP window alias and canonical URI must use the captured catalog")
		var envelope struct {
			Status string        `json:"status"`
			Data   *types.Window `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
		require.Equal(t, "ok", envelope.Status)
		require.NotNil(t, envelope.Data)
		require.NotNil(t, envelope.Data.Resource)
		require.Equal(t, "window://agently/chat-new", envelope.Data.Resource.URI)
	}

	_, err = handler.Layout(ctx, &metadata.LayoutRequest{})
	require.NoError(t, err, "captured workspace consumer must use the late-bound catalog")
	// A stale browser credential cannot borrow the successful fresh actor catalog.
	expiredJWT := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": authority.URL, "aud": "fixture", "sub": "fixture-user", "account": "fixture-account", "iat": time.Now().Add(-2 * time.Hour).Unix(), "exp": time.Now().Add(-time.Hour).Unix()})
	expiredJWT.Header["kid"] = "fixture"
	expired, signErr := expiredJWT.SignedString(key)
	require.NoError(t, signErr)
	expiredContext := svcauth.InjectTokens(svcauth.InjectUser(context.Background(), "fixture-user"), &scyauth.Token{IDToken: expired})
	deniedRequest := httptest.NewRequest(http.MethodGet, "/window/chat%2Fnew?platform=web&formFactor=desktop", nil).WithContext(expiredContext)
	deniedResponse := httptest.NewRecorder()
	windowHandler.ServeHTTP(deniedResponse, deniedRequest)
	require.NotEqual(t, http.StatusOK, deniedResponse.Code)
	// A subsequent normal verified request remains usable on the same gateway.
	freshRequest := httptest.NewRequest(http.MethodGet, "/window/chat%2Fnew?platform=web&formFactor=desktop", nil).WithContext(ctx)
	freshResponse := httptest.NewRecorder()
	windowHandler.ServeHTTP(freshResponse, freshRequest)
	require.Equal(t, http.StatusOK, freshResponse.Code)

}
