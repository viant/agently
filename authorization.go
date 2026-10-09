package agently

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	identity "github.com/viant/agently-core/protocol/resource"
	resourcesvc "github.com/viant/agently-core/service/resource"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/viant/agently-core/app/executor"
	svcauth "github.com/viant/agently-core/service/auth"
	"github.com/viant/agently-core/service/policy"
	forgeservice "github.com/viant/agently-core/service/primitiveprovider"
	"github.com/viant/agently-core/service/ui/permittedview"
	windowloader "github.com/viant/agently-core/service/ui/window"
	wscfg "github.com/viant/agently-core/workspace/config"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/authz/oauth"
	forgetypes "github.com/viant/forge/backend/types"
)

// AuthorizationProviders is an explicit trusted binding, implemented by an
// embedding host. This public module interprets no provider-specific identity DTO.
type AuthorizationProviders struct {
	Entitlements        map[string]gating.EntitlementProvider
	AccountProjection   permittedview.AccountProjection
	DecisionScope       executor.AuthorizationDecisionScope
	MetadataScope       forgeservice.MetadataReadScope
	Identity            oauth.IdentityAuthority
	AllowsTenant        func(string) bool
	EntityPermissions   gating.EntityPermissionProvider
	PermissionWithLease func(context.Context, authz.Facts, authz.Entity, string) (bool, time.Time, error)
}

// AuthorizationProviderFactory consumes only operator-owned configuration.
type AuthorizationProviderFactory func(context.Context, json.RawMessage, json.RawMessage) (AuthorizationProviders, error)

type hostAuthorizationFile struct {
	FrameworkWindowResources  []windowloader.ResourceBinding   `json:"frameworkWindowResources,omitempty"`
	ResourceSelectionProvider json.RawMessage                  `json:"resourceSelectionProvider,omitempty"`
	ReportResources           json.RawMessage                  `json:"reportResources,omitempty"`
	WindowResourcesPath       string                           `json:"windowResourcesPath,omitempty"`
	ResourceRevisionBindings  []policy.ResourceRevisionBinding `json:"resourceRevisionBindings,omitempty"`
	ResourceSelections        []authz.SelectionDocument        `json:"resourceSelections,omitempty"`
	WindowResources           []windowloader.ResourceBinding   `json:"windowResources,omitempty"`
	SchemaVersion             int                              `json:"schemaVersion"`
	ProviderRef               string                           `json:"providerRef"`
	CapabilityMappingRef      string                           `json:"capabilityMappingRef"`
	PolicyVersion             string                           `json:"policyVersion"`
	CatalogPath               string                           `json:"catalogPath"`
	AccountProjection         string                           `json:"accountProjection,omitempty"`
	ProtectTools              bool                             `json:"protectTools,omitempty"`
	// Identity and EntityEvaluation are opaque operator-owned settings. Only an
	// explicitly injected host factory may interpret provider-specific payloads.
	Identity              json.RawMessage                     `json:"identity"`
	EntityEvaluation      json.RawMessage                     `json:"entityEvaluation,omitempty"`
	Policies              []authz.Document                    `json:"policies"`
	Requirements          []gating.Binding                    `json:"requirements"`
	CapabilityPermissions []oauth.CapabilityPermissionBinding `json:"capabilityPermissions"`
	EntityRoles           []oauth.EntityRoleBinding           `json:"entityRoles,omitempty"`
	WholeResources        []policy.ResourceBinding            `json:"wholeResources"`
	Capabilities          []permittedview.CapabilityBinding   `json:"capabilities"`
	BackendResources      []policy.BackendBinding             `json:"backendResources,omitempty"`
	SelectedScopeBindings []hostSelectedScopeBinding          `json:"selectedScopeBindings,omitempty"`
}

// ValidateHostAuthorizationConfiguration validates operator-owned registration
// without execution services, schema/data writes or starting listeners.
// JWKS discovery uses the configured identity endpoint; no user token is sent.
func ValidateHostAuthorizationConfiguration(options ServeOptions) error {
	root := strings.TrimSpace(options.WorkspacePath)
	if root == "" {
		return fmt.Errorf("authorization validation requires an explicit workspace path")
	}
	if strings.TrimSpace(options.AuthzConfigPath) == "" && strings.TrimSpace(os.Getenv("AGENTLY_AUTHZ_CONFIG")) == "" {
		return fmt.Errorf("authorization validation requires an explicit authz config")
	}
	settings, err := wscfg.Load(root)
	if err != nil {
		return err
	}
	configure, err := configureHostAuthorization(options, root, settings, nil, &internalHostRegistration{})
	if err != nil {
		return err
	}
	return configure(context.Background(), executor.NewBuilder())
}

