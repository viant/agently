package agently

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/viant/afs"
	ui "github.com/viant/agently-core/adapter/http/ui"
	identity "github.com/viant/agently-core/protocol/resource"
	forge "github.com/viant/agently-core/service/primitiveprovider"
	resources "github.com/viant/agently-core/service/resource"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/forge/backend/service/meta"
	"github.com/viant/forge/backend/types"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeclaredLinkedChatShellIsPinnedAuthenticatedAndHasNoWorkspaceFallback(t *testing.T) {
	binding := windowloader.ResourceBinding{WindowKey: "chat/new", CatalogID: "chat/new", URI: "window://agently/chat-new"}
	source, e := newInternalWindowSnapshot(context.Background(), "", "", &hostAuthorizationFile{FrameworkWindowResources: []windowloader.ResourceBinding{binding}})
	if e != nil {
		t.Fatal(e)
	}
	authenticated := true
	principal := gating.Principal{Facts: authz.Facts{Subject: "fixture-user", Issuer: "fixture-issuer", Tenant: "fixture-tenant", ValidUntil: time.Now().Add(time.Minute)}, AccountID: "fixture-account", IdentityRevision: "fixture-revision"}
	authority := &nativeWindowAuthority{allowed: map[string]resources.LocalResourceBinding{binding.URI: {URI: binding.URI}}, principal: func(context.Context) (gating.Principal, error) {
		if !authenticated {
			return gating.Principal{}, identity.ErrResourceDenied
		}
		return principal, nil
	}}
	resolve := func(_ context.Context, key string) (*identity.ResourceResolver, identity.ResourceRef, error) {
		if key != binding.URI && key != binding.WindowKey {
			return nil, identity.ResourceRef{}, identity.ErrResourceDenied
		}
		return &identity.ResourceResolver{Source: source, Policy: &nativeWindowPolicy{authority: authority}, ProviderIdentity: "linked-fixture"}, identity.ResourceRef{URI: binding.URI}, nil
	}
	key := make([]byte, 32)
	rand.Read(key)
	proof, e := types.NewWindowTargetHMAC(key)
	if e != nil {
		t.Fatal(e)
	}
	catalog, e := forge.NewMetadataWindowCatalog(meta.New(afs.New(), t.TempDir()), "embed://localhost/", []forge.SavedWindow{{WindowDefinitionSummary: forge.WindowDefinitionSummary{WindowID: binding.WindowKey, ResourceURI: binding.URI}, Key: binding.WindowKey}}, forge.WithWindowResourceResolver(resolve), forge.WithWindowTargetProof(proof))
	if e != nil {
		t.Fatal(e)
	}
	bridge := forge.NewService(&forge.Config{WindowDefinitions: catalog})
	handler := ui.NewEmbeddedHandlerWithAuthorization("embed://localhost/", nil, nil, nil, ui.WithWindowResourceProvider(bridge))
	request := func(key string) *httptest.ResponseRecorder {
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, httptest.NewRequest("GET", "/window/"+key+"?platform=web", nil))
		return out
	}
	success := request("chat/new")
	if success.Code != 200 {
		t.Fatal(success.Code, success.Body.String())
	}
	var envelope struct {
		Data types.Window `json:"data"`
	}
	if json.Unmarshal(success.Body.Bytes(), &envelope) != nil || envelope.Data.Resource == nil || envelope.Data.Resource.URI != binding.URI || envelope.Data.ResourceTarget == nil || envelope.Data.ResourceTarget.SelectionToken == "" {
		t.Fatal("linked shell lost server-owned pin/target", success.Body.String())
	}
	// Shell's HTTP metadata/conversation services retain their original paths;
	// registration adds static definitions, no tool/backend capability mapping.
	if len(envelope.Data.DataSource) == 0 {
		t.Fatal("original shell descriptors lost")
	}
	for _, ds := range envelope.Data.DataSource {
		if ds.Service != nil && ds.Service.URI == "" {
			t.Fatal("linked service changed")
		}
	}
	if request("chat/unregistered").Code != 404 {
		t.Fatal("unknown framework fallback admitted")
	}
	authenticated = false
	if request("chat/new").Code != 404 {
		t.Fatal("unauthenticated linked shell admitted")
	}
}

