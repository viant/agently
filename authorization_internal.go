package agently

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/viant/agently-core/sdk"
	"time"

	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/app/host"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/service/policy"
	forge "github.com/viant/agently-core/service/primitiveprovider"
	reportcatalog "github.com/viant/agently-core/service/reporting/catalog"
	resources "github.com/viant/agently-core/service/resource"
	windowloader "github.com/viant/agently-core/service/ui/window"
	workspaceui "github.com/viant/agently-core/service/workspace"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/forge/backend/types"
)

type internalReportRegistration struct {
	bindings []resources.LocalResourceBinding
}
type internalHostRegistration struct {
	menuReader      func(*executor.Runtime) (workspaceui.LayoutMenuReader, error)
	hostedAppReader func(*executor.Runtime, sdk.Backend) (sdk.MCPUIResourceReader, sdk.MCPUIResourceContextBinder, error)
	workspaceRoot   string
	initialize      func(context.Context) error
	bind            func(context.Context, *executor.Runtime) error
}

func (r *internalHostRegistration) Bind(ctx context.Context, rt *executor.Runtime) error {
	if r != nil && r.initialize != nil {
		if err := r.initialize(ctx); err != nil {
			return err
		}
	}
	if r != nil && r.bind != nil {
		return r.bind(ctx, rt)
	}
	return nil
}

// candidateSource is used only to construct the existing operation policy. Its
// bytes cannot be read: it supports UI control visibility without loading a
// second copy of the definition. It does not authorize resource delivery.
type candidateSource struct{}

func (candidateSource) Candidates(context.Context, identity.ResourceURI) ([]identity.ResourceCandidate, error) {
	return nil, identity.ErrResourceDenied
}
func (candidateSource) ReadCandidate(context.Context, identity.ResourceURI, identity.ResourceCandidate) (json.RawMessage, error) {
	return nil, identity.ErrResourceDenied
}

