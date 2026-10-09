package agently

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/protocol/primitive"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/service/policy"
	"github.com/viant/agently-core/service/reporting"
	"github.com/viant/agently-core/service/reporting/catalog"
	resourcesvc "github.com/viant/agently-core/service/resource"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
)

// ReportResourceProviders supplies storage, never authorization. Source receives
// the current verified actor so a SQL implementation can bind tenant/account.
// Each canonical URI must have one configured source, including after imports.
type HostResourceAccess struct {
	// ProviderIdentity enables explicit native snapshot/provider construction.
	// It is supplied by the embedding host, never resource request data.
	ProviderIdentity string
	Resolver         func(context.Context, string, identity.ResourceSource) (*identity.ResourceResolver, error)
	Actor            func(context.Context) (identity.VerifiedActor, error)
	Authority        identity.ResourceAuthority
}

type ReportResourceProviders struct {
	LocalBindings  []resourcesvc.LocalResourceBinding
	WindowSource   func(context.Context, identity.VerifiedActor) (identity.ResourceSource, error)
	BuilderWindows map[string]string
	Writer         primitive.ResourceAuthoringFactory
	Source         func(context.Context, identity.VerifiedActor) (identity.ResourceSource, error)
	Inventories    []catalog.ReportInventory
}

// ReportResourceProviderFactory interprets only operator-owned storage settings.
// Construction must not migrate stores or import definitions. Those are explicit
// administrative operations, separate from validating/starting an HTTP host.
type ReportResourceProviderFactory func(context.Context, json.RawMessage, string, HostResourceAccess) (ReportResourceProviders, error)

func configureHostReportResources(options ServeOptions, config *hostAuthorizationFile, workspaceRoot string, prepared *executor.PreparedAuthorization, principals gating.PrincipalResolver, internal ...*internalReportRegistration) (*catalog.ReportCatalogService, reporting.ResourceResolver, primitive.ResourceAuthoring, func(context.Context, identity.VerifiedActor) (identity.ResourceSource, error), error) {
	if len(config.ReportResources) == 0 {
		return nil, nil, nil, nil, nil
	}
	if options.ReportResourceProviderFactory == nil {
		return nil, nil, nil, nil, fmt.Errorf("report resources require an explicit host storage factory")
	}
	access := preparedHostResourceAccess(prepared)
	access.ProviderIdentity = options.InternalResourceProviderIdentity
	providers, err := options.ReportResourceProviderFactory(context.Background(), config.ReportResources, workspaceRoot, access)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("report resource storage: %w", err)
	}
	if providers.Source == nil || len(providers.Inventories) == 0 || prepared == nil || principals == nil {
		return nil, nil, nil, nil, fmt.Errorf("report resources require source, inventory and verified authority")
	}
	if len(internal) > 0 && internal[0] != nil {
		internal[0].bindings = append([]resourcesvc.LocalResourceBinding(nil), providers.LocalBindings...)
	}
	actor := prepared.ResourceActor
	resolve := func(ctx context.Context, operation string) (*identity.ResourceResolver, error) {
		verified, err := actor(ctx)
		if err != nil {
			return nil, err
		}
		source, err := providers.Source(ctx, verified)
		if err != nil {
			return nil, err
		}
		return prepared.ResourceResolver(operation, source, nil)
	}
	reportCatalog := &catalog.ReportCatalogService{BuilderWindows: providers.BuilderWindows, Identity: actor, Inventories: append([]catalog.ReportInventory(nil), providers.Inventories...), Resolver: func(ctx context.Context) (*identity.ResourceResolver, error) { return resolve(ctx, "report.retrieve") }}
	reportCatalog.Capabilities = func(ctx context.Context, _ identity.VerifiedActor, _ catalog.ReportCatalogCandidate, pin identity.ResolvedResource) (catalog.ReportCapabilities, error) {
		var result catalog.ReportCapabilities
		for _, check := range []struct {
			operation string
			allowed   *bool
		}{
			{"report.retrieve", &result.Open}, {"report.execute", &result.Run},
			{"report.edit", &result.Edit}, {"report.duplicate", &result.Duplicate},
			{"report.edit", &result.Rename}, {"report.export", &result.Export},
			{"report.delete", &result.Delete}, {"report.runRange", &result.RunRange},
		} {
			resolver, err := resolve(ctx, check.operation)
			if err != nil {
				return catalog.ReportCapabilities{}, err
			}
			_, _, err = resolver.ReadResolved(ctx, pin)
			if err == nil {
				*check.allowed = true
				continue
			}
			// Identity outages/rejection must discard the response, not appear as
			// a harmless absent button. Only ordinary action denial is a flag.
			if errors.Is(err, policy.ErrIdentityRejected) || errors.Is(err, authz.ErrIdentityDenied) || !errors.Is(err, identity.ErrResourceDenied) {
				return catalog.ReportCapabilities{}, err
			}
		}
		return result, nil
	}
	var writer primitive.ResourceAuthoring
	if providers.Writer != nil {
		writer, err = providers.Writer(prepared.ResourceAuthority())
		if err != nil || writer == nil {
			return nil, nil, nil, nil, fmt.Errorf("report writer authority binding failed")
		}
	}
	return reportCatalog, resolve, writer, providers.WindowSource, nil
}

func preparedHostResourceAccess(prepared *executor.PreparedAuthorization) HostResourceAccess {
	return HostResourceAccess{Actor: prepared.ResourceActor, Authority: prepared.ResourceAuthority(), Resolver: func(_ context.Context, operation string, source identity.ResourceSource) (*identity.ResourceResolver, error) {
		return prepared.ResourceResolver(operation, source, nil)
	}}
}
