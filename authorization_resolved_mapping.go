package agently

import (
	"fmt"
	identity "github.com/viant/agently-core/protocol/resource"
)

// Runtime chooses the exact opened URI/stamp; construction only verifies that
// this trusted tenant/action is declared on a configured canonical resource.
// Other URIs/revisions still deny through the live policy store and gates.
func validateResolvedHostMapping(config *hostAuthorizationFile, tenant, action string) error {
	if tenant == "" || tenant == "*" || action == "" {
		return fmt.Errorf("resolved mapping requires explicit tenant/action")
	}
	approved := map[string]bool{}
	for _, binding := range config.ResourceRevisionBindings {
		approved[binding.Resource.ID] = true
	}
	for _, binding := range config.WindowResources {
		approved[binding.URI] = true
	}
	for _, binding := range config.FrameworkWindowResources {
		approved[binding.URI] = true
	}
	for _, document := range config.Policies {
		uri, err := identity.ParseResourceURI(document.Resource.ID)
		if err == nil && approved[uri.String()] && uri.Kind == document.Resource.Kind && document.Resource.Tenant == tenant && document.Policies[action].Mode != "" {
			for _, requirement := range config.Requirements {
				if requirement.Resource == document.Resource && requirement.Action == action {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("resolved mapping tenant/action lacks a canonical resource policy")
}
