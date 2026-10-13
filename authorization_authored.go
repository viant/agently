package agently

import (
	"context"
	"fmt"
	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/app/host"
	identity "github.com/viant/agently-core/protocol/resource"
	resources "github.com/viant/agently-core/service/resource"
	"github.com/viant/agently-core/workspace"
)

// HostAuthoredSource declares a native authoring folder and logical namespace.
// It grants no identity, business permissions, or revision selection.
type HostAuthoredSource struct {
	Namespace string `json:"namespace"`
	Directory string `json:"directory"`
}

type authoredCompiler interface {
	Prepare(context.Context, *resources.ExtensionReader) error
	Release()
}

func configuredAuthoredDirectories(config *hostAuthorizationFile) ([]resources.ExtensionDirectory, []authoredCompiler, error) {
	var dirs []resources.ExtensionDirectory
	var compilers []authoredCompiler
	for _, src := range config.TemplateSources {
		if src.Namespace == "users" {
			return nil, nil, fmt.Errorf("users templates require trusted ownership projection")
		}
		c := resources.NewAuthoredCompilation(src.Directory)
		compilers = append(compilers, c)
		dirs = append(dirs, c.TemplateDirectory(src.Namespace))
	}
	for _, src := range config.IntentSources {
		if src.Namespace == "users" {
			return nil, nil, fmt.Errorf("users intents require trusted ownership projection")
		}
		c := resources.NewIntentCompilation(src.Directory, workspace.KindToolBundle)
		compilers = append(compilers, c)
		dirs = append(dirs, c.IntentDirectory(src.Namespace))
	}
	return dirs, compilers, nil
}
func configuredAuthoredBindings(ctx context.Context, root, owner string, config *hostAuthorizationFile, prepared *executor.PreparedAuthorization, snapshot identity.ResourceSource) ([]resources.LocalResourceBinding, error) {
	if len(config.TemplateSources)+len(config.IntentSources) == 0 {
		return nil, nil
	}
	if owner == "" || prepared == nil || snapshot == nil {
		return nil, identity.ErrResourceDenied
	}
	dirs, _, err := configuredAuthoredDirectories(config)
	if err != nil {
		return nil, err
	}
	return host.BindSources(ctx, root, owner, dirs, func(directory resources.ExtensionDirectory) (resources.ExtensionDirectory, error) {
		return directory, nil
	}, func(ctx context.Context, a identity.VerifiedActor, _ identity.ResourceURI, operation string) (identity.ResourceRevisionPolicy, error) {
		if err := internalActorVerifier(prepared)(ctx, a); err != nil {
			return nil, err
		}
		r, err := prepared.ResourceResolver(operation, candidateSource{}, nil)
		if err != nil {
			return nil, err
		}
		return r.Policy, nil
	}, snapshot)
}