// reportControlVisibility evaluates UI controls only. Report delivery and tool
// execution are authorized by the managing MCP, never by this callback.
func reportControlVisibility(prepared *executor.PreparedAuthorization, owner string, ctx context.Context, operation string, pin identity.ResolvedResource) error {
	if pin.ProviderIdentity != owner {
		return identity.ErrResourceDenied
	}
	resolver, err := prepared.ResourceResolver(operation, candidateSource{}, nil)
	if err != nil {
		return err
	}
	_, err = resolver.Policy.SelectRevision(ctx, identity.ResourceRef{URI: pin.URI, Revision: pin.Selector()}, []identity.ResourceCandidate{pin.ResourceCandidate})
	return err
}
func internalActorVerifier(prepared *executor.PreparedAuthorization) resources.ActorVerifier {
	return func(ctx context.Context, held identity.VerifiedActor) error {
		current, err := prepared.ResourceActor(ctx)
		if err != nil {
			return err
		}
		if !held.Valid(time.Now()) || !current.Valid(time.Now()) || held.Subject != current.Subject || held.Issuer != current.Issuer || held.TenantID != current.TenantID || held.AccountID != current.AccountID || held.IdentityRevision != current.IdentityRevision {
			return identity.ErrResourceDenied
		}
		return nil
	}
}
func configureInternalHostResources(owner string, config *hostAuthorizationFile, prepared *executor.PreparedAuthorization, windowResolve func(context.Context, string) (*identity.ResourceResolver, identity.ResourceRef, error), snapshot *internalWindowSnapshot, reports *internalReportRegistration, catalog *reportcatalog.ReportCatalogService, scope forge.MetadataReadScope, proof types.WindowTargetProof, principals gating.PrincipalResolver, registration *internalHostRegistration, native ...*nativeWindowAuthority) (*resources.LocalProvider, error) {
	if owner == "" || prepared == nil || registration == nil {
		return nil, fmt.Errorf("explicit internal provider and runtime binding required")
	}
	var nativeAuthority *nativeWindowAuthority
	if len(native) > 0 {
		nativeAuthority = native[0]
	}
	actorResolver := resources.ActorResolver(prepared.ResourceActor)
	if nativeAuthority != nil {
		actorResolver = nativeAuthority.resolveActor(actorResolver)
	}
	verify := verifyActor(actorResolver)
	var bindings []resources.LocalResourceBinding
	for _, group := range [][]windowloader.ResourceBinding{config.WindowResources, config.FrameworkWindowResources} {
		for _, declared := range group {
			if windowResolve == nil {
				return nil, fmt.Errorf("internal windows require exact configured resolver")
			}
			entry := declared
			bindings = append(bindings, resources.LocalResourceBinding{URI: entry.URI, Title: entry.WindowKey, FormatVersion: 2, Resolver: func(ctx context.Context, actor identity.VerifiedActor, _ string) (*identity.ResourceResolver, error) {
				if err := verify(ctx, actor); err != nil {
					return nil, err
				}
				resolver, ref, err := windowResolve(ctx, entry.URI)
				if err != nil || resolver == nil || ref.URI != entry.URI {
					return nil, identity.ErrResourceDenied
				}
				copied := *resolver
				copied.ProviderIdentity = owner
				if snapshot != nil && snapshot.workspace != nil {
					if _, linked := snapshot.framework[entry.URI]; !linked {
						// Preserve the snapshot's terminal metadata checks and pure
						// validator proof rather than wrapping away those contracts.
						copied.Source = snapshot.workspace
					}
				}
				return &copied, nil
			}})
		}
	}
	if catalog != nil {
		if reports == nil || len(reports.bindings) == 0 {
			return nil, fmt.Errorf("internal reports require explicit trusted native bindings")
		}
		for _, declared := range reports.bindings {
			entry := declared
			original := entry.Resolver
			if original == nil {
				return nil, fmt.Errorf("internal report resolver missing")
			}
			entry.Resolver = func(ctx context.Context, actor identity.VerifiedActor, operation string) (*identity.ResourceResolver, error) {
				if err := internalActorVerifier(prepared)(ctx, actor); err != nil {
					return nil, err
				}
				resolver, err := original(ctx, actor, operation)
				if err != nil || resolver == nil {
					return nil, identity.ErrResourceDenied
				}
				copied := *resolver
				copied.ProviderIdentity = owner
				return &copied, nil
			}
			bindings = append(bindings, entry)
		}
	}
	if len(config.ToolfeedSources)+len(config.TemplateSources)+len(config.IntentSources) > 0 && snapshot == nil {
		var snapshotErr error
		snapshot, snapshotErr = newInternalWindowSnapshot(context.Background(), registration.workspaceRoot, "", config)
		if snapshotErr != nil {
			return nil, snapshotErr
		}
	}
	sourcePolicy := internalSourceRevisionPolicy(prepared)
	toolfeeds, err := host.BindToolfeedSources(context.Background(), registration.workspaceRoot, owner, config.ToolfeedSources, sourcePolicy, snapshot)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, toolfeeds...)
	authored, err := host.BindAuthoredSources(context.Background(), registration.workspaceRoot, owner, config.TemplateSources, config.IntentSources, sourcePolicy, snapshot)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, authored...)
	menus, err := configuredMenuBindings(owner, config, prepared, snapshot)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, menus...)
	authorize, err := internalNamespaceIsolation(config, bindings, nativeAuthority)
	if err != nil {
		return nil, err
	}
	visibility, err := internalWindowVisibility(config, principals)
	if err != nil {
		return nil, err
	}
	localConfig := resources.LocalConfig{ProviderIdentity: owner, Actor: actorResolver, Verify: verify, Authorize: authorize, Bindings: bindings, Validators: map[string]resources.LocalResourceValidator{"window": resources.ValidateWindowBundle, "report": resources.ValidateReportEnvelope, "toolfeed": resources.ValidateToolfeedDefinition}}
	if len(authored) > 0 {
		localConfig.Validators["template"] = resources.ValidateTemplateDefinition
		localConfig.Validators["intent"] = resources.ValidateIntentDefinition
	}
	if len(menus) > 0 {
		localConfig.Validators["navigation"] = resources.ValidateMenu
		localConfig.MenuActionVisibility, err = configuredMenuVisibility(config, principals, authorize)
		if err != nil {
			return nil, err
		}
	}
	if snapshot != nil {
		localConfig.WindowIndex, localConfig.WindowListVisibility = snapshot.WindowIndex, visibility
	}
	local, err := resources.NewLocalProvider(localConfig)
	if err != nil {
		return nil, err
	}
	registration.bind = func(ctx context.Context, rt *executor.Runtime) error {
		if rt == nil || rt.PrimitiveProviders == nil {
			return fmt.Errorf("internal resource gateway was not constructed")
		}
		if snapshot != nil {
			rt.NativeLayoutSource = snapshot.layoutSource
		}
		if len(config.WindowResources)+len(config.FrameworkWindowResources) > 0 {
			if rt.UIBridge == nil || proof == nil {
				return fmt.Errorf("internal windows require the host bridge and target proof")
			}
			remote := &resources.WindowCatalog{Gateway: rt.PrimitiveProviders, TargetProof: proof}
			// These bindings are captured from host-owned workspace/framework
			// assets. Preserve their native HTTP services on this exact source;
			// an external provider cannot enable host HTTP transport this way.
			var nativeURIs []string
			for _, group := range [][]windowloader.ResourceBinding{config.WindowResources, config.FrameworkWindowResources} {
				for _, binding := range group {
					nativeURIs = append(nativeURIs, binding.URI)
				}
			}
			if err := remote.RegisterNativeHTTPWindows(resources.Connection{Name: "internal", ProviderIdentity: owner}, nativeURIs); err != nil {
				return err
			}

			if snapshot != nil {
				remote.ContentCurrent = map[string]resources.WindowContentCheck{
					owner: func(ctx context.Context, pin identity.ResolvedResource) error {
						uri, err := identity.ParseResourceURI(pin.URI)
						if err != nil || uri.Kind != "window" || pin.ProviderIdentity != owner {
							return identity.ErrResourceDenied
						}
						return snapshot.CheckCandidate(ctx, uri, pin.ResourceCandidate)
					},
				}
			}
			windows, err := newInternalWindowCatalog(remote, append(append([]windowloader.ResourceBinding(nil), config.WindowResources...), config.FrameworkWindowResources...))
			if err != nil {
				return err
			}
			windows.providerIdentity = owner
			if nativeAuthority != nil {
				windows.native = nativeAuthority
				// This phase reads static native definitions only; entity admission
				// and datasource execution retain their separate fresh providers.
				rt.UIBridge.ConfigureWindowReadDecisionScope(func(ctx context.Context) (context.Context, func() error, error) {
					return ctx, func() error { return ctx.Err() }, nil
				})
			}
			bindInternalWindowCatalog(rt, owner, windows)
		}
		if catalog == nil {
			return nil
		}
		if rt.Reporting == nil {
			return fmt.Errorf("internal report runtime missing")
		}
		gatewayCatalog := &resources.ReportGatewayCatalog{Gateway: rt.PrimitiveProviders, BuilderWindows: catalog.BuilderWindows, Capabilities: hostedReportCapabilities(prepared, owner)}
		if scope != nil {
			gatewayCatalog.BeginMetadataRead = scope.BeginMetadataRead
		}
		rt.Reporting.SetReportCatalog(gatewayCatalog)
		rt.Reporting.SetResourceReaderFactory(gatewayCatalog.Reader)
		return nil
	}
	return local, nil
}

