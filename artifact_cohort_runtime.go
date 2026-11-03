package agently

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/artifact/cohort"
	"github.com/viant/agently-core/protocol/primitive/observe"
	resources "github.com/viant/agently-core/service/resource"
)

// Both hosts install the same local metadata source before warming executable
// catalogs. The scheduler still discovers public and private tools as its user.
func activateArtifactCohorts(ctx, lifetimeCtx context.Context, rt *executor.Runtime, configPath string, bindings []ArtifactCohortBinding, registrations []ArtifactCohortRegistration) ([]ArtifactCohortBinding, func(), error) {
	bindings = append([]ArtifactCohortBinding(nil), bindings...)
	var stops []func()
	closeBindings := func() {
		for i := len(stops) - 1; i >= 0; i-- {
			stops[i]()
		}
		closeArtifactCohorts(bindings)
	}
	if configPath != "" {
		prepared, err := cohort.PrepareLocalBindings(ctx, rt, configPath)
		if err != nil {
			return bindings, closeBindings, fmt.Errorf("local artifact cohort activation: %w", err)
		}
		bindings = append(bindings, prepared...)
	}
	for _, registration := range registrations {
		binding, err := cohort.PrepareBinding(ctx, rt, registration)
		if err != nil {
			return bindings, closeBindings, fmt.Errorf("artifact cohort registration: %w", err)
		}
		bindings = append(bindings, binding)
	}
	for _, binding := range bindings {
		providerIdentity := strings.TrimSpace(binding.ProviderIdentity)
		if providerIdentity == "" || binding.Source == nil {
			return bindings, closeBindings, fmt.Errorf("invalid artifact cohort provider registration")
		}
		if binding.Start != nil {
			if err := binding.Start(ctx); err != nil {
				return bindings, closeBindings, fmt.Errorf("artifact cohort notifications %s: %w", providerIdentity, err)
			}
		}
		timing := observe.Start(ctx, "startup.provider.preload", observe.Labels{Kind: "artifact"})
		if err := binding.Source.Sync(ctx); err != nil {
			timing.End(observe.OutcomeFor(err), observe.CacheUnknown)
			return bindings, closeBindings, fmt.Errorf("artifact cohort preload %s: %w", providerIdentity, err)
		}
		timing.End(observe.OK, observe.CacheUnknown)
		if binding.ConnectionName == "" || binding.Metadata == nil {
			return bindings, closeBindings, fmt.Errorf("artifact cohort metadata source is required for %s", providerIdentity)
		}
		if err := rt.InstallPublishedMetadata(resources.Connection{Name: binding.ConnectionName, ProviderIdentity: providerIdentity}, binding.Namespaces, binding.Metadata, binding.ConnectionAliases...); err != nil {
			return bindings, closeBindings, fmt.Errorf("install artifact cohort metadata %s: %w", providerIdentity, err)
		}
		log.Printf("artifact cohort ready: connection=%q aliases=%q provider=%q namespaces=%q metadata=local", binding.ConnectionName, binding.ConnectionAliases, providerIdentity, binding.Namespaces)
		stops = append(stops, cohort.StartReconciliation(lifetimeCtx, binding))
	}
	return bindings, closeBindings, nil
}

func closeArtifactCohorts(bindings []ArtifactCohortBinding) {
	for _, binding := range bindings {
		if binding.Close != nil {
			binding.Close()
		}
	}
}
