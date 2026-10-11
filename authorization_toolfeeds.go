package agently

import (
	"context"
	"fmt"

	"github.com/viant/agently-core/app/executor"
	identity "github.com/viant/agently-core/protocol/resource"
	resources "github.com/viant/agently-core/service/resource"
)

// HostToolfeedSource explicitly assigns a confined workspace directory to a
// logical namespace. It grants no tenant membership or revision authority.
type HostToolfeedSource struct {
	Namespace string `json:"namespace"`
	Directory string `json:"directory"`
}

func configuredToolfeedBindings(ctx context.Context, root, owner string, sources []HostToolfeedSource, prepared *executor.PreparedAuthorization, snapshot ...identity.ResourceSource) ([]resources.LocalResourceBinding, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	if prepared == nil || owner == "" {
		return nil, fmt.Errorf("toolfeed sources require explicit provider and authorization")
	}
	directories := make([]resources.ExtensionDirectory, 0, len(sources))
	for _, source := range sources {
		if source.Namespace == "users" {
			return nil, fmt.Errorf("toolfeed users namespace requires trusted ownership projection")
		}
		directories = append(directories, resources.ToolfeedDirectory(source.Namespace, source.Directory))
	}
	policy := func(ctx context.Context, actor identity.VerifiedActor, _ identity.ResourceURI, operation string) (identity.ResourceRevisionPolicy, error) {
		if err := internalActorVerifier(prepared)(ctx, actor); err != nil {
			return nil, err
		}
		resolver, err := prepared.ResourceResolver(operation, candidateSource{}, nil)
		if err != nil {
			return nil, err
		}
		return resolver.Policy, nil
	}
	bindings, err := resources.ExtensionBindings(ctx, root, directories, policy)
	if err != nil {
		return nil, err
	}
	for i := range bindings {
		original := bindings[i].Resolver
		bindings[i].Resolver = func(ctx context.Context, actor identity.VerifiedActor, operation string) (*identity.ResourceResolver, error) {
			resolver, err := original(ctx, actor, operation)
			if err != nil || resolver == nil {
				return nil, identity.ErrResourceDenied
			}
			copied := *resolver
			copied.ProviderIdentity = owner
			if len(snapshot) > 0 && snapshot[0] != nil {
				copied.Source = snapshot[0]
			}
			return &copied, nil
		}
	}
	return bindings, nil
}