// Late internal registration replaces only the local side of an already
// configured composite. Delegated providers keep their admission and proof.
func bindInternalWindowCatalog(rt *executor.Runtime, owner string, windows forge.WindowDefinitionCatalog) {
	catalog := windows
	if rt.PrimitiveWindows != nil {
		rt.PrimitiveWindows.Local = windows
		rt.PrimitiveWindows.LocalProviderIdentity = owner
		catalog = rt.PrimitiveWindows
	}
	rt.UIBridge.ConfigureWindowCatalog(func(forge.WindowDefinitionCatalog) forge.WindowDefinitionCatalog { return catalog })
}

func internalReportCapabilities(prepared *executor.PreparedAuthorization, owner string) func(context.Context, identity.VerifiedActor, reportcatalog.ReportCatalogCandidate, identity.ResolvedResource) (reportcatalog.ReportCapabilities, error) {
	return func(ctx context.Context, _ identity.VerifiedActor, _ reportcatalog.ReportCatalogCandidate, pin identity.ResolvedResource) (reportcatalog.ReportCapabilities, error) {
		var result reportcatalog.ReportCapabilities
		for _, check := range []struct {
			operation string
			allowed   *bool
		}{{"report.retrieve", &result.Open}, {"report.execute", &result.Run}, {"report.edit", &result.Edit}, {"report.duplicate", &result.Duplicate}, {"report.edit", &result.Rename}, {"report.export", &result.Export}, {"report.delete", &result.Delete}, {"report.runRange", &result.RunRange}} {
			err := reportControlVisibility(prepared, owner, ctx, check.operation, pin)
			if err == nil {
				*check.allowed = true
				continue
			}
			if errors.Is(err, policy.ErrIdentityRejected) || errors.Is(err, authz.ErrIdentityDenied) || !(errors.Is(err, identity.ErrResourceDenied) || errors.Is(err, authz.ErrDenied) || errors.Is(err, policy.ErrDenied)) {
				return reportcatalog.ReportCapabilities{}, err
			}
		}
		return result, nil
	}
}

func hostedReportCapabilities(prepared *executor.PreparedAuthorization, owner string) func(context.Context, identity.VerifiedActor, reportcatalog.ReportCatalogCandidate, identity.ResolvedResource) (reportcatalog.ReportCapabilities, error) {
	local := internalReportCapabilities(prepared, owner)
	return func(ctx context.Context, actor identity.VerifiedActor, candidate reportcatalog.ReportCatalogCandidate, pin identity.ResolvedResource) (reportcatalog.ReportCapabilities, error) {
		if pin.ProviderIdentity == owner {
			return local(ctx, actor, candidate, pin)
		}
		result := reportcatalog.ReportCapabilities{}
		for _, check := range []struct {
			operation string
			allowed   *bool
		}{{"report.retrieve", &result.Open}, {"report.execute", &result.Run}, {"report.export", &result.Export}} {

			*check.allowed = true
		}
		return result, nil
	}
}

// This callback belongs to the managing in-memory provider, not a consumer.
func internalSourceRevisionPolicy(prepared *executor.PreparedAuthorization) resources.LocalRevisionPolicyFactory {
	if prepared == nil {
		return nil
	}
	verify := internalActorVerifier(prepared)
	return func(ctx context.Context, actor identity.VerifiedActor, _ identity.ResourceURI, operation string) (identity.ResourceRevisionPolicy, error) {
		if err := verify(ctx, actor); err != nil {
			return nil, err
		}
		resolver, err := prepared.ResourceResolver(operation, candidateSource{}, nil)
		if err != nil {
			return nil, err
		}
		return resolver.Policy, nil
	}
}
