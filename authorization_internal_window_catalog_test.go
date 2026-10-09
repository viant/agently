package agently

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corecfg "github.com/viant/agently-core/protocol/mcp/config"
	"github.com/viant/agently-core/protocol/mcp/manager"
	"github.com/viant/agently-core/protocol/primitive"
	identity "github.com/viant/agently-core/protocol/resource"
	primitiveprovider "github.com/viant/agently-core/service/primitiveprovider"
	resources "github.com/viant/agently-core/service/resource"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/forge/backend/types"
	"github.com/viant/mcp-protocol/schema"
	mcpclient "github.com/viant/mcp/client"
)

type internalWindowTestOptions map[string]*corecfg.MCPClient

func (p internalWindowTestOptions) Options(_ context.Context, name string) (*corecfg.MCPClient, error) {
	return p[name], nil
}
func (p internalWindowTestOptions) Names(context.Context) ([]string, error) {
	var names []string
	for name := range p {
		names = append(names, name)
	}
	return names, nil
}

type internalWindowTestProvider struct {
	mcpclient.Interface
	owner      string
	definition map[string]json.RawMessage
	lease      time.Time
	mu         sync.Mutex
	getCalls   int
	listCalls  int
	denied     bool
}

func (p *internalWindowTestProvider) ListTools(context.Context, *string, ...mcpclient.RequestOption) (*schema.ListToolsResult, error) {
	meta := map[string]any{primitive.AuthoringExtension: map[string]any{"version": 1, "providerIdentity": p.owner, "transport": "tools/call"}}
	return &schema.ListToolsResult{Tools: []schema.Tool{
		{Name: "namespaces/list", Meta: meta},
		{Name: "namespaces/get", Meta: meta},
		{Name: "windows/list", Meta: meta},
		{Name: "windows/get", Meta: meta},
	}}, nil
}

