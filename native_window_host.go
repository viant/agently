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
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/viant/agently-core/app/executor"
	ds "github.com/viant/agently-core/protocol/datasource"
	identity "github.com/viant/agently-core/protocol/resource"
	requestctx "github.com/viant/agently-core/runtime/requestctx"
	"github.com/viant/agently-core/sdk/api"
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
// ToolCapabilities is a reviewed exact service/method map, not a name heuristic.
type NativeWindowHostConfig struct {
	SchemaVersion     int                            `json:"schemaVersion"`
	ProviderIdentity  string                         `json:"providerIdentity"`
	Identity          oauth.HTTPIdentityConfig       `json:"identity"`
	Windows           []windowloader.ResourceBinding `json:"windows"`
	FrameworkWindows  []windowloader.ResourceBinding `json:"frameworkWindows,omitempty"`
	Bootstrap         []WindowBootstrapBinding       `json:"bootstrap"`
	ToolCapabilities  map[string]string              `json:"toolCapabilities"`
	ExecutionBindings []NativeWindowExecutionBinding `json:"executionBindings"`
}

// NativeWindowExecutionBinding correlates an exact authored mutation to the
// selected entity. Selectors traverse fixed object keys and array wildcards.
// Unmapped mutations are denied, including cross-entity actions.
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
	snapshot, err := resources.NewNativeAssetSnapshot(ctx, root, definitions, resources.NativeSnapshotOptions{ImmutableUntilRestart: true})
	if err != nil {
		return nil, err
	}
	if err := validateNativeExecutionDefinitions(ctx, snapshot, config.ExecutionBindings); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// Declarations are checked against the compiled closure of their exact URI.
// A declaration may be specific to an authored target variant.
func validateNativeExecutionDefinitions(ctx context.Context, source identity.ResourceSource, bindings []NativeWindowExecutionBinding) error {
	for _, binding := range bindings {
		uri, err := identity.ParseResourceURI(binding.URI)
		if err != nil {
			return err
		}
		candidates, err := source.Candidates(ctx, uri)
		if err != nil || len(candidates) != 1 {
			return fmt.Errorf("native execution window unavailable: %s", binding.URI)
		}
		raw, err := source.ReadCandidate(ctx, uri, candidates[0])
		if err != nil {
			return err
		}
		var envelope types.WindowResourceEnvelope
		if err = json.Unmarshal(raw, &envelope); err != nil {
			return err
		}
		if err = envelope.Validate(); err != nil {
			return err
		}
		found := false
		for _, variant := range envelope.Variants {
			if descriptor, ok := variant.DataSources[binding.DataSource]; ok {
				var parsed ds.DataSource
				if json.Unmarshal(descriptor, &parsed) == nil && parsed.ID == binding.DataSource {
					found = true
					break
				}
			}
		}
		if !found {
			return fmt.Errorf("native execution datasource %s absent from %s", binding.DataSource, binding.URI)
		}
	}
	return nil
}

type nativeWindowHost struct {
	config      NativeWindowHostConfig
	source      identity.ResourceSource
	authority   *nativeWindowAuthority
	catalog     *forge.MetadataWindowCatalog
	permissions permittedview.Resolver
	datasource  WindowBootstrapDatasource
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
	for _, cap := range config.ToolCapabilities {
		if cap != "read" && cap != "write" && cap != "archive" {
			return options, fmt.Errorf("invalid native tool capability")
		}
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
	// Validate declared bootstrap against the immutable original web variant
	// before serving. This reads trusted static bytes, never entity data.
	for _, binding := range config.Bootstrap {
		uri, err := identity.ParseResourceURI(binding.URI)
		if err != nil {
			return options, err
		}
		candidates, err := snapshot.Candidates(context.Background(), uri)
		if err != nil || len(candidates) != 1 {
			return options, fmt.Errorf("native bootstrap candidate unavailable")
		}
		raw, err := snapshot.ReadCandidate(context.Background(), uri, candidates[0])
		if err != nil {
			return options, err
		}
		variant, err := types.SelectWindowResource(raw, &types.WindowTarget{Platform: "web"})
		if err != nil || !matchesDeclaredBootstrap(variant.Window, binding) {
			return options, fmt.Errorf("native bootstrap does not match authored window %s", binding.WindowKey)
		}
	}
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
	resolve := func(ctx context.Context, key string) (*identity.ResourceResolver, identity.ResourceRef, error) {
		uri, ok := aliases[key]
		if !ok {
			return nil, identity.ResourceRef{}, identity.ErrResourceDenied
		}
		return &identity.ResourceResolver{Source: source, Policy: &nativeWindowPolicy{authority: authority}, ProviderIdentity: config.ProviderIdentity}, identity.ResourceRef{URI: uri}, nil
	}
	key := make([]byte, 32)
	if _, e = rand.Read(key); e != nil {
		return options, e
	}
	proof, e := types.NewWindowTargetHMAC(key)
	if e != nil {
		return options, e
	}
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
	host := &nativeWindowHost{config: config, source: source, authority: authority, catalog: catalog}
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
		b.WithUIBridge(forge.NewService(&forge.Config{WindowDefinitions: catalog, ResolvedWindowAuthorizer: func(ctx context.Context, pin identity.ResolvedResource, method string, _ map[string]any) error {
			if method != "ui.window.open" {
				return identity.ErrResourceDenied
			}
			return authority.admit(ctx, pin)
		}}))
		b.WithPinnedDatasourceDefinitions(revalidate, definition, host.authorize)
		b.WithDatasourceDenialRootRead(host.authorizeRootRead)
		return nil
	}
	factory := ConfiguredWindowBootstrap(config.Bootstrap)
	options.WindowOpenBootstrapFactory = func(ctx context.Context, source WindowBootstrapDatasource) (permittedview.OpenBootstrap, permittedview.OpenSelectionCheck, error) {
		// Runtime construction has retained the configured real MCP resolver.
		runtime := permittedview.DefaultRuntime()
		if runtime == nil || runtime.Resolver == nil {
			return nil, nil, fmt.Errorf("configured native MCP permission resolver unavailable")
		}
		host.permissions = runtime.Resolver
		host.datasource = source
		return factory(ctx, source)
	}
	return options, nil
}

