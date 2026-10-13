package agently

import (
	"context"
	"fmt"

	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/app/host"
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
	var sourceSnapshot identity.ResourceSource
	if len(snapshot) > 0 {
		sourceSnapshot = snapshot[0]
	}
	return host.BindSources(ctx, root, owner, sources, func(source HostToolfeedSource) (resources.ExtensionDirectory, error) {
		if source.Namespace == "users" {
			return resources.ExtensionDirectory{}, fmt.Errorf("toolfeed users namespace requires trusted ownership projection")
		}
		return resources.ToolfeedDirectory(source.Namespace, source.Directory), nil
	}, policy, sourceSnapshot)
}