func (p *internalWindowTestProvider) CallTool(_ context.Context, request *schema.CallToolRequestParams, options ...mcpclient.RequestOption) (*schema.CallToolResult, error) {
	if !mcpclient.NewRequestOptions(options).NoRetry {
		return nil, errors.New("expected no-retry request")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.denied {
		return nil, identity.ErrResourceDenied
	}
	arguments := request.Arguments
	var result any
	switch request.Name {
	case "namespaces/list":
		result = primitive.NamespaceListResult{ProviderIdentity: p.owner, Namespaces: []primitive.Namespace{{Name: "platform", Kinds: []string{"window"}}}, Complete: true}
	case "namespaces/get":
		result = primitive.NamespaceCapabilities{ProviderIdentity: p.owner, Namespace: "platform", Kinds: []primitive.KindSupport{{Kind: "window", FormatVersions: []int64{2}, Operations: []string{"list", "get"}, Methods: primitive.Methods("window", []string{"list", "get"})}}}
	case "windows/list":
		p.listCalls++
		rows := make([]*primitive.ResourceState, 0, len(p.definition))
		for uri := range p.definition {
			parsed, _ := identity.ParseResourceURI(uri)
			rows = append(rows, &primitive.ResourceState{URI: uri, Kind: "window", Namespace: parsed.Namespace, Name: parsed.Name, Title: parsed.Name})
		}
		result = primitive.ListResult{Resources: rows, Complete: true}
	case "windows/get":
		p.getCalls++
		uri, _ := arguments["uri"].(string)
		raw := p.definition[uri]
		if len(raw) == 0 {
			return nil, identity.ErrResourceDenied
		}
		candidate := identity.ResourceCandidate{Kind: identity.WorkingCandidate, ContentFingerprint: identity.ContentFingerprint(raw)}
		result = primitive.GetResult{
			Resource:         &primitive.ResourceState{URI: uri, Kind: "window", Definition: raw, DefinitionBytes: append([]byte(nil), raw...), ContentFingerprint: candidate.ContentFingerprint},
			ResolvedResource: &identity.ResolvedResource{ProviderIdentity: p.owner, URI: uri, ResourceCandidate: candidate, AuthorityBinding: "current-authority", ValidUntil: p.lease},
		}
	default:
		return nil, errors.New("unexpected fixture tool")
	}
	return &schema.CallToolResult{StructuredContent: result}, nil
}

func newInternalWindowTestCatalog(t *testing.T, definitions map[string]json.RawMessage) (*internalWindowCatalog, *internalWindowTestProvider) {
	t.Helper()
	provider := &internalWindowTestProvider{owner: "studio-provider", definition: definitions, lease: time.Now().Add(time.Minute)}
	options := internalWindowTestOptions{"studio": &corecfg.MCPClient{}}
	mgr, err := manager.New(options, manager.WithClientFactory(func(context.Context, string, string) (mcpclient.Interface, error) { return provider, nil }))
	require.NoError(t, err)
	t.Cleanup(func() { mgr.CloseConversation("") })
	actor := identity.VerifiedActor{Subject: "alice", Issuer: "https://issuer.example", TenantID: "platform", AccountID: "opaque-account", IdentityRevision: "rev-1", ValidUntil: time.Now().Add(time.Minute)}
	gateway := resources.NewGateway(mgr, func(context.Context) (identity.VerifiedActor, error) { return actor, nil }, func(_ context.Context, held identity.VerifiedActor) error {
		if !held.Valid(time.Now()) || held.Subject != actor.Subject || held.AccountID != actor.AccountID || held.IdentityRevision != actor.IdentityRevision {
			return identity.ErrResourceDenied
		}
		return nil
	}, "internal-host")
	proof, err := types.NewWindowTargetHMAC(make([]byte, 32))
	require.NoError(t, err)
	remote := &resources.WindowCatalog{Gateway: gateway, TargetProof: proof, Admission: func(_ context.Context, pin identity.ResolvedResource, _ *types.Window) error {
		if pin.ProviderIdentity != provider.owner || !pin.ValidUntil.After(time.Now()) {
			return identity.ErrResourceDenied
		}
		return nil
	}}
	catalog, err := newInternalWindowCatalog(remote, []windowloader.ResourceBinding{{CatalogID: "orders", WindowKey: "deliver/orders", URI: "window://platform/deliver/orders"}})
	require.NoError(t, err)
	return catalog, provider
}

func TestInternalWindowCatalogMapsAliasesToExactGatewayResources(t *testing.T) {
	definitions := map[string]json.RawMessage{
		"window://platform/deliver/orders": json.RawMessage(`{"schemaVersion":2,"view":{"title":"Orders","content":{"id":"orders"}}}`),
		"window://platform/hidden/other":   json.RawMessage(`{"schemaVersion":2,"view":{"title":"Other","content":{"id":"other"}}}`),
	}
	catalog, provider := newInternalWindowTestCatalog(t, definitions)
	if _, e := catalog.remote.List(context.Background(), &primitiveprovider.WindowDefinitionListInput{Limit: 100}); e != nil {
		t.Fatalf("remote list fixture: %v", e)
	}
	listed, err := catalog.List(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, listed.Windows, 1)
	require.Equal(t, "orders", listed.Windows[0].WindowID)
	require.Equal(t, "window://platform/deliver/orders", listed.Windows[0].ResourceURI)
	require.Zero(t, provider.getCalls, "catalog listing must stay metadata-only")

	byAlias, err := catalog.Get(context.Background(), &primitiveprovider.WindowDefinitionGetInput{WindowID: "orders"})
	require.NoError(t, err)
	require.Equal(t, "orders", byAlias.WindowID)
	require.Equal(t, "window://platform/deliver/orders", byAlias.Definition.Resource.URI)
	require.Equal(t, "studio-provider", byAlias.Definition.Resource.ProviderIdentity)
	require.NotEmpty(t, byAlias.Definition.ResourceTarget.SelectionToken)

	canonical, err := catalog.Get(context.Background(), &primitiveprovider.WindowDefinitionGetInput{
		WindowID:         "window://platform/deliver/orders",
		Resource:         &identity.ResourceRef{URI: byAlias.Definition.Resource.URI, Revision: byAlias.Definition.Resource.Selector()},
		ResolvedResource: byAlias.Definition.Resource,
		Target:           byAlias.Definition.ResourceTarget,
	})
	require.NoError(t, err)
	require.Equal(t, byAlias.Definition.Resource.ContentFingerprint, canonical.Definition.Resource.ContentFingerprint)
	require.Equal(t, byAlias.Definition.Resource.ProviderIdentity, canonical.Definition.Resource.ProviderIdentity)
	require.Equal(t, byAlias.Definition.ResourceTarget.SelectionToken, canonical.Definition.ResourceTarget.SelectionToken)
	require.Equal(t, 4, provider.getCalls, "each alias and canonical read revalidates through the delegated Gateway")

	_, err = catalog.Get(context.Background(), &primitiveprovider.WindowDefinitionGetInput{WindowID: "window://platform/not-registered/orders"})
	require.ErrorIs(t, err, identity.ErrResourceDenied)
	require.Equal(t, 4, provider.getCalls, "unregistered URI must be denied before provider dispatch")
	_, err = catalog.Get(context.Background(), &primitiveprovider.WindowDefinitionGetInput{WindowID: "deliver/orders/../../hidden/other"})
	require.ErrorIs(t, err, identity.ErrResourceDenied)
}

func TestInternalWindowCatalogRejectsPinAndTargetDrift(t *testing.T) {
	catalog, provider := newInternalWindowTestCatalog(t, map[string]json.RawMessage{
		"window://platform/deliver/orders": json.RawMessage(`{"schemaVersion":2,"view":{"title":"Orders","content":{"id":"orders"}}}`),
	})
	opened, err := catalog.Get(context.Background(), &primitiveprovider.WindowDefinitionGetInput{WindowID: "deliver/orders"})
	require.NoError(t, err)
	pin := *opened.Definition.Resource
	wrongOwner := pin
	wrongOwner.ProviderIdentity = "another-provider"
	_, err = catalog.Get(context.Background(), &primitiveprovider.WindowDefinitionGetInput{WindowID: "orders", ResolvedResource: &wrongOwner, Target: opened.Definition.ResourceTarget})
	require.ErrorIs(t, err, identity.ErrResourceDenied)
	changedContent := pin
	changedContent.ContentFingerprint = strings.Repeat("0", 64)
	_, err = catalog.Get(context.Background(), &primitiveprovider.WindowDefinitionGetInput{WindowID: "orders", ResolvedResource: &changedContent, Target: opened.Definition.ResourceTarget})
	require.Error(t, err)

	badTarget := *opened.Definition.ResourceTarget
	badTarget.Capabilities = append(badTarget.Capabilities, "injected")
	require.Error(t, catalog.VerifyWindowTarget(context.Background(), pin, badTarget, "variant"))
	variantFingerprint := identity.ContentFingerprint(provider.definition[pin.URI])
	require.NoError(t, catalog.VerifyWindowTarget(context.Background(), pin, *opened.Definition.ResourceTarget, variantFingerprint))
	_, err = catalog.RevalidateResource(context.Background(), "orders", pin)
	require.NoError(t, err)

	provider.definition[pin.URI] = json.RawMessage(`{"schemaVersion":2,"view":{"title":"Changed","content":{"id":"orders"}}}`)
	_, err = catalog.RevalidateResource(context.Background(), "orders", pin)
	require.ErrorIs(t, err, identity.ErrResourceDenied)
}

func TestInternalWindowCatalogBindingAmbiguityFailsClosed(t *testing.T) {
	remote, _ := newInternalWindowTestCatalog(t, map[string]json.RawMessage{"window://platform/deliver/orders": json.RawMessage(`{"schemaVersion":2,"view":{"content":{"id":"orders"}}}`)})
	_, err := newInternalWindowCatalog(remote.remote, []windowloader.ResourceBinding{
		{CatalogID: "orders", WindowKey: "deliver/orders", URI: "window://platform/deliver/orders"},
		{CatalogID: "orders", WindowKey: "deliver/other", URI: "window://platform/deliver/other"},
	})
	require.Error(t, err)
}