func (h *nativeWindowHost) authorize(ctx context.Context, descriptor *ds.DataSource, inputs map[string]interface{}) error {
	return h.authorizeMode(ctx, descriptor, inputs, false)
}
func (h *nativeWindowHost) authorizeRootRead(ctx context.Context, descriptor *ds.DataSource, inputs map[string]interface{}) error {
	return h.authorizeMode(ctx, descriptor, inputs, true)
}
func (h *nativeWindowHost) authorizeMode(ctx context.Context, descriptor *ds.DataSource, inputs map[string]interface{}, rootRead bool) error {
	pin, ok := requestctx.ResolvedResourceFromContext(ctx)
	target, present := requestctx.WindowTargetFromContext(ctx)
	if !ok || !present || target == nil || h.permissions == nil || descriptor == nil {
		return identity.ErrResourceDenied
	}
	definition, e := h.catalog.Get(ctx, &forge.WindowDefinitionGetInput{WindowID: pin.URI, ResolvedResource: pin, Target: target})
	if e != nil {
		return nativeWindowAuthorityError(e)
	}
	if definition == nil || definition.Definition == nil {
		return identity.ErrResourceDenied
	}
	return h.authorizeWindowMode(ctx, pin.URI, definition.Definition, descriptor, inputs, rootRead)
}