type hostSelectedScopeBinding struct {
	Resource   authz.Resource `json:"resource"`
	Action     string         `json:"action"`
	Permission string         `json:"permission"`
}

type hostSelectedScopes struct{ provider authz.SelectedScopeProvider }

func (s hostSelectedScopes) ResolveSelectedScope(ctx context.Context, request authz.Request, document authz.Document, facts authz.Facts) (authz.SelectedScopeDecision, error) {
	return s.provider.ResolveSelectedScope(svcauth.AuthzIDTokenContext(ctx), request, document, facts)
}

func selectedScopeProvider(client gating.EntityPermissionProvider, principals gating.PrincipalResolver, bindings []hostSelectedScopeBinding) (authz.SelectedScopeProvider, error) {
	index := map[string]string{}
	key := func(resource authz.Resource, action string) string {
		data, _ := json.Marshal(struct {
			Resource authz.Resource
			Action   string
		}{resource, action})
		return string(data)
	}
	for _, binding := range bindings {
		if binding.Resource.Kind == "" || binding.Resource.ID == "" || binding.Resource.Version == "" || binding.Resource.Tenant == "" || binding.Action == "" || binding.Permission == "" {
			return nil, fmt.Errorf("invalid selected scope binding")
		}
		entry := key(binding.Resource, binding.Action)
		if _, exists := index[entry]; exists {
			return nil, fmt.Errorf("duplicate selected scope binding")
		}
		index[entry] = binding.Permission
	}
	if len(index) == 0 {
		return nil, nil
	}
	return &oauth.EntityEvaluationScopeProvider{Client: client, Principals: principals, Permission: func(request authz.Request, _ authz.Document) (string, error) {
		permission := index[key(request.Resource, request.Action)]
		if permission == "" {
			return "", authz.ErrDenied
		}
		return permission, nil
	}}, nil
}

func readHostAuthorization(path string) (*hostAuthorizationFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config hostAuthorizationFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&config); err != nil {
		return nil, err
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("authorization config must contain one JSON object")
	}
	if config.SchemaVersion != 1 || config.CatalogPath == "" {
		return nil, fmt.Errorf("authorization schemaVersion 1 and catalogPath are required")
	}
	if config.AccountProjection != "" && config.AccountProjection != "numeric" && config.AccountProjection != "opaque" {
		return nil, fmt.Errorf("unsupported authorization accountProjection")
	}
	if !filepath.IsAbs(config.CatalogPath) {
		config.CatalogPath = filepath.Join(filepath.Dir(path), config.CatalogPath)
	}
	if config.WindowResourcesPath != "" {
		if len(config.WindowResources) > 0 {
			return nil, fmt.Errorf("configure either windowResources or windowResourcesPath")
		}
		resourcesPath := config.WindowResourcesPath
		if !filepath.IsAbs(resourcesPath) {
			resourcesPath = filepath.Join(filepath.Dir(path), resourcesPath)
		}
		raw, err := os.ReadFile(resourcesPath)
		if err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&config.WindowResources); err != nil {
			return nil, err
		}
		if decoder.Decode(new(any)) != io.EOF {
			return nil, fmt.Errorf("invalid window resource manifest")
		}
	}
	return &config, nil
}

func validateHostAuthorizationSelection(config *hostAuthorizationFile, workspaceConfig *wscfg.Root) error {
	ui := workspaceConfig.UIAuthorizationSettings()
	policySettings := workspaceConfig.PolicyAuthorizationSettings()
	if ui.Mode != "authz" || policySettings.Mode != "authz" || ui.ProviderRef != config.ProviderRef || policySettings.ProviderRef != config.ProviderRef || ui.CapabilityMappingRef != config.CapabilityMappingRef {
		return fmt.Errorf("explicit authorization config requires matching authz UI and policy workspace settings")
	}
	return nil
}

