package agently

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/viant/afs"
	"github.com/viant/agently-core/app/executor"
	ds "github.com/viant/agently-core/protocol/datasource"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/runtime/requestctx"
	auth "github.com/viant/agently-core/service/auth"
	datasources "github.com/viant/agently-core/service/datasource"
	forge "github.com/viant/agently-core/service/primitiveprovider"
	resources "github.com/viant/agently-core/service/resource"
	"github.com/viant/agently-core/service/ui/permittedview"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/authz/oauth"
	"github.com/viant/datly/standalone"
	"github.com/viant/forge/backend/service/meta"
	"github.com/viant/forge/backend/types"
)

type nativeDeliveryUsers struct {
	auth.UserService
	record *auth.User
}

func (u *nativeDeliveryUsers) GetByID(_ context.Context, id string) (*auth.User, error) {
	if u.record == nil || u.record.ID != id {
		return nil, nil
	}
	copy := *u.record
	return &copy, nil
}
func (u *nativeDeliveryUsers) GetBySubjectAndProvider(_ context.Context, subject, provider string) (*auth.User, error) {
	if u.record == nil || u.record.Subject != subject || u.record.Provider != provider {
		return nil, nil
	}
	copy := *u.record
	return &copy, nil
}

type nativeDeliveryIngress struct{}
type nativeDeliveryOperation struct{}

type nativeDeliveryFixture struct {
	host       *nativeWindowHost
	principal  gating.Principal
	users      *nativeDeliveryUsers
	ctx        context.Context
	window     *types.Window
	parameters map[string]any
	root       string
	business   permittedview.Resolver
}