func (h *nativeWindowHost) authorizeWindow(ctx context.Context, uri string, window *types.Window, descriptor *ds.DataSource, inputs map[string]interface{}) error {
	return h.authorizeWindowMode(ctx, uri, window, descriptor, inputs, false)
}
func (h *nativeWindowHost) authorizeWindowMode(ctx context.Context, uri string, window *types.Window, descriptor *ds.DataSource, inputs map[string]interface{}, rootRead bool) error {
	if window == nil || descriptor == nil || h.permissions == nil {
		return identity.ErrResourceDenied
	}
	capability := "read"
	if descriptor.Backend != nil && descriptor.Backend.Kind == ds.BackendMCPTool {
		var found bool
		capability, found = h.config.ToolCapabilities[descriptor.Backend.Service+":"+descriptor.Backend.Method]
		if !found {
			return identity.ErrResourceDenied
		}
	} else if descriptor.Backend != nil && descriptor.Backend.Kind != ds.BackendInline {
		return identity.ErrResourceDenied
	}
	if rootRead {
		capability = "read"
	}
	spec := window.Authorization
	if spec == nil {
		return identity.ErrResourceDenied
	}
	resourceType := spec.ResourceType
	if spec.Resource != nil {
		resourceType = spec.Resource.Type
	}
	if resourceType == "" {
		return identity.ErrResourceDenied
	}
	params := requestctx.WindowParametersFromContext(ctx)
	var ids []int
	if strings.EqualFold(spec.Scope, "resource") {
		var binding *WindowBootstrapBinding
		for i := range h.config.Bootstrap {
			if h.config.Bootstrap[i].URI == uri {
				binding = &h.config.Bootstrap[i]
				break
			}
		}
		if binding == nil || !matchesDeclaredBootstrap(window, *binding) {
			return identity.ErrResourceDenied
		}
		value := params[binding.Parameter]
		// Only the operator-declared ById bootstrap can select from its fixed input.
		if value == nil && descriptor.ID == binding.DataSource {
			value = inputs[binding.Input]
		}
		requested, e := authoredBootstrapIDs(value)
		if e != nil || len(requested) != 1 {
			return identity.ErrResourceDenied
		}
		selected := requested[0]
		ids = []int{int(selected)}
		// Authored windowForm inputs cannot be changed independently of the
		// entity selection. Compare only supplied inputs; the binder fills defaults.
		for _, p := range descriptor.Parameters {
			from := p.From
			if from == "" {
				from = p.In
			}
			if from != "windowForm" || (p.Location != binding.Parameter && !strings.HasPrefix(p.Location, binding.Parameter+".")) {
				continue
			}
			actual, exists := inputs[p.Name]
			if !exists {
				continue
			}
			expected, exists := bootstrapParameterValue(map[string]any{binding.Parameter: []int64{selected}}, p.Location)
			if !exists || !sameNativeSelection(actual, expected) {
				return identity.ErrResourceDenied
			}
		}
		if capability != "read" {
			var declared *NativeWindowExecutionBinding
			for i := range h.config.ExecutionBindings {
				b := &h.config.ExecutionBindings[i]
				if b.URI == uri && b.DataSource == descriptor.ID {
					if declared != nil {
						return identity.ErrResourceDenied
					}
					declared = b
				}
			}
			if declared == nil || len(declared.IdentitySelectors) == 0 {
				return identity.ErrResourceDenied
			}
			for _, selector := range declared.IdentitySelectors {
				values, err := nativeSelectionValues(inputs, strings.Split(selector, "."))
				if err != nil || len(values) == 0 {
					return identity.ErrResourceDenied
				}
				for _, value := range values {
					id, err := authoredBootstrapID(value)
					if err != nil || id != selected {
						return identity.ErrResourceDenied
					}
				}
			}
		}
	}
	requested := []string{"read"}
	if capability != "read" {
		requested = append(requested, capability)
	}
	globals := []string{capability}
	if strings.EqualFold(spec.Scope, "principal") {
		requested = append([]string(nil), spec.RequestedCapabilities...)
		globals = append([]string(nil), spec.RequestedGlobalCapabilities...)
		if capability != "read" {
			globals = append(globals, capability)
		}
	}
	snapshot, e := h.permissions.Resolve(ctx, &permittedview.Request{ResourceType: resourceType, ResourceIDs: ids, RequestedCapabilities: requested, RequestedGlobalCapabilities: globals, IncludePrincipal: true})
	if e != nil {
		return nativeWindowAuthorityError(e)
	}
	if snapshot == nil || snapshot.AuthorizationVersion == "" || !snapshot.ExpiresAt.After(time.Now()) {
		return identity.ErrResourceDenied
	}
	if len(ids) == 0 {
		if (!strings.EqualFold(spec.Scope, "principal") || capability != "read") && !snapshot.GlobalCapabilities[capability] {
			return identity.ErrResourceDenied
		}
		return nil
	}
	row := snapshot.Resources[strconv.Itoa(ids[0])]
	if row == nil || row.ID != ids[0] || !strings.EqualFold(row.Type, resourceType) || !row.Capabilities["read"] || !row.Capabilities[capability] {
		return identity.ErrResourceDenied
	}
	if capability != "read" {
		for _, b := range h.config.ExecutionBindings {
			if b.URI != uri || b.DataSource != descriptor.ID || len(b.ParentSelectors) == 0 {
				continue
			}
			if h.datasource == nil {
				return identity.ErrResourceDenied
			}
			pin, ok := requestctx.ResolvedResourceFromContext(ctx)
			target, present := requestctx.WindowTargetFromContext(ctx)
			if !ok || !present || target == nil {
				return identity.ErrResourceDenied
			}
			var bootstrap *WindowBootstrapBinding
			for i := range h.config.Bootstrap {
				if h.config.Bootstrap[i].URI == uri {
					bootstrap = &h.config.Bootstrap[i]
					break
				}
			}
			if bootstrap == nil {
				return identity.ErrResourceDenied
			}
			fetched, e := h.datasource.FetchDatasource(ctx, &api.FetchDatasourceInput{ID: bootstrap.DataSource, Resource: pin, Target: target, WindowParameters: map[string]any{bootstrap.Parameter: ids}, Inputs: map[string]interface{}{bootstrap.Input: ids}, Cache: &api.DatasourceCacheHints{BypassCache: true}})
			if e != nil {
				return nativeWindowAuthorityError(e)
			}
			if fetched == nil || len(fetched.Rows) != 1 {
				return identity.ErrResourceDenied
			}
			returned, e := authoredBootstrapID(fetched.Rows[0][bootstrap.RowIdentity])
			if e != nil || returned != int64(ids[0]) {
				return identity.ErrResourceDenied
			}
			for selector, field := range b.ParentSelectors {
				values, e := nativeOptionalSelectionValues(inputs, strings.Split(selector, "."))
				if e != nil {
					return identity.ErrResourceDenied
				}
				for _, v := range values {
					if !sameNativeSelection(v, fetched.Rows[0][field]) {
						return identity.ErrResourceDenied
					}
				}
			}
		}
	}
	// A fresh parent bootstrap may outlive the permission lease. Recheck
	// immediately before handing control back to protected execution.
	if !snapshot.ExpiresAt.After(time.Now()) {
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

func nativeOptionalSelectionValues(value any, path []string) ([]any, error) {
	if len(path) == 0 {
		return []any{value}, nil
	}
	if path[0] == "*" {
		v := reflect.ValueOf(value)
		if !v.IsValid() || (v.Kind() != reflect.Slice && v.Kind() != reflect.Array) || v.Len() == 0 {
			return nil, identity.ErrResourceDenied
		}
		var values []any
		for i := 0; i < v.Len(); i++ {
			found, e := nativeOptionalSelectionValues(v.Index(i).Interface(), path[1:])
			if e != nil {
				return nil, e
			}
			values = append(values, found...)
		}
		return values, nil
	}
	m, ok := value.(map[string]interface{})
	if !ok {
		return nil, identity.ErrResourceDenied
	}
	v, exists := m[path[0]]
	if !exists {
		return nil, nil
	}
	return nativeOptionalSelectionValues(v, path[1:])
}

func sameNativeSelection(actual, expected any) bool {
	a, ea := authoredBootstrapIDs(actual)
	b, eb := authoredBootstrapIDs(expected)
	return ea == nil && eb == nil && reflect.DeepEqual(a, b)
}
func nativeSelectionValues(value any, path []string) ([]any, error) {
	if len(path) == 0 {
		return []any{value}, nil
	}
	if path[0] == "*" {
		v := reflect.ValueOf(value)
		if !v.IsValid() || (v.Kind() != reflect.Slice && v.Kind() != reflect.Array) || v.Len() == 0 {
			return nil, identity.ErrResourceDenied
		}
		var result []any
		for i := 0; i < v.Len(); i++ {
			found, e := nativeSelectionValues(v.Index(i).Interface(), path[1:])
			if e != nil {
				return nil, e
			}
			result = append(result, found...)
		}
		return result, nil
	}
	m, ok := value.(map[string]interface{})
	if !ok {
		return nil, identity.ErrResourceDenied
	}
	next, ok := m[path[0]]
	if !ok || path[0] == "" {
		return nil, identity.ErrResourceDenied
	}
	return nativeSelectionValues(next, path[1:])
}

func validateNativeWindowConfig(config NativeWindowHostConfig) error {
	if err := validateWindowBootstrapBindings(config.Bootstrap); err != nil {
		return err
	}
	windows := map[string]string{}
	for _, w := range append(append([]windowloader.ResourceBinding(nil), config.Windows...), config.FrameworkWindows...) {
		if previous, ok := windows[w.URI]; ok && previous != w.WindowKey {
			return fmt.Errorf("ambiguous native window")
		}
		windows[w.URI] = w.WindowKey
	}
	for _, b := range config.Bootstrap {
		if windows[b.URI] != b.WindowKey {
			return fmt.Errorf("bootstrap outside native catalog")
		}
	}
	seen := map[string]bool{}
	for _, b := range config.ExecutionBindings {
		key := b.URI + "\x00" + b.DataSource
		if windows[b.URI] == "" || b.DataSource == "" || seen[key] || len(b.IdentitySelectors) == 0 {
			return fmt.Errorf("invalid native execution binding")
		}
		seen[key] = true
		for _, selector := range b.IdentitySelectors {
			if !validNativeSelector(selector) {
				return fmt.Errorf("invalid native identity selector")
			}
		}
		for selector, field := range b.ParentSelectors {
			if !validNativeSelector(selector) || field == "" || strings.ContainsAny(field, ".*/[]") {
				return fmt.Errorf("invalid native parent selector")
			}
		}
	}
	return nil
}
func validNativeSelector(selector string) bool {
	if selector == "" {
		return false
	}
	parts := strings.Split(selector, ".")
	if parts[0] == "*" {
		return false
	}
	for _, part := range parts {
		if part == "*" {
			continue
		}
		if part == "" || strings.TrimSpace(part) != part || strings.ContainsAny(part, "/[]*") {
			return false
		}
	}
	return parts[len(parts)-1] != "*"
}