func validateHostAuthorizationMappings(config *hostAuthorizationFile, bundle *oauth.StaticAuthorization) error {
	check := func(resource authz.Resource, action string) error {
		policyDocument, err := bundle.Policies.Get(context.Background(), resource)
		if err != nil || policyDocument.Policies[action].Mode == "" {
			return fmt.Errorf("authorization mapping %s:%s action %s lacks an ACL policy", resource.Kind, resource.ID, action)
		}
		if _, err = bundle.Requirements.GetRequirements(context.Background(), resource, action); err != nil {
			return fmt.Errorf("authorization mapping %s:%s action %s lacks requirements", resource.Kind, resource.ID, action)
		}
		return nil
	}
	for _, binding := range config.WholeResources {
		if err := check(binding.Resource, binding.Action); err != nil {
			return err
		}
	}
	for _, binding := range config.Capabilities {
		if binding.UseResolvedResource {
			if err := validateResolvedHostMapping(config, binding.Resource.Tenant, binding.Action); err != nil {
				return err
			}
			continue
		}
		if err := check(binding.Resource, binding.Action); err != nil {
			return err
		}
	}
	for _, binding := range config.BackendResources {
		if binding.UseResolvedResource {
			if err := validateResolvedHostMapping(config, binding.Resource.Tenant, binding.Action); err != nil {
				return err
			}
			continue
		}
		if err := check(binding.Resource, binding.Action); err != nil {
			return err
		}
	}
	for _, binding := range config.SelectedScopeBindings {
		if err := check(binding.Resource, binding.Action); err != nil {
			return err
		}
	}
	// External mappings can be partitioned by the current verified account.
	// No user identity exists during startup; their chosen revisions still pass
	// the same ACL and requirements checks on every runtime selection.
	if len(config.ResourceSelectionProvider) > 0 {
		if len(config.ResourceSelections) > 0 {
			return fmt.Errorf("choose one resource selection source")
		}

		for _, binding := range config.ResourceRevisionBindings {
			if selector, explicit := directResourceSelector(binding.Operation); explicit {
				if err := check(authz.Resource{Kind: binding.Resource.Kind, ID: binding.Resource.ID, Tenant: binding.Resource.Tenant, Version: selector}, binding.Action); err != nil {
					return err
				}
			}
		}
		return nil
	}
	selections, err := authz.NewStaticSelectionStore(config.ResourceSelections)
	if err != nil {
		return err
	}
	for _, binding := range config.ResourceRevisionBindings {
		if selector, explicit := directResourceSelector(binding.Operation); explicit {
			if err := check(authz.Resource{Kind: binding.Resource.Kind, ID: binding.Resource.ID, Tenant: binding.Resource.Tenant, Version: selector}, binding.Action); err != nil {
				return err
			}
			continue
		}
		doc, err := selections.GetSelection(context.Background(), binding.Resource)
		if err != nil {
			return fmt.Errorf("resource revision binding lacks selection policy")
		}
		versions := []string{doc.DefaultVersion}
		for _, override := range doc.Overrides {
			versions = append(versions, override.Version)
		}
		for _, version := range versions {
			if err := check(authz.Resource{Kind: binding.Resource.Kind, ID: binding.Resource.ID, Tenant: binding.Resource.Tenant, Version: version}, binding.Action); err != nil {
				return err
			}
		}
	}
	return nil
}

