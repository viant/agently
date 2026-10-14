package agently

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/viant/afs"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/forge/backend/service/meta"
	"log"
	"os"
	"strings"

	"github.com/viant/agently-core/app/executor"
	ds "github.com/viant/agently-core/protocol/datasource"
	identity "github.com/viant/agently-core/protocol/resource"
	requestctx "github.com/viant/agently-core/runtime/requestctx"
	auth "github.com/viant/agently-core/service/auth"
	"github.com/viant/agently-core/service/policy"
	forge "github.com/viant/agently-core/service/primitiveprovider"
	resources "github.com/viant/agently-core/service/resource"
	"github.com/viant/agently-core/service/ui/permittedview"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/authz/oauth"
	"github.com/viant/forge/backend/types"
)

// NativeWindowHostConfig binds operator-installed templates without replacing
// the workspace's explicitly configured MCP permission provider.
// ToolCapabilities and ExecutionBindings are retained for config compatibility only.
// Managing MCP owns data authorization; no UI business grant is consulted.
type NativeWindowHostConfig struct {
	SchemaVersion     int                            `json:"schemaVersion"`
	ProviderIdentity  string                         `json:"providerIdentity"`
	Identity          oauth.HTTPIdentityConfig       `json:"identity"`
	Windows           []windowloader.ResourceBinding `json:"windows"`
	FrameworkWindows  []windowloader.ResourceBinding `json:"frameworkWindows,omitempty"`
	ToolCapabilities  map[string]string              `json:"toolCapabilities"`
	ExecutionBindings []NativeWindowExecutionBinding `json:"executionBindings"`
}

// NativeWindowExecutionBinding is a legacy declaration retained for config decoding.
// The managing MCP endpoint owns argument and entity authorization.
type NativeWindowExecutionBinding struct {
	URI               string            `json:"uri"`
	DataSource        string            `json:"dataSource"`
	IdentitySelectors []string          `json:"identitySelectors"`
	ParentSelectors   map[string]string `json:"parentSelectors,omitempty"`
}

