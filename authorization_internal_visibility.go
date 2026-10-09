package agently

import (
	"context"
	"fmt"
	"os"
	"time"

	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/service/policy"
	forge "github.com/viant/agently-core/service/primitiveprovider"
	resources "github.com/viant/agently-core/service/resource"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/authz/gating"
	"gopkg.in/yaml.v3"
)

// Isolation never selects or reads a resource. The operator's explicit logical
// mappings establish namespace membership for the verified tenant. Native
// platform templates carry no individual ownership; users namespaces need a
// separate trusted directory/owner projection and are deliberately not served
// by this platform-only adapter.
func internalNamespaceIsolation(config *hostAuthorizationFile, bindings []resources.LocalResourceBinding) (resources.LocalResourceAuthorizer, error) {
	byURI := map[string]map[string]bool{}
	byNamespace := map[string]map[string]bool{}
	for _, entry := range bindings {
		uri, err := identity.ParseResourceURI(entry.URI)
		if err != nil {
			return nil, err
		}
		tenants := map[string]bool{}
		for _, mapping := range config.ResourceRevisionBindings {
			if mapping.URI == entry.URI && mapping.Resource.Kind == uri.Kind && mapping.Resource.ID == entry.URI && mapping.Resource.Tenant != "" {
				tenants[mapping.Resource.Tenant] = true
			}
		}
		if len(tenants) == 0 {
			return nil, fmt.Errorf("internal resource lacks explicit tenant mapping: %s", entry.URI)
		}
		byURI[entry.URI] = tenants
		if byNamespace[uri.Namespace] == nil {
			byNamespace[uri.Namespace] = map[string]bool{}
		}
		for tenant := range tenants {
			byNamespace[uri.Namespace][tenant] = true
		}
	}
	return func(ctx context.Context, actor identity.VerifiedActor, uri identity.ResourceURI, _ string) error {
		if ctx == nil || ctx.Err() != nil || !actor.Valid(time.Now()) || uri.Namespace == "users" {
			return identity.ErrResourceDenied
		}
		tenants := byURI[uri.String()]
		if uri.Kind == "resource" {
			tenants = byNamespace[uri.Namespace]
		}
		if !tenants[actor.TenantID] && !tenants["*"] {
			return identity.ErrResourceDenied
		}
		return nil
	}, nil
}

type internalVisibilityRequirements struct {
	exposures  []string
	roleGroups [][]string
}

// List is metadata discovery. Only declared roles/features participate; entity
// selections, datasource read checks and write capabilities belong to Open and
// execution, not catalog enumeration.
func internalWindowVisibility(config *hostAuthorizationFile, principals gating.PrincipalResolver) (resources.LocalWindowListVisibility, error) {
	var manifest struct {
		Windows []forge.SavedWindow `yaml:"windows"`
	}
	raw, err := os.ReadFile(config.CatalogPath)
	if err != nil {
		return nil, err
	}
	if err = yaml.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	byURI := map[string]internalVisibilityRequirements{}
	for _, binding := range append(append([]windowloader.ResourceBinding(nil), config.WindowResources...), config.FrameworkWindowResources...) {
		var required internalVisibilityRequirements
		for _, entry := range manifest.Windows {
			if entry.WindowID == binding.CatalogID || entry.WindowID == binding.WindowKey || entry.ResourceURI == binding.URI {
				if len(entry.Roles) > 0 {
					required.roleGroups = append(required.roleGroups, append([]string(nil), entry.Roles...))
				}
			}
		}
		for _, mapping := range config.ResourceRevisionBindings {
			if mapping.URI != binding.URI || mapping.Operation != policy.OperationWindowView {
				continue
			}
			for _, requirement := range config.Requirements {
				if requirement.Resource.Kind != "window" || requirement.Resource.ID != binding.URI || requirement.Resource.Tenant != mapping.Resource.Tenant || requirement.Action != mapping.Action {
					continue
				}
				declared := requirement.Document.Requirements
				required.exposures = append(required.exposures, declared.RequiredExposures...)
				if len(declared.AllowedRoles) > 0 {
					required.roleGroups = append(required.roleGroups, append([]string(nil), declared.AllowedRoles...))
				}
			}
		}
		byURI[binding.URI] = required
	}
	return func(ctx context.Context, actor identity.VerifiedActor, binding resources.LocalResourceBinding) (bool, error) {
		required := byURI[binding.URI]
		if len(required.exposures) == 0 && len(required.roleGroups) == 0 {
			return true, nil
		}
		if principals == nil {
			return false, identity.ErrResourceDenied
		}
		principal, err := principals.ResolvePrincipal(ctx)
		if err != nil {
			return false, err
		}
		facts := principal.Facts
		if !facts.ValidUntil.After(time.Now()) || facts.Subject != actor.Subject || facts.Issuer != actor.Issuer || facts.Tenant != actor.TenantID || principal.AccountID != actor.AccountID || principal.IdentityRevision != actor.IdentityRevision {
			return false, identity.ErrResourceDenied
		}
		contains := func(values []string, value string) bool {
			for _, candidate := range values {
				if candidate == value {
					return true
				}
			}
			return false
		}
		for _, exposure := range required.exposures {
			if !contains(facts.Exposures, exposure) {
				return false, nil
			}
		}
		for _, group := range required.roleGroups {
			matched := false
			for _, role := range group {
				if contains(facts.Roles, role) {
					matched = true
					break
				}
			}
			if !matched {
				return false, nil
			}
		}
		return true, nil
	}, nil
}