func configureHostAuthorization(options ServeOptions, workspaceRoot string, workspaceConfig *wscfg.Root, enrich windowloader.WorkspaceWindowEnricher, internal ...*internalHostRegistration) (func(context.Context, *executor.Builder) error, error) {
	path := strings.TrimSpace(options.AuthzConfigPath)
	if path == "" {
		path = strings.TrimSpace(os.Getenv("AGENTLY_AUTHZ_CONFIG"))
	}
	if path == "" {
		if options.InternalResourceProviderIdentity != "" {
			return nil, fmt.Errorf("internal resource provider requires explicit host authorization configuration")
		}
		return options.ConfigureBuilder, nil
	}
	config, err := readHostAuthorization(path)
	if err != nil {
		return nil, fmt.Errorf("authorization config: %w", err)
	}
	if err = validateHostAuthorizationSelection(config, workspaceConfig); err != nil {
		return nil, err
	}
	if options.AuthorizationProviderFactory == nil {
		return nil, fmt.Errorf("authorization configuration requires an explicit host provider factory")
	}
	providers, err := options.AuthorizationProviderFactory(context.Background(), config.Identity, config.EntityEvaluation)
	if err != nil {
		return nil, fmt.Errorf("authorization provider binding: %w", err)
	}
	bundle, err := oauth.NewStaticAuthorization(oauth.StaticAuthorizationConfig{
		Identity: providers.Identity, AllowsTenant: providers.AllowsTenant,
		Policies: config.Policies, Requirements: config.Requirements, CapabilityPermissions: config.CapabilityPermissions, EntityRoles: config.EntityRoles,
		EntityPermissions: providers.EntityPermissions, Entitlements: providers.Entitlements,
	})
	if err != nil {
		return nil, fmt.Errorf("authorization providers: %w", err)
	}
	if err = validateHostAuthorizationMappings(config, bundle); err != nil {
		return nil, err
	}
	entityCapability := bundle.EntityCapability
	var entityCapabilityWithLease func(context.Context, authz.Facts, authz.Entity, string) (bool, time.Time, error)
	var selectedScopes authz.SelectedScopeProvider
	if len(config.SelectedScopeBindings) > 0 && providers.EntityPermissions == nil {
		return nil, fmt.Errorf("selected scope bindings require an entity authority")
	}
	if providers.PermissionWithLease != nil {
		entityCapabilityWithLease, err = oauth.NewLeasedCapabilityPermissionResolver(config.CapabilityPermissions, providers.PermissionWithLease)
		if err != nil {
			return nil, err
		}
		entityCapability = func(ctx context.Context, facts authz.Facts, entity authz.Entity, capability string) (bool, error) {
			allowed, _, err := entityCapabilityWithLease(ctx, facts, entity, capability)
			return allowed, err
		}
	}
	selectedScopes, err = selectedScopeProvider(providers.EntityPermissions, providers.Identity, config.SelectedScopeBindings)
	if err != nil {
		return nil, err
	}
	bundle.ACL.SelectedScopes = selectedScopes
	resolveEntity := func(ctx context.Context, facts authz.Facts, entity authz.Entity, capability string) (bool, error) {
		return entityCapability(svcauth.AuthzIDTokenContext(ctx), facts, entity, capability)
	}
	acl := &authz.Service{Store: bundle.Policies, Provider: svcauth.IDTokenFactProvider{Source: bundle.Identity}}
	if selectedScopes != nil {
		acl.SelectedScopes = hostSelectedScopes{provider: selectedScopes}
	}
	var decisionScope executor.AuthorizationDecisionScope
	if providers.DecisionScope != nil {
		decisionScope = &idTokenDecisionScope{source: providers.DecisionScope}
	}
	registration := executor.StaticAuthorizationRegistration{DecisionScope: decisionScope, AuthoritySnapshot: svcauth.IDTokenIdentityProvider{Source: bundle.Identity}.ResolvePrincipal, ProviderRef: config.ProviderRef, CapabilityMappingRef: config.CapabilityMappingRef, PolicyVersion: config.PolicyVersion, Service: acl, Account: svcauth.IDTokenAccountResolver(bundle.Identity), AuthorityRevision: svcauth.IDTokenAuthorityRevisionResolver(bundle.Identity), GateEvaluator: bundle.Gates, EntityPermission: resolveEntity, EntityRoles: bundle.EntityRoleProjection, WholeResources: config.WholeResources, Capabilities: config.Capabilities, BackendResources: config.BackendResources, ProtectTools: config.ProtectTools}
	registration.ComponentAuthoritySnapshot = freshComponentAuthority(
		svcauth.IDTokenIdentityProvider{Source: bundle.Identity}.ResolvePrincipal,
		providers.MetadataScope, decisionScope)
	registration.ExecutionContext = func(ctx context.Context) context.Context {
		return freshExecutionContext(ctx, providers.MetadataScope, decisionScope)
	}
	if entityCapabilityWithLease != nil {
		registration.EntityPermissionWithLease = func(ctx context.Context, facts authz.Facts, entity authz.Entity, capability string) (bool, time.Time, error) {
			return entityCapabilityWithLease(svcauth.AuthzIDTokenContext(ctx), facts, entity, capability)
		}
	}
	registration.AccountProjection, err = configuredAccountProjection(config.AccountProjection, providers.AccountProjection)
	if err != nil {
		return nil, err
	}
	registration.ResourceRevisionBindings = config.ResourceRevisionBindings
	if len(config.ResourceSelectionProvider) > 0 {
		registration.ResourceRevisionMappings, err = authz.NewStaticSelectionStore(nil)
	} else {
		registration.ResourceRevisionMappings, err = configureHostResourceSelections(options, config, workspaceRoot, HostResourceAccess{})
	}
	if err != nil {
		return nil, err
	}
	prepared, err := executor.PrepareStaticAuthorization(registration)
	if err != nil {
		return nil, fmt.Errorf("authorization mappings: %w", err)
	}
	if len(config.ResourceSelectionProvider) > 0 {
		store, err := configureHostResourceSelections(options, config, workspaceRoot, preparedHostResourceAccess(prepared))
		if err != nil {
			return nil, err
		}
		if err := prepared.SetResourceRevisionMappings(store); err != nil {
			return nil, err
		}
	}
	var metadataScope forgeservice.MetadataReadScope
	if providers.MetadataScope != nil {
		metadataScope = &idTokenMetadataScope{source: providers.MetadataScope}
	}
	reportRegistration := &internalReportRegistration{}
	reportCatalog, reportResolver, reportWriter, windowSource, err := configureHostReportResources(options, config, workspaceRoot, prepared, svcauth.IDTokenIdentityProvider{Source: bundle.Identity}, reportRegistration)
	if err != nil {
		return nil, err
	}
	var nativeWindows *internalWindowSnapshot
	if options.InternalResourceProviderIdentity != "" {
		if options.WindowTargetProof == nil {
			key := make([]byte, 32)
			if _, err = rand.Read(key); err != nil {
				return nil, err
			}
			options.WindowTargetProof, err = forgetypes.NewWindowTargetHMAC(key)
			if err != nil {
				return nil, err
			}
		}
		nativeWindows, err = newInternalWindowSnapshot(context.Background(), workspaceRoot, workspaceConfig.ForgeReportingRoot(), config)
		if err != nil {
			return nil, err
		}
		windowSource = func(context.Context, identity.VerifiedActor) (identity.ResourceSource, error) {
			return nativeWindows, nil
		}
	}
	catalogOptions := []forgeservice.WindowCatalogOption{forgeservice.WithWindowMetadataScope(metadataScope)}
	if options.WindowTargetProof != nil {
		catalogOptions = append(catalogOptions, forgeservice.WithWindowTargetProof(options.WindowTargetProof))
	}
	if options.WindowLegacyTargetSupport != nil {
		catalogOptions = append(catalogOptions, forgeservice.WithWindowLegacyTargetSupport(options.WindowLegacyTargetSupport))
	} else if len(config.FrameworkWindowResources) > 0 {
		// The operator explicitly binds these linked framework resources. Only
		// byte-proven target equivalences for this exact artifact are advertised;
		// an older/different stamp never acquires current branch content.
		profiles := []forgetypes.WindowTarget{{Platform: "web"}, {Platform: "web", FormFactor: "desktop"}, {Platform: "web", FormFactor: "phone"}, {Platform: "web", FormFactor: "tablet"}, {Platform: "ios", FormFactor: "phone"}, {Platform: "ios", FormFactor: "tablet"}, {Platform: "android", FormFactor: "phone"}, {Platform: "android", FormFactor: "tablet"}}
		support, err := NewFrameworkLegacyTargetSupport(context.Background(), config.FrameworkWindowResources, profiles)
		if err != nil {
			return nil, err
		}
		catalogOptions = append(catalogOptions, forgeservice.WithWindowLegacyTargetSupport(support))
	}
	var windowResolve func(context.Context, string) (*identity.ResourceResolver, identity.ResourceRef, error)
	if len(config.WindowResources) > 0 || len(config.FrameworkWindowResources) > 0 {
		resolve, resolveErr := configuredWindowResourceResolver(config, workspaceRoot, prepared, enrich, windowSource)
		if resolveErr != nil {
			return nil, resolveErr
		}
		windowResolve = resolve
		if options.InternalResourceProviderIdentity != "" {
			original := windowResolve
			windowResolve = func(ctx context.Context, key string) (*identity.ResourceResolver, identity.ResourceRef, error) {
				resolver, ref, err := original(ctx, key)
				if err != nil {
					return nil, ref, err
				}
				copied := *resolver
				copied.ProviderIdentity = options.InternalResourceProviderIdentity
				return &copied, ref, nil
			}
			resolve = windowResolve
		}
		if err := prepared.WithWindowResourceResolver(resolve); err != nil {
			return nil, err
		}
		catalogOptions = append(catalogOptions, forgeservice.WithWindowResourceResolver(resolve))
	} else {
		catalogOptions = append(catalogOptions, forgeservice.WithWindowAuthorizer(prepared.WindowAuthorizer), forgeservice.WithWindowDefinitionLoader(func(ctx context.Context, id string) (*forgetypes.Window, error) {
			return windowloader.LoadWorkspaceWindowWithEnricherAt(ctx, workspaceRoot, id, nil, enrich)
		}))
	}
	catalog, err := forgeservice.LoadWindowCatalog(config.CatalogPath, catalogOptions...)
	if err != nil {
		return nil, fmt.Errorf("authorization catalog: %w", err)
	}
	if catalog.UsesResourceResolution() {
		if err := prepared.WithWindowTargetVerifier(catalog.VerifyWindowTarget); err != nil {
			return nil, err
		}
	}
	if reportCatalog != nil && metadataScope != nil {
		reportCatalog.BeginMetadataRead = metadataScope.BeginMetadataRead
	}
	var internalProvider *resourcesvc.LocalProvider
	if options.InternalResourceProviderIdentity != "" {
		if len(internal) != 1 || internal[0] == nil {
			return nil, fmt.Errorf("internal resource runtime binder required")
		}
		internalProvider, err = configureInternalHostResources(options.InternalResourceProviderIdentity, config, prepared, windowResolve, nativeWindows, reportRegistration, reportCatalog, metadataScope, options.WindowTargetProof, svcauth.IDTokenIdentityProvider{Source: bundle.Identity}, internal[0])
		if err != nil {
			return nil, err
		}
	}
	return func(ctx context.Context, builder *executor.Builder) error {
		if internalProvider != nil {
			builder.WithLocalPrimitiveProvider(internalProvider)
			builder.WithPrimitiveAuthority(prepared.ResourceActor, internalActorVerifier(prepared), options.InternalResourceProviderIdentity)
		}
		if reportCatalog != nil {
			builder.WithReportResources(reportCatalog, reportResolver)
			builder.WithReportResourceService(reportWriter)
		}
		builder.WithUIBridge(forgeservice.NewService(&forgeservice.Config{MetadataScope: metadataScope, WindowDefinitions: catalog, DynamicWindowAuthorizer: prepared.WindowAuthorizer, ResolvedWindowAuthorizer: prepared.AuthorizeResolvedWindow}))
		if err := prepared.Register(builder); err != nil {
			return err
		}
		if options.ConfigureBuilder != nil {
			return options.ConfigureBuilder(ctx, builder)
		}
		return nil
	}, nil
}