// Both operator-installed windows and linked definitions share the explicit
// workspace source anchor and the same immutable dependency snapshot.
func newNativeWindowHostSnapshot(ctx context.Context, root string, config NativeWindowHostConfig) (*resources.NativeAssetSnapshot, error) {
	definitions, err := resources.SnapshotWindowDefinitions(ctx, root, config.Windows, nil)
	if err != nil {
		return nil, err
	}
	linked, err := newEmbeddedWindowSource(config.FrameworkWindows)
	if err != nil {
		return nil, err
	}
	for _, binding := range config.FrameworkWindows {
		binding := binding
		uri, err := identity.ParseResourceURI(binding.URI)
		if err != nil {
			return nil, err
		}
		base := serializedWindowBase(func(ctx context.Context, target *types.WindowTarget) (json.RawMessage, error) {
			return linked.loadTarget(ctx, uri, target)
		})
		definitions = append(definitions, resources.NativeSnapshotDefinition{URI: binding.URI, Title: binding.WindowKey, FormatVersion: 2, File: "extension/forge", Load: resources.NativeComposedWindowLoader(binding, base, nil)})
	}
	snapshot, err := resources.NewNativeAssetSnapshot(ctx, root, definitions, resources.NativeSnapshotOptions{})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

type nativeWindowHost struct {
	config    NativeWindowHostConfig
	source    identity.ResourceSource
	authority *nativeWindowAuthority
	catalog   *forge.MetadataWindowCatalog
	delivery  *nativeWindowDelivery
}

func configureNativeWindowHost(options ServeOptions) (ServeOptions, error) {
	if options.NativeWindowConfigPath == "" {
		return options, nil
	}
	if options.AuthzConfigPath != "" || os.Getenv("AGENTLY_AUTHZ_CONFIG") != "" || options.InternalResourceProviderIdentity != "" {
		return options, fmt.Errorf("native window configuration cannot mix authorization providers")
	}
	raw, e := os.ReadFile(options.NativeWindowConfigPath)
	if e != nil {
		return options, e
	}
	var config NativeWindowHostConfig
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(&config); e != nil {
		return options, e
	}
	if config.SchemaVersion != 1 || config.ProviderIdentity == "" || len(config.Windows) == 0 {
		return options, fmt.Errorf("native windows require exact operator bindings")
	}
	if e = validateNativeWindowConfig(config); e != nil {
		return options, e
	}
	providers, e := oauth.NewHTTPProviders(context.Background(), config.Identity, nil)
	if e != nil {
		return options, e
	}
	snapshot, e := newNativeWindowHostSnapshot(context.Background(), options.WorkspacePath, config)
	if e != nil {
		return options, e
	}
	var source identity.ResourceSource = snapshot
	bindings := append(append([]windowloader.ResourceBinding(nil), config.Windows...), config.FrameworkWindows...)
	authority := &nativeWindowAuthority{principal: func(ctx context.Context) (p gating.Principal, err error) {
		p, err = providers.NativeWindowIdentity(auth.AuthzIDTokenContext(ctx))
		if err != nil {
			log.Printf("native window verified identity unavailable (request ID token present=%t)", auth.IDToken(ctx) != "")
		}
		return p, err
	}, allowed: map[string]resources.LocalResourceBinding{}}
	aliases := map[string]string{}
	for _, w := range bindings {
		for _, key := range []string{w.URI, w.WindowKey, w.CatalogID} {
			if key == "" {
				continue
			}
			if previous, ok := aliases[key]; ok && previous != w.URI {
				return options, fmt.Errorf("ambiguous native window alias")
			}
			aliases[key] = w.URI
		}
		authority.allowed[w.URI] = resources.LocalResourceBinding{URI: w.URI}
	}
	var delivery *nativeWindowDelivery
	resolve := func(ctx context.Context, key string) (*identity.ResourceResolver, identity.ResourceRef, error) {
		uri, ok := aliases[key]
		if !ok {
			return nil, identity.ResourceRef{}, identity.ErrResourceDenied
		}
		return &identity.ResourceResolver{Source: source, Policy: &nativeDeliveryPolicy{base: &nativeWindowPolicy{authority: authority}, delivery: delivery}, ProviderIdentity: config.ProviderIdentity}, identity.ResourceRef{URI: uri}, nil
	}
	key := make([]byte, 32)
	if _, e = rand.Read(key); e != nil {
		return options, e
	}
	proof, e := types.NewWindowTargetHMAC(key)
	if e != nil {
		return options, e
	}
	selectionKey := make([]byte, 32)
	if _, e = rand.Read(selectionKey); e != nil {
		return options, e
	}
	selection, e := identity.NewSelectionHMAC(selectionKey)
	if e != nil {
		return options, e
	}
	delivery = &nativeWindowDelivery{authority: authority, signer: selection, target: proof, resolve: resolve}
	// A metadata loader is required by the catalog API; canonical entries always
	// resolve the immutable source above, never this legacy fallback.
	entries := make([]forge.SavedWindow, 0, len(bindings))
	for _, w := range bindings {
		entries = append(entries, forge.SavedWindow{WindowDefinitionSummary: forge.WindowDefinitionSummary{WindowID: w.WindowKey, ResourceURI: w.URI, Title: w.WindowKey}, Key: w.WindowKey})
	}
	catalog, e := forge.NewMetadataWindowCatalog(meta.New(afs.New(), options.WorkspacePath), options.WorkspacePath, entries, forge.WithWindowResourceResolver(resolve), forge.WithWindowTargetProof(proof))
	if e != nil {
		return options, e
	}
	host := &nativeWindowHost{config: config, source: source, authority: authority, catalog: catalog, delivery: delivery}
	revalidate := func(ctx context.Context, pin identity.ResolvedResource) (*identity.ResolvedResource, error) {
		resolver, ref, e := resolve(ctx, pin.URI)
		if e != nil || ref.URI != pin.URI {
			return nil, identity.ErrResourceDenied
		}
		_, fresh, e := resolver.ReadResolved(ctx, pin)
		return fresh, e
	}
	definition := func(ctx context.Context, pin identity.ResolvedResource, target *types.WindowTarget, id string) (*ds.DataSource, error) {
		if target == nil {
			return nil, identity.ErrResourceDenied
		}
		resolver, _, e := resolve(ctx, pin.URI)
		if e != nil {
			return nil, e
		}
		raw, _, e := resolver.ReadResolved(ctx, pin)
		if e != nil {
			return nil, e
		}
		variant, e := types.SelectWindowResource(raw, target)
		if e != nil {
			return nil, e
		}
		if e = catalog.VerifyWindowTarget(ctx, pin, *target, variant.Fingerprint); e != nil {
			return nil, e
		}
		var value ds.DataSource
		if json.Unmarshal(variant.DataSources[id], &value) != nil || value.ID != id {
			return nil, identity.ErrResourceDenied
		}
		return &value, nil
	}
	previous := options.ConfigureBuilder
	options.ConfigureBuilder = func(ctx context.Context, b *executor.Builder) error {
		if previous != nil {
			if e := previous(ctx, b); e != nil {
				return e
			}
		}
		b.WithUIBridge(forge.NewService(&forge.Config{WindowDefinitions: catalog, WindowSelectionEmitter: delivery.emitSelection}))

		b.WithPinnedDatasourceDefinitions(revalidate, definition, host.authorize)
		b.WithNewDatasourceDeliveryPreparer(delivery.prepare)
		return nil
	}
	options.nativeWindowRuntimeBinder = func(rt *executor.Runtime) error {
		if rt == nil || rt.Native == nil {
			return identity.ErrResourceDenied
		}
		return delivery.bindUsers(auth.NewDatlyUserService(rt.Native))
	}
	return options, nil
}

func (h *nativeWindowHost) authorize(ctx context.Context, descriptor *ds.DataSource, inputs map[string]interface{}) error {
	if h == nil || h.delivery == nil || descriptor == nil {
		return identity.ErrResourceDenied
	}
	pin, ok := requestctx.ResolvedResourceFromContext(ctx)
	target, present := requestctx.WindowTargetFromContext(ctx)
	if !ok || !present || target == nil {
		return identity.ErrResourceDenied
	}
	if err := h.verifyDeliveryContext(ctx, *pin, target, descriptor.ID, inputs); err != nil {
		return err
	}
	definition, err := h.catalog.Get(ctx, &forge.WindowDefinitionGetInput{WindowID: pin.URI, ResolvedResource: pin, Target: target})
	if err != nil {
		return nativeWindowAuthorityError(err)
	}
	if definition == nil || definition.Definition == nil {
		return identity.ErrResourceDenied
	}
	return nil
}

// Preserve only trusted error classifications. Unknown verification failures
// deny execution; upstream details never become browser-visible error text.
func nativeWindowAuthorityError(err error) error {
	if errors.Is(err, ds.ErrDatasourceDenied) {
		return ds.ErrDatasourceDenied
	}
	var upstream *ds.UpstreamAuthorizationError
	if errors.As(err, &upstream) && (upstream.Status == 401 || upstream.Status == 403) {
		return &ds.UpstreamAuthorizationError{Status: upstream.Status}
	}
	if errors.Is(err, identity.ErrResourceDenied) || errors.Is(err, identity.ErrResourceStale) || errors.Is(err, policy.ErrDenied) || errors.Is(err, authz.ErrDenied) {
		return identity.ErrResourceDenied
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(err, permittedview.ErrUnavailable) || errors.Is(err, authz.ErrUnavailable) || errors.Is(err, gating.ErrUnavailable):
		return permittedview.ErrUnavailable
	case errors.Is(err, context.Canceled):
		return context.Canceled
	default:
		return identity.ErrResourceDenied
	}
}

func validateNativeWindowConfig(config NativeWindowHostConfig) error {
	windows := map[string]string{}
	for _, w := range append(append([]windowloader.ResourceBinding(nil), config.Windows...), config.FrameworkWindows...) {
		if previous, ok := windows[w.URI]; ok && previous != w.WindowKey {
			return fmt.Errorf("ambiguous native window")
		}
		windows[w.URI] = w.WindowKey
	}
	return nil
}
