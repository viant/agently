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
// ToolCapabilities classifies exact methods for selection fidelity only.
// Managing MCP owns data authorization; no UI business grant is consulted.
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
// Selected-entity mutations require their declared body relation. This
// establishes fidelity to the signed selection, never a business ACL grant.
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
	config     NativeWindowHostConfig
	source     identity.ResourceSource
	authority  *nativeWindowAuthority
	catalog    *forge.MetadataWindowCatalog
	delivery   *nativeWindowDelivery
	datasource WindowBootstrapDatasource
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
		b.WithUIBridge(forge.NewService(&forge.Config{WindowDefinitions: catalog, WindowSelectionEmitter: delivery.emitSelection, ResolvedWindowAuthorizer: func(ctx context.Context, pin identity.ResolvedResource, method string, _ map[string]any) error {
			if method != "ui.window.open" {
				return identity.ErrResourceDenied
			}
			return authority.admit(ctx, pin)
		}}))
		b.WithPinnedDatasourceDefinitions(revalidate, definition, host.authorize)
		b.WithNewDatasourceDeliveryPreparer(delivery.prepare)
		b.WithDatasourceDenialRootRead(host.authorizeRootRead)
		return nil
	}
	options.nativeWindowRuntimeBinder = func(rt *executor.Runtime) error {
		if rt == nil || rt.Native == nil {
			return identity.ErrResourceDenied
		}
		return delivery.bindUsers(auth.NewDatlyUserService(rt.Native))
	}
	factory := ConfiguredWindowBootstrap(config.Bootstrap)
	options.WindowOpenBootstrapFactory = func(ctx context.Context, source WindowBootstrapDatasource) (permittedview.OpenBootstrap, permittedview.OpenSelectionCheck, error) {
		host.datasource = source
		load, match, err := factory(ctx, &nativeBootstrapDatasource{host: host, source: source})
		if err != nil {
			return nil, nil, err
		}
		return func(ctx context.Context, pin identity.ResolvedResource, window *types.Window, parameters map[string]any) (map[string]any, error) {
			return load(requestctx.WithWindowParameters(ctx, parameters), pin, window, parameters)
		}, match, nil
	}
	return options, nil
}

func (h *nativeWindowHost) authorize(ctx context.Context, descriptor *ds.DataSource, inputs map[string]interface{}) error {
	return h.authorizeMode(ctx, descriptor, inputs, false)
}
func (h *nativeWindowHost) authorizeRootRead(ctx context.Context, descriptor *ds.DataSource, inputs map[string]interface{}) error {
	return h.authorizeMode(ctx, descriptor, inputs, true)
}
func (h *nativeWindowHost) authorizeMode(ctx context.Context, descriptor *ds.DataSource, inputs map[string]interface{}, _ bool) error {
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
	return h.authorizeWindow(ctx, pin.URI, definition.Definition, descriptor, inputs)
}

// A datasource admission checks only original artifact and selection fidelity.
// Its managing MCP endpoint owns every business read/write authorization check.
func (h *nativeWindowHost) authorizeWindow(ctx context.Context, uri string, window *types.Window, descriptor *ds.DataSource, inputs map[string]interface{}) error {
	if h == nil || window == nil || descriptor == nil {
		return identity.ErrResourceDenied
	}
	classification := "read"
	if descriptor.Backend != nil && descriptor.Backend.Kind == ds.BackendMCPTool {
		var known bool
		classification, known = h.config.ToolCapabilities[descriptor.Backend.Service+":"+descriptor.Backend.Method]
		if !known {
			return identity.ErrResourceDenied
		}
	} else if descriptor.Backend != nil && descriptor.Backend.Kind != ds.BackendInline {
		return identity.ErrResourceDenied
	}
	if marker, _ := ctx.Value(nativeBootstrapKey{}).(*nativeBootstrapRead); marker != nil && marker.owner == h && classification != "read" {
		return identity.ErrResourceDenied
	}
	parameters := requestctx.WindowParametersFromContext(ctx)
	for _, parameter := range descriptor.Parameters {
		from := parameter.From
		if from == "" {
			from = parameter.In
		}
		if from != "windowForm" {
			continue
		}
		actual, supplied := inputs[parameter.Name]
		if !supplied {
			continue
		}
		expected, present := bootstrapParameterValue(parameters, parameter.Location)
		if !present || !nativeJSONEqual(actual, expected) {
			return identity.ErrResourceDenied
		}
	}
	if classification == "read" {
		return nil
	}
	var selection *WindowBootstrapBinding
	for i := range h.config.Bootstrap {
		if h.config.Bootstrap[i].URI == uri {
			selection = &h.config.Bootstrap[i]
			break
		}
	}
	var binding *NativeWindowExecutionBinding
	for i := range h.config.ExecutionBindings {
		candidate := &h.config.ExecutionBindings[i]
		if candidate.URI == uri && candidate.DataSource == descriptor.ID {
			if binding != nil {
				return identity.ErrResourceDenied
			}
			binding = candidate
		}
	}
	if binding == nil {
		if selection != nil {
			return identity.ErrResourceDenied
		}
		return nil // No selected-entity relation was declared for this principal action.
	}
	if selection == nil || len(binding.IdentitySelectors) == 0 {
		return identity.ErrResourceDenied
	}
	selected, present := parameters[selection.Parameter]
	if !present {
		return identity.ErrResourceDenied
	}
	for _, selector := range binding.IdentitySelectors {
		values, err := nativeSelectionValues(inputs, strings.Split(selector, "."))
		if err != nil || len(values) == 0 {
			return identity.ErrResourceDenied
		}
		for _, value := range values {
			if !nativeSelectionContains(selected, value) {
				return identity.ErrResourceDenied
			}
		}
	}
	if len(binding.ParentSelectors) != 0 {
		return h.verifyParentSelection(ctx, *selection, *binding, inputs)
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