type idTokenMetadataScope struct {
	source forgeservice.MetadataReadScope
}

func (s *idTokenMetadataScope) BeginMetadataRead(ctx context.Context) (context.Context, func() error, error) {
	return s.source.BeginMetadataRead(svcauth.AuthzIDTokenContext(ctx))
}
func (s *idTokenMetadataScope) WithoutMetadataRead(ctx context.Context) context.Context {
	return s.source.WithoutMetadataRead(ctx)
}

// These lifecycle endpoints have explicit selectors; they never use a main map.
func directResourceSelector(operation string) (string, bool) {
	switch operation {
	case "resource.describe", "resource.selection.read", "resource.selection.update":
		return "logical", true
	case "resource.create", "resource.import", "resource.update", "resource.stamp", "resource.archive":
		return "working", true
	}
	return "", false
}

type idTokenDecisionScope struct {
	source executor.AuthorizationDecisionScope
}

func (s *idTokenDecisionScope) BeginDecision(ctx context.Context) (context.Context, func() error, error) {
	return s.source.BeginDecision(svcauth.AuthzIDTokenContext(ctx))
}
func (s *idTokenDecisionScope) WithoutDecision(ctx context.Context) context.Context {
	return s.source.WithoutDecision(ctx)
}

func (s *idTokenDecisionScope) MetadataReadActive(ctx context.Context) bool {
	return s.source.MetadataReadActive(svcauth.AuthzIDTokenContext(ctx))
}