func TestNativeWindowHostComposesFrameworkClosureAndValidatesExecutionDeclarations(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		p := filepath.Join(root, "extension/forge", name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("windows/local.yaml", "namespace: fixture\nview: {content: {id: local}}\n")
	write("lookups/order.yaml", "id: order\nbindings:\n  - named: {name: order}\n    lookup: {dialogId: adOrderPicker, dataSource: ad_order_lookup}\n")
	write("dialogs/order.yaml", "id: adOrderPicker\ndataSourceRef: ad_order_lookup\ncontent: {id: originalOrderTable, dataSourceRef: ad_order_lookup}\n")
	write("datasources/order.yaml", "id: ad_order_lookup\ncardinality: collection\nbackend: {kind: mcpTool, ownership: provider, service: arbitrary, method: fetch}\n")
	binding := windowloader.ResourceBinding{WindowKey: "chat/new", URI: "window://agently/chat-new"}
	config := NativeWindowHostConfig{Windows: []windowloader.ResourceBinding{{WindowKey: "local", URI: "window://tenant/local"}}, FrameworkWindows: []windowloader.ResourceBinding{binding}, ExecutionBindings: []NativeWindowExecutionBinding{{URI: binding.URI, DataSource: "ad_order_lookup", IdentitySelectors: []string{"Objects.*.Id"}}}}
	snapshot, err := newNativeWindowHostSnapshot(context.Background(), root, config)
	if err != nil {
		t.Fatal(err)
	}
	uri, _ := identity.ParseResourceURI(binding.URI)
	candidates, err := snapshot.Candidates(context.Background(), uri)
	if err != nil || len(candidates) != 1 {
		t.Fatal(err, candidates)
	}
	raw, err := snapshot.ReadCandidate(context.Background(), uri, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	variant, err := types.SelectWindowResource(raw, &types.WindowTarget{Platform: "web"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, dialog := range variant.Window.Dialogs {
		if dialog.Id == "adOrderPicker" {
			found = true
		}
	}
	if !found || len(variant.DataSources["ad_order_lookup"]) == 0 || variant.Window.ResourceDependencies["ad_order_lookup"] != identity.ContentFingerprint(variant.DataSources["ad_order_lookup"]) {
		t.Fatal("framework lost original picker closure")
	}
	var descriptor struct {
		Backend struct {
			Service string
			Method  string
		}
	}
	if err := json.Unmarshal(variant.DataSources["ad_order_lookup"], &descriptor); err != nil || descriptor.Backend.Service != "arbitrary" || descriptor.Backend.Method != "fetch" {
		t.Fatal("authored MCP backend changed", err)
	}
	config.ExecutionBindings[0].DataSource = "typo"
	if _, err := newNativeWindowHostSnapshot(context.Background(), root, config); err != nil {
		t.Fatal("legacy execution declaration affected artifact startup", err)
	}
	config.ExecutionBindings[0].DataSource = "ad_order_lookup"
	config.ExecutionBindings[0].URI = "window://tenant/local"
	if _, err := newNativeWindowHostSnapshot(context.Background(), root, config); err != nil {
		t.Fatal("legacy execution URI affected artifact startup", err)
	}
	write("dialogs/order.yaml", "id: adOrderPicker\ndataSourceRef: ad_order_lookup\ncontent: {id: drifted}\n")
	if _, err := snapshot.ReadCandidate(context.Background(), uri, candidates[0]); !errors.Is(err, identity.ErrResourceStale) {
		t.Fatal("framework source drift admitted", err)
	}
}

func TestSerializedLinkedWindowBasePreservesExactOuterParameters(t *testing.T) {
	for _, value := range []any{int64(9007199254740993), json.Number("9007199254740993")} {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "extension/forge"), 0700); err != nil {
			t.Fatal(err)
		}
		authored := &types.Window{Parameters: []map[string]any{{"name": "Exact", "default": value}}, View: types.View{Content: &types.Container{ID: "linked"}}}
		linkedBytes, err := json.Marshal(authored)
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		base := serializedWindowBase(func(context.Context, *types.WindowTarget) (json.RawMessage, error) { calls++; return linkedBytes, nil })
		binding := windowloader.ResourceBinding{WindowKey: "linked", URI: "window://test/linked"}
		snapshot, err := resources.NewNativeAssetSnapshot(context.Background(), root, []resources.NativeSnapshotDefinition{{URI: binding.URI, FormatVersion: 2, File: "extension/forge", Load: resources.NativeComposedWindowLoader(binding, base, nil)}}, resources.NativeSnapshotOptions{ImmutableUntilRestart: true})
		if err != nil {
			t.Fatal(err)
		}
		uri, _ := identity.ParseResourceURI(binding.URI)
		candidates, err := snapshot.Candidates(context.Background(), uri)
		if err != nil || len(candidates) != 1 {
			t.Fatal(err, candidates)
		}
		raw, err := snapshot.ReadCandidate(context.Background(), uri, candidates[0])
		if err != nil {
			t.Fatal(err)
		}
		var envelope types.WindowResourceEnvelope
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if err := decoder.Decode(&envelope); err != nil {
			t.Fatal(err)
		}
		if calls == 0 || len(envelope.Variants) == 0 {
			t.Fatal("serialized linked loader was not compiled")
		}
		for _, variant := range envelope.Variants {
			exact, ok := variant.Window.Parameters[0]["default"].(json.Number)
			if !ok || exact.String() != "9007199254740993" {
				t.Fatalf("outer parameter rounded: %v", variant.Window.Parameters[0]["default"])
			}
		}
	}
}
func TestTrustedLinkedWindowDecodeRejectsIncompleteOrMultipleJSON(t *testing.T) {
	for _, raw := range []string{`{"parameters":`, `{}` + `{}`, `{}` + ` garbage`} {
		base := serializedWindowBase(func(context.Context, *types.WindowTarget) (json.RawMessage, error) { return json.RawMessage(raw), nil })
		if _, err := base(context.Background(), nil); err == nil {
			t.Fatalf("invalid linked JSON admitted: %s", raw)
		}
	}
	if _, err := decodeTrustedWindow(json.RawMessage("{} \n")); err != nil {
		t.Fatal(err)
	}
}