func newNativeDeliveryFixture(t *testing.T) *nativeDeliveryFixture {
	t.Helper()
	root := t.TempDir()
	file := filepath.Join(root, "extension/forge/windows/edit.yaml")
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(`namespace: fixture
parameters: [{name: Selected, type: "int[]"}]
view: {content: {id: original}}
dataSource:
  rows: {selectors: {data: data}, backend: {kind: mcp_tool, service: arbitrary, method: Read}}
  change: {selectors: {data: data}, backend: {kind: mcp_tool, service: arbitrary, method: Patch}}
  details:
    selectors: {data: data}
    uniqueKey: [{field: wireKey}]
    parameters: [{name: Object, in: windowForm, location: Selected}]
    backend: {kind: mcp_tool, service: arbitrary, method: ById}
`), 0600); err != nil {
		t.Fatal(err)
	}
	for id, method := range map[string]string{"rows": "Read", "change": "Patch", "details": "ById"} {
		path := filepath.Join(root, "extension/forge/datasources", id+".yaml")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		body := "id: " + id + "\ncardinality: collection\nselectors: {data: data}\nbackend: {kind: mcp_tool, service: arbitrary, method: " + method + "}\n"
		if id == "details" {
			body += "uniqueKey: [{field: wireKey}]\nparameters: [{name: Object, in: windowForm, location: Selected}]\n"
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	uri := "window://tenant/edit"
	f := &nativeDeliveryFixture{root: root, principal: gating.Principal{Facts: authz.Facts{Subject: "external-subject", Issuer: "https://issuer.example", Tenant: "tenant", ValidUntil: time.Now().Add(time.Hour)}, AccountID: "account", IdentityRevision: "credential-1"}, users: &nativeDeliveryUsers{record: &auth.User{ID: "canonical-user", Subject: "external-subject", Provider: "configured-oauth"}}, parameters: map[string]any{"Selected": []int{42}, "Context": map[string]any{"opaque": "item:alpha", "exact": json.Number("9007199254740993")}}}
	f.ctx = context.WithValue(context.WithValue(context.Background(), nativeDeliveryIngress{}, "canonical-user"), nativeDeliveryOperation{}, "original-operation")
	config := NativeWindowHostConfig{ProviderIdentity: "native", Windows: []windowloader.ResourceBinding{{WindowKey: "edit", URI: uri}}, Bootstrap: []WindowBootstrapBinding{{URI: uri, WindowKey: "edit", ResourceType: "widget", Parameter: "Selected", DataSource: "details", Input: "Object", RowIdentity: "wireKey", IdentityType: "integer"}}, ToolCapabilities: map[string]string{"arbitrary:Read": "read", "arbitrary:Patch": "write", "arbitrary:ById": "read"}, ExecutionBindings: []NativeWindowExecutionBinding{{URI: uri, DataSource: "change", IdentitySelectors: []string{"Objects.*.Id"}}}}
	snapshot, err := newNativeWindowHostSnapshot(f.ctx, root, config)
	if err != nil {
		t.Fatal(err)
	}
	authority := &nativeWindowAuthority{principal: func(context.Context) (gating.Principal, error) { return f.principal, nil }, allowed: map[string]resources.LocalResourceBinding{uri: {URI: uri}}}
	proof, err := types.NewWindowTargetHMAC([]byte(strings.Repeat("t", 32)))
	if err != nil {
		t.Fatal(err)
	}
	signer, err := identity.NewSelectionHMAC([]byte(strings.Repeat("s", 32)))
	if err != nil {
		t.Fatal(err)
	}
	delivery := &nativeWindowDelivery{authority: authority, signer: signer, target: proof}
	if err := delivery.bindUsers(f.users); err != nil {
		t.Fatal(err)
	}
	// This simulates only the trusted ingress getter. Wire parameters never
	// populate this private context key or the actual canonical directory.
	delivery.canonical = func(ctx context.Context) (string, string) {
		id, _ := ctx.Value(nativeDeliveryIngress{}).(string)
		return id, "jwt"
	}
	delivery.resolve = func(_ context.Context, key string) (*identity.ResourceResolver, identity.ResourceRef, error) {
		if key != uri && key != "edit" {
			return nil, identity.ResourceRef{}, identity.ErrResourceDenied
		}
		return &identity.ResourceResolver{Source: snapshot, Policy: &nativeDeliveryPolicy{base: &nativeWindowPolicy{authority: authority}, delivery: delivery}, ProviderIdentity: "native"}, identity.ResourceRef{URI: uri}, nil
	}
	catalog, err := forge.NewMetadataWindowCatalog(meta.New(afs.New(), root), root, []forge.SavedWindow{{WindowDefinitionSummary: forge.WindowDefinitionSummary{WindowID: "edit", ResourceURI: uri}, Key: "edit"}}, forge.WithWindowResourceResolver(delivery.resolve), forge.WithWindowTargetProof(proof))
	if err != nil {
		t.Fatal(err)
	}
	f.host = &nativeWindowHost{config: config, source: snapshot, authority: authority, catalog: catalog, delivery: delivery}
	got, err := catalog.Get(f.ctx, &forge.WindowDefinitionGetInput{WindowID: uri, Target: &types.WindowTarget{Platform: "web"}})
	if err != nil {
		t.Fatal(err)
	}
	f.window = got.Definition
	return f
}

func (f *nativeDeliveryFixture) bind(t *testing.T) *identity.SelectionBinding {
	t.Helper()
	service := forge.NewService(&forge.Config{WindowDefinitions: f.host.catalog, WindowSelectionEmitter: f.host.delivery.emitSelection})
	output, err := service.EmitWindowSelection(f.ctx, f.window, f.parameters)
	if err != nil {
		t.Fatal(err)
	}
	if output.SelectionBinding == nil {
		t.Fatal("successful original delivery omitted selection binding")
	}
	return output.SelectionBinding
}

func (f *nativeDeliveryFixture) service(t *testing.T, execute datasources.ToolExecutor) *datasources.Service {
	t.Helper()
	d := f.host.delivery
	return datasources.New(datasources.Options{PermissionResolver: f.business, NewDeliveryPreparer: d.prepare, ResolveResource: func(ctx context.Context, pin identity.ResolvedResource) (*identity.ResolvedResource, error) {
		resolver, _, err := d.resolve(ctx, pin.URI)
		if err != nil {
			return nil, err
		}
		_, fresh, err := resolver.ReadResolved(ctx, pin)
		return fresh, err
	}, ResolveDefinition: func(ctx context.Context, pin identity.ResolvedResource, target *types.WindowTarget, id string) (*ds.DataSource, error) {
		resolver, _, err := d.resolve(ctx, pin.URI)
		if err != nil {
			return nil, err
		}
		raw, _, err := resolver.ReadResolved(ctx, pin)
		if err != nil {
			return nil, err
		}
		variant, err := types.SelectWindowResource(raw, target)
		if err != nil {
			return nil, err
		}
		if err := d.target.Verify(ctx, pin, *target, variant.Fingerprint, target.SelectionToken); err != nil {
			return nil, err
		}
		var definition ds.DataSource
		decoder := json.NewDecoder(strings.NewReader(string(variant.DataSources[id])))
		decoder.UseNumber()
		if decoder.Decode(&definition) != nil || definition.ID != id {
			return nil, identity.ErrResourceDenied
		}
		return &definition, nil
	}, AuthorizeDefinition: f.host.authorize, DenialRootRead: f.host.authorizeRootRead, Executor: execute})
}

func TestNativeNewDeliveryValidAndExpiredSelection(t *testing.T) {
	for _, state := range []string{"valid", "expired", "credential rotation"} {
		t.Run(state, func(t *testing.T) {
			f := newNativeDeliveryFixture(t)
			if state == "expired" {
				f.window.Resource.ValidUntil = time.Now().Add(100 * time.Millisecond)
				variantRaw, err := f.host.source.ReadCandidate(f.ctx, mustNativeURI(f.window.Resource.URI), f.window.Resource.ResourceCandidate)
				if err != nil {
					t.Fatal(err)
				}
				variant, err := types.SelectWindowResource(variantRaw, f.window.ResourceTarget)
				if err != nil {
					t.Fatal(err)
				}
				f.window.ResourceTarget.SelectionToken, err = f.host.delivery.target.Sign(f.ctx, *f.window.Resource, *f.window.ResourceTarget, variant.Fingerprint)
				if err != nil {
					t.Fatal(err)
				}
			}
			binding := f.bind(t)
			original, originalTarget := *f.window.Resource, *f.window.ResourceTarget
			if state == "expired" {
				time.Sleep(110 * time.Millisecond)
			}
			if state == "credential rotation" {
				f.principal.IdentityRevision = "credential-2"
			}
			ctx, prepared, err := f.host.delivery.prepare(f.ctx, datasources.OriginalDelivery{Resource: &original, Target: &originalTarget, SelectionBinding: binding, WindowParameters: f.parameters})
			if err != nil {
				t.Fatal(err)
			}
			if ctx.Value(nativeDeliveryOperation{}) != "original-operation" || prepared.Resource.ResourceCandidate != original.ResourceCandidate {
				t.Fatal("new delivery replaced context or original candidate")
			}
			if state == "valid" && (!nativeJSONEqual(prepared.Resource, original) || !nativeJSONEqual(prepared.Target, originalTarget)) {
				t.Fatal("valid artifact changed")
			}
			if state != "valid" && (!prepared.Resource.ValidUntil.After(time.Now()) || prepared.Target.SelectionToken == originalTarget.SelectionToken) {
				t.Fatal("new lease/target was not freshly prepared")
			}
			calls := 0
			service := f.service(t, nativePermissionExecutor(func(ctx context.Context, name string, args map[string]any) (string, error) {
				calls++
				if ctx.Value(nativeDeliveryOperation{}) != "original-operation" {
					t.Fatal("MCP credentials/context lost")
				}
				return `{"data":[{"id":42}]}`, nil
			}))
			_, err = service.Fetch(f.ctx, "change", map[string]any{"Objects": []map[string]any{{"Id": 42}}}, datasources.FetchOptions{Resource: &original, Target: &originalTarget, SelectionBinding: binding, WindowParameters: f.parameters})
			if err != nil || calls != 1 {
				t.Fatalf("dispatch=%d error=%v", calls, err)
			}
		})
	}
}

func TestNativeNewDeliveryRejectsTamperingAndRevokedMappingsBeforeDispatch(t *testing.T) {
	for _, change := range []string{"selection", "actor", "issuer", "tenant", "account", "disabled", "mapping", "canonical", "namespace", "candidate", "signature", "target", "missing binding", "canceled", "source drift", "association", "window decision", "held dispatch", "prepared inheritance", "namespace revoked", "nested selection"} {
		t.Run(change, func(t *testing.T) {
			f := newNativeDeliveryFixture(t)
			binding := f.bind(t)
			pin := *f.window.Resource
			target := *f.window.ResourceTarget
			params := f.parameters
			ctx := f.ctx
			inputs := map[string]any{"Objects": []map[string]any{{"Id": 42}}}
			switch change {
			case "selection":
				params = map[string]any{"Selected": []int{99}}
			case "actor":
				f.principal.Facts.Subject = "another"
			case "issuer":
				f.principal.Facts.Issuer = "https://another.example"
			case "tenant":
				f.principal.Facts.Tenant = "another"
			case "account":
				f.principal.AccountID = "another"
			case "disabled":
				f.users.record.Disabled = true
			case "mapping":
				f.users.record.ID = "reassigned"
			case "canonical":
				ctx = context.WithValue(ctx, nativeDeliveryIngress{}, "other-canonical")
			case "namespace":
				pin.URI = "window://foreign/edit"
			case "candidate":
				pin.ContentFingerprint = strings.Repeat("f", 64)
			case "signature":
				binding.Token = "forged"
			case "target":
				target.Platform = "mobile"
			case "missing binding":
				binding = nil
			case "canceled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			case "association":
				var err error
				ctx, err = requestctx.WithResourceReadAssociation(ctx, &pin, &target)
				if err != nil {
					t.Fatal(err)
				}
			case "window decision":
				ctx, _ = requestctx.WithWindowReadDecision(ctx)
			case "held dispatch":
				ctx = requestctx.WithResourceReadDispatch(ctx, nativeHeldDispatch{})
			case "prepared inheritance":
				var err error
				ctx, _, err = f.host.delivery.prepare(ctx, datasources.OriginalDelivery{Resource: &pin, Target: &target, SelectionBinding: binding, WindowParameters: params})
				if err != nil {
					t.Fatal(err)
				}
			case "namespace revoked":
				delete(f.host.authority.allowed, pin.URI)
			case "nested selection":
				params = map[string]any{"Selected": []int{42}, "Context": map[string]any{"opaque": "changed", "exact": json.Number("9007199254740993")}}
			case "source drift":
				p := filepath.Join(f.root, "extension/forge/windows/edit.yaml")
				raw, _ := os.ReadFile(p)
				if err := os.WriteFile(p, append(raw, []byte("\n# drift\n")...), 0600); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			service := f.service(t, nativePermissionExecutor(func(context.Context, string, map[string]any) (string, error) { calls++; return `{"data":[]}`, nil }))
			_, err := service.Fetch(ctx, "change", inputs, datasources.FetchOptions{Resource: &pin, Target: &target, SelectionBinding: binding, WindowParameters: params})
			if err == nil || calls != 0 {
				t.Fatalf("tampering/revocation dispatched=%d error=%v", calls, err)
			}
		})
	}
}

func TestNativeManagingMCPDenialRemainsDatasetScopedAndRootFailureDoesNot(t *testing.T) {
	f := newNativeDeliveryFixture(t)
	binding := f.bind(t)
	calls := 0
	service := f.service(t, nativePermissionExecutor(func(context.Context, string, map[string]any) (string, error) {
		calls++
		return "", &ds.UpstreamAuthorizationError{Status: 403}
	}))
	_, err := service.Fetch(f.ctx, "rows", nil, datasources.FetchOptions{Resource: f.window.Resource, Target: f.window.ResourceTarget, SelectionBinding: binding, WindowParameters: f.parameters})
	if !errors.Is(err, ds.ErrDatasourceDenied) || calls != 1 {
		t.Fatalf("managing403 scope/dispatch=%d err=%v", calls, err)
	}
	f.users.record.Disabled = true
	_, err = service.Fetch(f.ctx, "rows", nil, datasources.FetchOptions{Resource: f.window.Resource, Target: f.window.ResourceTarget, SelectionBinding: binding, WindowParameters: f.parameters})
	if errors.Is(err, ds.ErrDatasourceDenied) || err == nil || calls != 1 {
		t.Fatalf("root denial was relabeled or dispatched: %v", err)
	}
}

type nativeHeldDispatch struct{}

func (nativeHeldDispatch) BeforeDispatch(context.Context) error { return nil }
func (nativeHeldDispatch) AfterDispatch(context.Context) error  { return nil }

func TestNativeRuntimeBinderFailsClosedAndBindsActualRuntimeDirectoryOnce(t *testing.T) {
	f := newNativeDeliveryFixture(t)
	// The constructor installs the actual Serve runtime binder. Its identity
	// verifier is configured, but this test never fetches a token or user record.
	config := f.host.config
	config.SchemaVersion = 1
	config.Bootstrap = nil
	config.Identity = oauth.HTTPIdentityConfig{Kind: "oidc-userinfo", Issuer: "https://issuer.example", Audience: "workspace", Algorithms: []string{"RS256"}, UserInfoURL: "https://issuer.example/identity", JWKSURL: "https://issuer.example/keys", JWKSRefreshInterval: "1m", FactLease: "30s", TenantForAllAccounts: "tenant", Mapping: oauth.HTTPIdentityMapping{Subject: "sub", AccountID: "account"}}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "native.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	options, err := configureNativeWindowHost(ServeOptions{WorkspacePath: f.root, NativeWindowConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if options.nativeWindowRuntimeBinder == nil || options.ConfigureBuilder == nil || options.WindowOpenBootstrapFactory == nil {
		t.Fatal("native startup seams missing")
	}
	if err := bindNativeWindowRuntime(options, nil); !errors.Is(err, identity.ErrResourceDenied) {
		t.Fatal("missing runtime accepted", err)
	}
	if err := bindNativeWindowRuntime(options, &executor.Runtime{}); !errors.Is(err, identity.ErrResourceDenied) {
		t.Fatal("missing actual directory backing accepted", err)
	}
	// This is a directory invoker placeholder, not a started runtime or database.
	// NewDatlyUserService binds the same actual Native object as auth.NewRuntime.
	runtime := &executor.Runtime{Native: &standalone.Server{}}
	if err := bindNativeWindowRuntime(options, runtime); err != nil {
		t.Fatal(err)
	}
	if err := bindNativeWindowRuntime(options, runtime); !errors.Is(err, identity.ErrResourceDenied) {
		t.Fatal("directory rebound after activation", err)
	}
	if err := bindNativeWindowRuntime(ServeOptions{}, nil); err != nil {
		t.Fatal("ordinary host startup changed", err)
	}
}

// The managing endpoint resolves ownership from its own directory, never from
// the caller's account argument or the proxy's original window selection.
type nativeManagingAccountExecutor struct {
	account           string
	owners            map[string]string
	calls, executions int
	last              map[string]any
}

func (m *nativeManagingAccountExecutor) Execute(_ context.Context, name string, args map[string]any) (string, error) {
	m.calls++
	raw, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	m.last = nil
	if err = decoder.Decode(&m.last); err != nil {
		return "", err
	}
	var ids []string
	switch name {
	case "arbitrary:ById":
		value, ok := args["Object"].(string)
		if !ok {
			return "", &ds.UpstreamAuthorizationError{Status: 403}
		}
		ids = append(ids, value)
	case "arbitrary:Patch":
		objects, ok := m.last["Objects"].([]any)
		if !ok {
			return "", &ds.UpstreamAuthorizationError{Status: 403}
		}
		for _, value := range objects {
			object, ok := value.(map[string]any)
			if !ok {
				return "", &ds.UpstreamAuthorizationError{Status: 403}
			}
			for _, field := range []string{"Id", "ParentId"} {
				id, ok := object[field].(string)
				if !ok {
					return "", &ds.UpstreamAuthorizationError{Status: 403}
				}
				ids = append(ids, id)
			}
		}
	default:
		return "", &ds.UpstreamAuthorizationError{Status: 403}
	}
	if len(ids) == 0 {
		return "", &ds.UpstreamAuthorizationError{Status: 403}
	}
	for _, id := range ids {
		if m.owners[id] != m.account {
			return "", &ds.UpstreamAuthorizationError{Status: 403}
		}
	}
	m.executions++
	return `{"data":[{"wireKey":"owned-other","id":"owned-other"}]}`, nil
}

func TestNativeManagingMCPOwnsMutableArgumentsAndAccountAuthorization(t *testing.T) {
	f := newNativeDeliveryFixture(t)
	f.host.config.ToolCapabilities = nil
	f.host.config.ExecutionBindings[0].ParentSelectors = map[string]string{"Objects.*.ParentId": "parentKey"}
	businessCalls := 0
	f.business = &permittedview.MCPResolver{ToolName: "window:BusinessOpen", Executor: nativePermissionExecutor(func(context.Context, string, map[string]any) (string, error) {
		businessCalls++
		return "", errors.New("unexpected datasource business preflight")
	})}
	binding := f.bind(t)
	manager := &nativeManagingAccountExecutor{account: "account", owners: map[string]string{"owned-other": "account", "owned-parent": "account", "foreign": "other-account"}}
	service := f.service(t, manager)
	f.host.datasource = nativeDeliverySDKBackend{service: service}
	opts := datasources.FetchOptions{Resource: f.window.Resource, Target: f.window.ResourceTarget, SelectionBinding: binding, WindowParameters: f.parameters}
	for _, tc := range []struct {
		id   string
		args map[string]any
	}{
		{"details", map[string]any{"Object": "owned-other", "filters": map[string]any{"active": false, "limit": json.Number("9007199254740993")}}},
		{"change", map[string]any{"Objects": []map[string]any{{"Id": "owned-other", "ParentId": "owned-parent"}}, "filters": map[string]any{"enabled": false}}},
	} {
		before, _ := json.Marshal(tc.args)
		if _, err := service.Fetch(f.ctx, tc.id, tc.args, opts); err != nil {
			t.Fatal(err)
		}
		if !nativeJSONEqual(manager.last, tc.args) {
			t.Fatal("proxy changed managing arguments")
		}
		after, _ := json.Marshal(tc.args)
		if string(before) != string(after) {
			t.Fatal("caller arguments changed")
		}
	}
	if manager.calls != 2 || manager.executions != 2 || businessCalls != 0 {
		t.Fatalf("calls=%d executions=%d business=%d", manager.calls, manager.executions, businessCalls)
	}
	for _, args := range []map[string]any{
		{"Objects": []map[string]any{{"Id": "foreign", "ParentId": "owned-parent"}}, "accountId": "account"},
		{"Objects": []map[string]any{{"Id": "owned-other", "ParentId": "foreign"}}},
	} {
		before := manager.calls
		if _, err := service.Fetch(f.ctx, "change", args, opts); !errors.Is(err, ds.ErrDatasourceDenied) {
			t.Fatalf("managing cross-account denial scope: %v", err)
		}
		if manager.calls != before+1 || manager.executions != 2 || businessCalls != 0 {
			t.Fatal("proxy preflight, parent read, execution or retry on denial")
		}
	}
}
