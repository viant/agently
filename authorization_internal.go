package agently

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/viant/agently-core/app/executor"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/service/policy"
	forge "github.com/viant/agently-core/service/primitiveprovider"
	reportcatalog "github.com/viant/agently-core/service/reporting/catalog"
	resources "github.com/viant/agently-core/service/resource"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/forge/backend/types"
)

type internalReportRegistration struct {
	bindings []resources.LocalResourceBinding
}
type internalHostRegistration struct {
	initialize func(context.Context) error
	bind       func(context.Context, *executor.Runtime) error
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
// bytes cannot be read: the gateway owns content and host admission checks the
// already-verified exact candidate without a second YAML materialization.
type candidateSource struct{}

func (candidateSource) Candidates(context.Context, identity.ResourceURI) ([]identity.ResourceCandidate, error) {
	return nil, identity.ErrResourceDenied
}
func (candidateSource) ReadCandidate(context.Context, identity.ResourceURI, identity.ResourceCandidate) (json.RawMessage, error) {
	return nil, identity.ErrResourceDenied
}
func internalCandidateAdmission(prepared *executor.PreparedAuthorization, owner string, ctx context.Context, operation string, pin identity.ResolvedResource) error {
	_, err := internalCandidateLease(prepared, owner, ctx, operation, pin)
	return err
}
func internalCandidateLease(prepared *executor.PreparedAuthorization, owner string, ctx context.Context, operation string, pin identity.ResolvedResource) (time.Time, error) {
	if pin.ProviderIdentity != owner || !pin.ValidUntil.After(time.Now()) {
		return time.Time{}, identity.ErrResourceDenied
	}
	resolver, err := prepared.ResourceResolver(operation, candidateSource{}, nil)
	if err != nil {
		return time.Time{}, err
	}
	decision, err := resolver.Policy.SelectRevision(ctx, identity.ResourceRef{URI: pin.URI, Revision: pin.Selector()}, []identity.ResourceCandidate{pin.ResourceCandidate})
	if err != nil {
		return time.Time{}, err
	}
	if decision.Candidate != pin.ResourceCandidate || decision.AuthorityBinding != pin.AuthorityBinding || !decision.ValidUntil.After(time.Now()) || !pin.ValidUntil.After(time.Now()) {
		return time.Time{}, identity.ErrResourceDenied
	}
	lease := pin.ValidUntil
	if decision.ValidUntil.Before(lease) {
		lease = decision.ValidUntil
	}
	return lease, nil
}
func internalOperationAdmission(prepared *executor.PreparedAuthorization, owner string) resources.ReportAdmission {
	return func(ctx context.Context, operation string, pin identity.ResolvedResource, raw json.RawMessage) error {
		if identity.ContentFingerprint(raw) != pin.ContentFingerprint {
			return identity.ErrResourceDenied
		}
		return internalCandidateAdmission(prepared, owner, ctx, operation, pin)
	}
}
func internalOperationLeaseAdmission(prepared *executor.PreparedAuthorization, owner string) resources.ReportLeaseAdmission {
	return func(ctx context.Context, operation string, pin identity.ResolvedResource, raw json.RawMessage) (time.Time, error) {
		if identity.ContentFingerprint(raw) != pin.ContentFingerprint {
			return time.Time{}, identity.ErrResourceDenied
		}
		return internalCandidateLease(prepared, owner, ctx, operation, pin)
	}
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
	authorize, err := internalNamespaceIsolation(config, bindings, nativeAuthority)
	if err != nil {
		return nil, err
	}
	visibility, err := internalWindowVisibility(config, principals)
	if err != nil {
		return nil, err
	}
	localConfig := resources.LocalConfig{ProviderIdentity: owner, Actor: actorResolver, Verify: verify, Authorize: authorize, Bindings: bindings, Validators: map[string]resources.LocalResourceValidator{"window": resources.ValidateWindowBundle, "report": resources.ValidateReportEnvelope}}
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
		if len(config.WindowResources)+len(config.FrameworkWindowResources) > 0 {
			if rt.UIBridge == nil || proof == nil {
				return fmt.Errorf("internal windows require the host bridge and target proof")
			}
			remote := &resources.WindowCatalog{Gateway: rt.PrimitiveProviders, TargetProof: proof, Admission: func(ctx context.Context, pin identity.ResolvedResource, _ *types.Window) error {
				if nativeAuthority != nil {
					return nativeAuthority.admit(ctx, pin)
				}
				return internalCandidateAdmission(prepared, owner, ctx, policy.OperationWindowView, pin)
			}}
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
			if nativeAuthority != nil {
				windows.native = nativeAuthority
				// This phase reads static native definitions only; entity admission
				// and datasource execution retain their separate fresh providers.
				rt.UIBridge.ConfigureWindowReadDecisionScope(func(ctx context.Context) (context.Context, func() error, error) {
					return ctx, func() error { return ctx.Err() }, nil
				})
			}
			rt.UIBridge.ConfigureWindowCatalog(func(forge.WindowDefinitionCatalog) forge.WindowDefinitionCatalog { return windows })
		}
		if catalog == nil {
			return nil
		}
		if rt.Reporting == nil {
			return fmt.Errorf("internal report runtime missing")
		}
		gatewayCatalog := &resources.ReportGatewayCatalog{Gateway: rt.PrimitiveProviders, AdmissionLease: internalOperationLeaseAdmission(prepared, owner), BuilderWindows: catalog.BuilderWindows, Capabilities: internalReportCapabilities(prepared, owner)}
		if scope != nil {
			gatewayCatalog.BeginMetadataRead = scope.BeginMetadataRead
		}
		rt.Reporting.SetReportCatalog(gatewayCatalog)
		rt.Reporting.SetResourceReaderFactory(gatewayCatalog.Reader)
		return nil
	}
	return local, nil
}

func internalReportCapabilities(prepared *executor.PreparedAuthorization, owner string) func(context.Context, identity.VerifiedActor, reportcatalog.ReportCatalogCandidate, identity.ResolvedResource) (reportcatalog.ReportCapabilities, error) {
	return func(ctx context.Context, _ identity.VerifiedActor, _ reportcatalog.ReportCatalogCandidate, pin identity.ResolvedResource) (reportcatalog.ReportCapabilities, error) {
		var result reportcatalog.ReportCapabilities
		for _, check := range []struct {
			operation string
			allowed   *bool
		}{{"report.retrieve", &result.Open}, {"report.execute", &result.Run}, {"report.edit", &result.Edit}, {"report.duplicate", &result.Duplicate}, {"report.edit", &result.Rename}, {"report.export", &result.Export}, {"report.delete", &result.Delete}, {"report.runRange", &result.RunRange}} {
			err := internalCandidateAdmission(prepared, owner, ctx, check.operation, pin)
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
